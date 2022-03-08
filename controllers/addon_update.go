package controllers

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func (c *Controller) updateAddonStatusLifecycle(ctx context.Context, namespace, name string, lifecycle string, lifecyclestatus wfv1.WorkflowPhase) error {
	c.logger.Info("updating addon ", namespace, "/", name, " ", lifecycle, " status to ", lifecyclestatus)

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

	prevStatus := latest.Status
	newStatus := addonv1.AddonStatus{
		Lifecycle: addonv1.AddonStatusLifecycle{},
		Resources: []addonv1.ObjectStatus{},
	}
	if lifecycle == "prereqs" {
		newStatus.Lifecycle.Prereqs = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
		newStatus.Lifecycle.Installed = prevStatus.Lifecycle.Installed
	} else if lifecycle == "install" || lifecycle == "delete" {
		newStatus.Lifecycle.Installed = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
		newStatus.Lifecycle.Prereqs = prevStatus.Lifecycle.Prereqs
	}
	newStatus.Resources = append(newStatus.Resources, prevStatus.Resources...)
	newStatus.Checksum = prevStatus.Checksum
	newStatus.Reason = prevStatus.Reason
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

	if reflect.DeepEqual(prevStatus, updating.Status) {
		msg := fmt.Sprintf("addon %s/%s status the same. skip update.", updating.Namespace, updating.Name)
		c.logger.Info(msg)
		return nil
	}

	_, err = c.addoncli.AddonmgrV1alpha1().Addons(updating.Namespace).UpdateStatus(ctx, updating, metav1.UpdateOptions{})
	if err != nil {
		switch {
		case errors.IsNotFound(err):
			msg := fmt.Sprintf("Addon %s/%s is not found. %v", updating.Namespace, updating.Name, err)
			c.logger.Error(msg)
			return fmt.Errorf(msg)
		case strings.Contains(err.Error(), "the object has been modified"):
			c.logger.Info("retry updating object for workflow status change.")
			if err := c.updateAddonStatusLifecycle(ctx, namespace, name, lifecycle, lifecyclestatus); err != nil {
				c.logger.Error("failed updating ", updating.Namespace, "/", updating.Name, " lifecycle status ", err)
				return err
			}
		default:
			c.logger.Error("failed updating ", updating.Namespace, "/", updating.Name, " status ", err)
			return err
		}
	}
	msg := fmt.Sprintf("successfully update addon %s/%s step %s status to %s", namespace, name, lifecycle, lifecyclestatus)
	c.logger.Info(msg)

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

// update the whole addon object, the apply should be cautious
func (c *Controller) updateAddon(ctx context.Context, updated *addonv1.Addon) error {
	latest, err := c.addoncli.AddonmgrV1alpha1().Addons(updated.Namespace).Get(ctx, updated.Name, metav1.GetOptions{})
	if err != nil {
		msg := fmt.Sprintf("[updateAddon] failed getting addon(name) %s err %#v", updated.Name, err)
		c.logger.Error(msg)
		return fmt.Errorf(msg)
	}

	if reflect.DeepEqual(updated, latest) {
		c.logger.Info("latest and updated addon is the same, skip updating")
		return nil
	}
	_, err = c.addoncli.AddonmgrV1alpha1().Addons(updated.Namespace).Update(ctx, updated,
		metav1.UpdateOptions{})
	if err != nil {
		msg := fmt.Sprintf("Failed updating addon %s/%s. err %v", updated.Namespace, updated.Name, err)
		fmt.Print(msg)
		return fmt.Errorf(msg)
	}
	return nil
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
	info := strings.Split(key, "/")
	ns, name := info[0], info[1]
	latest, err := c.addoncli.AddonmgrV1alpha1().Addons(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		msg := fmt.Sprintf("failed finding addon %s err %v.", key, err)
		c.logger.Error(msg)
		return fmt.Errorf(msg)
	}
	updating := latest.DeepCopy()

	c.logger.Info(" addon ", updating.Namespace, "/", updating.Name, " new resources -- ", resource, " existing resources -- ", updating.Status.Resources)
	newResources := []addonv1.ObjectStatus{resource}
	updating.Status.Resources = c.mergeResources(newResources, updating.Status.Resources)
	var errs []error
	if _, err = c.addoncli.AddonmgrV1alpha1().Addons(updating.Namespace).UpdateStatus(ctx, updating,
		metav1.UpdateOptions{}); err != nil {
		switch {
		case errors.IsNotFound(err):
			return err
		case strings.Contains(err.Error(), "the object has been modified"):
			c.logger.Warnf("retry updateAddonStatusResources %s  coz the object has been modified", resource)
			if err := c.updateAddonStatusResources(ctx, key, resource); err != nil {
				errs = append(errs, fmt.Errorf("[updateAddonStatusResources] failed to update addon %s/%s status: %w", updating.Namespace,
					updating.Name, err))
			}
		default:
			errs = append(errs, fmt.Errorf("[updateAddonStatusResources] default failed to update addon %s/%s status: %w", updating.Namespace,
				updating.Name, err))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("failed updating addon resources %#v", errs)
}
