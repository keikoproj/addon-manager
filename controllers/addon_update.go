package controllers

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	addonapiv1 "github.com/keikoproj/addon-manager/api/addon"
	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (c *Controller) isAddonCompleted(addon *addonv1.Addon) bool {
	_, ok := addon.Labels[addonapiv1.AddonCompleteLabel]
	return ok && addon.Labels[addonapiv1.AddonCompleteLabel] == addonapiv1.AddonCompleteTrueKey
}

func (c *Controller) updateAddonStatusLifecycle(ctx context.Context, namespace, name string, lifecycle string, lifecyclestatus wfv1.WorkflowPhase) error {
	c.logger.Info("[updateAddonStatusLifecycle] updating addon ", namespace, "/", name, " ", lifecycle, " status to ", lifecyclestatus)

	key := fmt.Sprintf("%s/%s", namespace, name)
	obj, exists, err := c.informer.GetIndexer().GetByKey(key)
	if err != nil || !exists {
		msg := fmt.Sprintf("[updateAddonStatusLifecycle] failed getting addon taged namespace/name %s/%s, err %v", namespace, name, err)
		c.logger.Error(msg)
		return fmt.Errorf(msg)
	}
	latest, err := common.FromUnstructured(obj.(*unstructured.Unstructured))
	if err != nil {
		msg := fmt.Sprintf("failed converting un to addon,  err %v", err)
		c.logger.Error(msg)
		return fmt.Errorf(msg)
	}
	updating := latest.DeepCopy()
	if c.isAddonCompleted(updating) {
		c.logger.Infof("[updateAddonStatusLifecycle] addon %s/%s completed. skip.", namespace, name)
		return nil
	}
	prevStatus := latest.Status

	// addon being deletion, skip non-delete wf update
	if lifecycle != "delete" &&
		prevStatus.Lifecycle.Installed == addonv1.Deleting {
		c.logger.Infof("[updateAddonStatusLifecycle] %s/%s is being deleting and delete wf not completed. skip update.", namespace, name)
		return nil
	}

	newStatus := addonv1.AddonStatus{
		Lifecycle: addonv1.AddonStatusLifecycle{},
		Resources: []addonv1.ObjectStatus{},
	}
	newStatus.Reason = prevStatus.Reason
	if lifecycle == "prereqs" {
		newStatus.Lifecycle.Prereqs = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
		newStatus.Lifecycle.Installed = prevStatus.Lifecycle.Installed
	} else if lifecycle == "install" || lifecycle == "delete" {
		newStatus.Lifecycle.Installed = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
		newStatus.Lifecycle.Prereqs = prevStatus.Lifecycle.Prereqs
		if addonv1.ApplicationAssemblyPhase(lifecyclestatus) == addonv1.Succeeded {
			newStatus.Reason = ""
		}
	}
	newStatus.Resources = append(newStatus.Resources, prevStatus.Resources...)
	newStatus.Checksum = prevStatus.Checksum
	newStatus.StartTime = prevStatus.StartTime
	updating.Status = newStatus

	if lifecycle == "delete" && addonv1.ApplicationAssemblyPhase(lifecyclestatus).Succeeded() {
		c.logger.Info("addon", namespace, "/", name, " installation completed or addon being deleting. the deletion wf completed.")
		if prevStatus.Lifecycle.Installed.Completed() || prevStatus.Lifecycle.Installed.Deleting() {
			c.removeFinalizer(updating)
			_, err = c.addoncli.AddonmgrV1alpha1().Addons(updating.Namespace).Update(ctx, updating, metav1.UpdateOptions{})
			if err != nil {
				switch {
				case errors.IsNotFound(err):
					msg := fmt.Sprintf("Addon %s/%s is not found. %v", updating.Namespace, updating.Name, err)
					c.logger.Error(msg)
					return fmt.Errorf(msg)
				case strings.Contains(err.Error(), "the object has been modified"):
					c.logger.Info("retry updating object for deleted addon.")
					if _, err := c.addoncli.AddonmgrV1alpha1().Addons(updating.Namespace).Update(ctx, updating, metav1.UpdateOptions{}); err != nil {
						c.logger.Error("failed retry updating ", updating.Namespace, updating.Name, " lifecycle status err ", err)
						return err
					}
				default:
					c.logger.Error("failed updating ", updating.Namespace, updating.Name, " lifecycle status err ", err)
					return err
				}
			}
			return nil
		} // goes to branch line 114, update status only
	}

	var afterupdating *addonv1.Addon
	patchLabel := false
	if lifecycle == "install" && addonv1.ApplicationAssemblyPhase(lifecyclestatus).Succeeded() {
		c.logger.Infof("addon %s/%s completed. patch complete label.", namespace, name)
		labels := updating.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[addonapiv1.AddonCompleteLabel] = addonapiv1.AddonCompleteTrueKey
		updating.SetLabels(labels)
		afterupdating, err = c.updateAddon(ctx, updating)
		if err != nil {
			c.logger.Errorf("updateAddonStatusLifecycle failed label addon %s/%s completed err %#v", updating.Namespace, updating.Name, err)
			return err
		}
		patchLabel = true
	}

	if patchLabel {
		updating = afterupdating.DeepCopy()
	}

	if reflect.DeepEqual(prevStatus, updating.Status) {
		msg := fmt.Sprintf("updateAddonStatusLifecycle addon %s/%s status the same. skip update.", updating.Namespace, updating.Name)
		c.logger.Info(msg)
		return nil
	}

	_, err = c.addoncli.AddonmgrV1alpha1().Addons(updating.Namespace).UpdateStatus(ctx, updating, metav1.UpdateOptions{})
	if err != nil {
		switch {
		case errors.IsNotFound(err):
			msg := fmt.Sprintf("updateAddonStatusLifecycle Addon %s/%s is not found. %v", updating.Namespace, updating.Name, err)
			c.logger.Error(msg)
			return fmt.Errorf(msg)
		case strings.Contains(err.Error(), "the object has been modified"):
			c.logger.Warn("updateAddonStatusLifecycle retry updating object for workflow status change.")
			if err := c.updateAddonStatusLifecycle(ctx, namespace, name, lifecycle, lifecyclestatus); err != nil {
				c.logger.Error("updateAddonStatusLifecycle failed updating ", updating.Namespace, "/", updating.Name, " lifecycle status ", err)
				return err
			}
		default:
			c.logger.Error("updateAddonStatusLifecycle failed updating ", updating.Namespace, "/", updating.Name, " status ", err)
			return err
		}
	}
	c.logger.Infof("updateAddonStatusLifecycle successfully update addon %s/%s step %s status to %s", namespace, name, lifecycle, lifecyclestatus)
	return nil

}

func (c *Controller) resetAddonStatus(ctx context.Context, addon *addonv1.Addon) error {
	latest, err := c.addoncli.AddonmgrV1alpha1().Addons(addon.Namespace).Get(ctx, addon.Name, metav1.GetOptions{})
	if err != nil {
		msg := fmt.Sprintf("failed finding addon %s err %v.", addon.Name, err)
		c.logger.Error(msg)
		return fmt.Errorf(msg)
	}
	updating := latest.DeepCopy()

	updating.Status.StartTime = common.GetCurretTimestamp()
	updating.Status.Lifecycle.Prereqs = ""
	updating.Status.Lifecycle.Installed = ""
	updating.Status.Reason = ""
	updating.Status.Resources = []addonv1.ObjectStatus{}
	updating.Status.Checksum = addon.Status.Checksum

	_, err = c.addoncli.AddonmgrV1alpha1().Addons(updating.Namespace).UpdateStatus(ctx, updating, metav1.UpdateOptions{})
	if err != nil {
		switch {
		case errors.IsNotFound(err):
			msg := fmt.Sprintf("Addon %s/%s is not found. %v", updating.Namespace, updating.Name, err)
			c.logger.Error(msg)
			return fmt.Errorf(msg)
		case strings.Contains(err.Error(), "the object has been modified"):
			if latest.Status.Lifecycle.Installed != addonv1.Deleting { // edge case: latest is in an error status, skip retry
				c.logger.Warnf("[resetAddonStatus] retry updating %s/%s coz objects has been modified", latest.Namespace, latest.Name)
				if err := c.resetAddonStatus(ctx, addon); err != nil {
					c.logger.Error("failed retry updating ", updating.Namespace, updating.Name, " lifecycle status ", err)
				}
			}
		default:
			c.logger.Error("failed updating ", updating.Namespace, updating.Name, " status err ", err)
			return err
		}
	}
	msg := fmt.Sprintf("successfully updated addon %s status", updating.Name)
	c.logger.Info(msg)
	return nil
}

func (c *Controller) updateAddonStatus(ctx context.Context, addon *addonv1.Addon) (*addonv1.Addon, error) {
	c.logger.Infof("updateAddonStatus %s/%s ", addon.Namespace, addon.Name)
	latest, err := c.addoncli.AddonmgrV1alpha1().Addons(addon.Namespace).Get(ctx, addon.Name, metav1.GetOptions{})
	if err != nil {
		msg := fmt.Sprintf("failed finding addon %s err %v.", addon.Name, err)
		c.logger.Error(msg)
		return nil, fmt.Errorf(msg)
	}
	updating := latest.DeepCopy()

	if reflect.DeepEqual(updating.Status, addon.Status) {
		c.logger.Infof("updateAddonStatus %s/%s the same. skip.", addon.Namespace, addon.Name)
		return nil, nil
	}

	updating.Status = addonv1.AddonStatus{
		Checksum: addon.Status.Checksum,
		Lifecycle: addonv1.AddonStatusLifecycle{
			Installed: addon.Status.Lifecycle.Installed,
			Prereqs:   addon.Status.Lifecycle.Prereqs,
		},
		Reason:    addon.Status.Reason,
		StartTime: addon.Status.StartTime,
		Resources: c.mergeResources(addon.Status.Resources, latest.Status.Resources),
	}

	updated, err := c.addoncli.AddonmgrV1alpha1().Addons(updating.Namespace).UpdateStatus(ctx, updating, metav1.UpdateOptions{})
	if err != nil {
		switch {
		case errors.IsNotFound(err):
			msg := fmt.Sprintf("[updateAddonStatus] Addon %s/%s is not found. %v", updating.Namespace, updating.Name, err)
			c.logger.Error(msg)
			return nil, fmt.Errorf(msg)
		case strings.Contains(err.Error(), "the object has been modified"):
			c.logger.Warnf("[updateAddonStatus] retry updating %s/%s coz objects has been modified", latest.Namespace, latest.Name)
			if _, err := c.updateAddonStatus(ctx, addon); err != nil {
				c.logger.Error("[updateAddonStatus] failed retry updating ", updating.Namespace, updating.Name, " lifecycle status ", err)
			}

		default:
			c.logger.Error("[updateAddonStatus] failed updating ", updating.Namespace, updating.Name, " status err ", err)
			return nil, err
		}
	}
	c.logger.Infof("[updateAddonStatus] %s successfully", updating.Name)
	return updated, nil
}

func (c *Controller) updateAddonLifeCycle(ctx context.Context, namespace, name string, prereqphase, installphase *addonv1.ApplicationAssemblyPhase, reason string) error {
	latest, err := c.addoncli.AddonmgrV1alpha1().Addons(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("[updateAddonLifeCycle] failed get addon %s/%s ", namespace, name)
	}
	updating := latest.DeepCopy()
	if prereqphase != nil {
		updating.Status.Lifecycle.Prereqs = *prereqphase
	}
	if installphase != nil {
		updating.Status.Lifecycle.Installed = *installphase
	}
	if len(reason) > 0 {
		updating.Status.Reason = reason
	}

	_, err = c.addoncli.AddonmgrV1alpha1().Addons(updating.Namespace).UpdateStatus(ctx, updating, metav1.UpdateOptions{})
	if err != nil {
		switch {
		case errors.IsNotFound(err):
			msg := fmt.Sprintf("Addon %s/%s is not found. %v", updating.Namespace, updating.Name, err)
			c.logger.Error(msg)
			return fmt.Errorf(msg)
		case strings.Contains(err.Error(), "the object has been modified"):
			if latest.Status.Lifecycle.Installed != addonv1.Deleting { // edge case: latest is in an error status, skip retry
				c.logger.Warnf("[updateAddonLifeCycle] retry updating %s/%s coz objects has been modified", latest.Namespace, latest.Name)
				if err := c.updateAddonLifeCycle(ctx, namespace, name, prereqphase, installphase, reason); err != nil {
					c.logger.Error("failed retry updating ", updating.Namespace, updating.Name, " lifecycle status ", err)
				}
			}
		default:
			c.logger.Error("failed updating ", updating.Namespace, updating.Name, " status err ", err)
			return err
		}
	}

	if prereqphase != nil {
		c.logger.Infof("successfully updated addon %s prereqp to status  %s", name, *prereqphase)
	}
	if installphase != nil {
		c.logger.Infof("successfully updated addon %s install to status  %s", name, *installphase)
	}

	return nil
}

func (c *Controller) mergeLabels(old, new map[string]string) map[string]string {
	merged := map[string]string{}
	for k, v := range old {
		merged[k] = v
	}
	for k, v := range new {
		merged[k] = v
	}
	return merged
}

func (c *Controller) mergeFinlizer(old, new []string) []string {
	merged := map[string]int{}
	for _, finalizer := range old {
		merged[finalizer] = 1
	}
	for _, finalizer := range new {
		merged[finalizer] = 1
	}
	ret := []string{}
	for f, _ := range merged {
		ret = append(ret, f)
	}
	return ret
}

// update addon meta ojbect first, then update status
func (c *Controller) updateAddon(ctx context.Context, updated *addonv1.Addon) (*addonv1.Addon, error) {
	var errs []error

	latest, err := c.addoncli.AddonmgrV1alpha1().Addons(updated.Namespace).Get(ctx, updated.Name, metav1.GetOptions{})
	if err != nil || latest == nil {
		msg := fmt.Sprintf("[updateAddon] failed getting %s err %#v", updated.Name, err)
		c.logger.Error(msg)
		return nil, err
	} else {

		if reflect.DeepEqual(updated, latest) {
			c.logger.Infof("[updateAddon] latest and updated %s/%s is the same, skip", updated.Namespace, updated.Name)
			return nil, nil

		}

		// update object metata only
		updating := latest.DeepCopy()
		updating.ObjectMeta.Finalizers = c.mergeFinlizer(latest.Finalizers, updated.Finalizers)
		updating.ObjectMeta.Labels = c.mergeLabels(latest.Labels, updated.Labels)

		_, err := c.addoncli.AddonmgrV1alpha1().Addons(updated.Namespace).Update(ctx, updating,
			metav1.UpdateOptions{})

		if err != nil {
			switch {
			case errors.IsNotFound(err):
				msg := fmt.Sprintf("[updateAddon] Addon %s/%s is not found. %v", updated.Namespace, updated.Name, err)
				c.logger.Error(msg)
				return nil, fmt.Errorf(msg)
			case strings.Contains(err.Error(), "the object has been modified"):
				errs = append(errs, err)
				c.logger.Warnf("[updateAddon] retry updating object metadata %s/%s coz objects has been modified", updated.Namespace, updated.Name)
				if _, err := c.updateAddon(ctx, updated); err != nil {
					c.logger.Errorf("[updateAddon] retry updating %s/%s, coz err %#v", updated.Namespace, updated.Name, err)
				}
			default:
				c.logger.Error("[updateAddon] failed  ", updated.Namespace, updated.Name, " err ", err)
				return nil, err
			}
		}
		_, err = c.updateAddonStatus(ctx, updated)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		c.logger.Infof("[updateAddon] %s/%s succeed.", updated.Namespace, updated.Name)
		return updated, nil
	}
	c.logger.Errorf("[updateAddon] %s/%s failed.", updated.Namespace, updated.Name)
	return nil, fmt.Errorf("%v", errs)
}

func (c *Controller) mergeResources(res1, res2 []addonv1.ObjectStatus) []addonv1.ObjectStatus {
	merged := []addonv1.ObjectStatus{}
	check := make(map[string]addonv1.ObjectStatus)
	mix := append(res1, res2...)
	for _, obj := range mix {
		id := fmt.Sprintf("%s-%s-%s", strings.TrimSpace(obj.Name), strings.TrimSpace(obj.Kind), strings.TrimSpace(obj.Group))
		check[id] = obj
	}
	for _, obj := range check {
		merged = append(merged, obj)
	}
	return merged
}

func (c *Controller) updateAddonStatusResources(ctx context.Context, key string, resource addonv1.ObjectStatus) error {
	c.logger.Infof("updateAddonStatusResources %s resource %s", key, resource)
	_, name := c.namespacenameFromKey(key)
	un, err := c.dynCli.Resource(schema.GroupVersionResource{
		Group:    addonapiv1.Group,
		Version:  "v1alpha1",
		Resource: addonapiv1.AddonPlural,
	}).Get(ctx, name, metav1.GetOptions{})
	var errs []error
	if err == nil && un != nil {
		updating, err := common.FromUnstructured(un)
		if err != nil {
			msg := fmt.Sprintf("[updateAddonStatusResources] failed finding addon %s err %v.", key, err)
			c.logger.Error(msg)
			return fmt.Errorf(msg)
		}

		c.logger.Info("updateAddonStatusResources  ", updating.Namespace, "/", updating.Name, " new resources -- ", resource, " existing resources -- ", updating.Status.Resources)
		newResources := []addonv1.ObjectStatus{resource}
		updating.Status.Resources = c.mergeResources(newResources, updating.Status.Resources)

		if _, err = c.addoncli.AddonmgrV1alpha1().Addons(updating.Namespace).UpdateStatus(ctx, updating,
			metav1.UpdateOptions{}); err != nil {
			switch {
			case errors.IsNotFound(err):
				return err
			case strings.Contains(err.Error(), "the object has been modified"):
				c.logger.Warnf("[updateAddonStatusResources] retry %s coz the object has been modified", resource)
				if err := c.updateAddonStatusResources(ctx, key, resource); err != nil {
					errs = append(errs, fmt.Errorf("[updateAddonStatusResources] failed to update addon %s/%s resources: %w", updating.Namespace,
						updating.Name, err))
				}
			default:
				errs = append(errs, fmt.Errorf("[updateAddonStatusResources] default failed to update addon %s/%s resources: %w", updating.Namespace,
					updating.Name, err))
			}
		}
	}

	if len(errs) == 0 {
		c.logger.Infof("updateAddonStatusResources %s resource %s successfully", key, resource)
		return nil
	}
	c.logger.Errorf("updateAddonStatusResources %s resource %s failed", key, resource)
	return fmt.Errorf("failed updating addon resources %#v", errs)
}
