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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// if labeled "addons.addonmgr.keikoproj.io/completed"
func (c *Controller) isAddonCompleted(addon *addonv1.Addon) bool {
	_, ok := addon.Labels[addonapiv1.AddonCompleteLabel]
	return ok && addon.Labels[addonapiv1.AddonCompleteLabel] == addonapiv1.AddonCompleteTrueKey
}

func (c *Controller) updateAddonStatusLifecycle(ctx context.Context, namespace, name string, lifecycle string, lifecyclestatus wfv1.WorkflowPhase) error {
	c.logger.Info(fmt.Sprintf("[updateAddonStatusLifecycle] updating addon %s/%s step %s status to %s", namespace, name, lifecycle, lifecyclestatus))

	key := fmt.Sprintf("%s/%s", namespace, name)
	latest, err := c.getExistingAddon(ctx, key)
	if err != nil || latest == nil {
		return err
	}
	updating := latest.DeepCopy()
	prevStatus := latest.Status
	if c.isAddonCompleted(updating) && prevStatus.Lifecycle.Installed != addonv1.Deleting {
		c.logger.Info(fmt.Sprintf("[updateAddonStatusLifecycle] addon %s/%s completed, but not deleting. skip.", namespace, name))
		return nil
	}

	// addon being deletion, skip non-delete wf update
	if lifecycle != "delete" &&
		prevStatus.Lifecycle.Installed == addonv1.Deleting {
		c.logger.Info(fmt.Sprintf("[updateAddonStatusLifecycle] %s/%s is being deleting. skip non-delete wf update.", namespace, name))
		return nil
	}

	newStatus := addonv1.AddonStatus{
		Lifecycle: addonv1.AddonStatusLifecycle{},
		Resources: []addonv1.ObjectStatus{},
	}
	newStatus.Reason = prevStatus.Reason
	newStatus.Resources = append(newStatus.Resources, prevStatus.Resources...)
	newStatus.Checksum = prevStatus.Checksum
	newStatus.StartTime = prevStatus.StartTime
	if lifecycle == "prereqs" {
		newStatus.Lifecycle.Prereqs = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
		newStatus.Lifecycle.Installed = prevStatus.Lifecycle.Installed
		if newStatus.Lifecycle.Prereqs == addonv1.Failed {
			newStatus.Lifecycle.Installed = addonv1.Failed
			newStatus.Reason = "prereqs wf fails."
			c.logger.Info("[updateAddonStatusLifecycle] %s/%s prereq failed. mark addon failure also", namespace, name)
		}
	} else if lifecycle == "install" || lifecycle == "delete" {
		newStatus.Lifecycle.Installed = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
		newStatus.Lifecycle.Prereqs = prevStatus.Lifecycle.Prereqs
		if addonv1.ApplicationAssemblyPhase(lifecyclestatus) == addonv1.Succeeded {
			newStatus.Reason = ""
		}

		// check whether need patch complete
		if lifecycle == "install" && newStatus.Lifecycle.Installed.Completed() {
			c.logger.Info("[updateAddonStatusLifecycle] %s/%s completed. patch complete label.",
				updating.Namespace, updating.Name)
			labels := updating.GetLabels()
			if labels == nil {
				labels = map[string]string{}
			}
			labels[addonapiv1.AddonCompleteLabel] = addonapiv1.AddonCompleteTrueKey
			updating.SetLabels(labels)

			updating.Status = newStatus
			if err := c.updateAddon(ctx, updating); err != nil {
				c.logger.Error(err, "updateAddonStatusLifecycle %s/%s update complete failed.", namespace, name)
				return err
			}
			return nil
		}
	}
	updating.Status = newStatus

	if lifecycle == "delete" && addonv1.ApplicationAssemblyPhase(lifecyclestatus).Succeeded() {
		c.logger.Info(fmt.Sprintf("addon %s/%s installation completed or addon being deleting. the deletion wf completed.", namespace, name))
		if prevStatus.Lifecycle.Installed.Completed() || prevStatus.Lifecycle.Installed.Deleting() {
			c.removeFinalizer(updating)
			if err := c.updateAddon(ctx, updating); err != nil {
				//c.logger.Error(err, "updateAddonStatusLifecycle failed updating ", updating.Namespace, updating.Name, " lifecycle status err ", err)
				return err
			}
			c.removeFromCache(updating.Name)
			return nil
		}
	}

	if lifecycle == "install" && addonv1.ApplicationAssemblyPhase(lifecyclestatus).Succeeded() {
		c.logger.Info(fmt.Sprintf("addon %s/%s completed. patch complete label.", namespace, name))
		labels := updating.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[addonapiv1.AddonCompleteLabel] = addonapiv1.AddonCompleteTrueKey
		updating.SetLabels(labels)
		err = c.updateAddon(ctx, updating)
		if err != nil {
			//c.logger.Error(err, "updateAddonStatusLifecycle failed label addon %s/%s completed err %#v", updating.Namespace, updating.Name, err)
			return err
		}
	}

	if reflect.DeepEqual(prevStatus, updating.Status) {
		msg := fmt.Sprintf("updateAddonStatusLifecycle addon %s/%s status the same. skip update.", updating.Namespace, updating.Name)
		c.logger.Info(msg)
		return nil
	}

	err = c.updateAddonStatus(ctx, updating)
	if err != nil {
		//c.logger.Error(err, "updateAddonStatusLifecycle failed updating ", updating.Namespace, "/", updating.Name, " status ", err)
		return err
	}
	c.logger.Info(fmt.Sprintf("updateAddonStatusLifecycle successfully update addon %s/%s step %s status to %s", namespace, name, lifecycle, lifecyclestatus))
	return nil

}

func (c *Controller) resetAddonStatus(ctx context.Context, addon *addonv1.Addon) error {

	addon.Status.StartTime = common.GetCurretTimestamp()
	addon.Status.Lifecycle.Prereqs = ""
	addon.Status.Lifecycle.Installed = ""
	addon.Status.Reason = ""
	addon.Status.Resources = []addonv1.ObjectStatus{}
	addon.Status.Checksum = addon.CalculateChecksum()

	// remove complete label also
	labels := addon.GetLabels()
	if labels != nil {
		delete(labels, string(addonapiv1.AddonCompleteLabel))
	}
	addon.SetLabels(labels)

	err := c.updateAddon(ctx, addon)
	if err != nil {
		c.logger.Error(err, "failed resetting ", addon.Namespace, addon.Name, " status err ", err)
		return err
	}

	msg := fmt.Sprintf("successfully reset addon %s status", addon.Name)
	c.logger.Info(msg)
	return nil
}

func (c *Controller) updateAddonStatus(ctx context.Context, addon *addonv1.Addon) error {
	c.logger.WithValues("[updateAddonStatus]", fmt.Sprintf(" %s/%s ", addon.Namespace, addon.Name))
	latest, err := c.addoncli.AddonmgrV1alpha1().Addons(addon.Namespace).Get(ctx, addon.Name, metav1.GetOptions{})
	if err != nil {
		msg := fmt.Sprintf("updateAddonStatus failed finding addon %s err %v.", addon.Name, err)
		c.logger.Error(err, msg)
		return err
	}
	updating := latest.DeepCopy()
	if reflect.DeepEqual(updating.Status, addon.Status) {
		c.logger.WithValues("[updateAddonStatus]", fmt.Sprintf(" %s/%s the same. skip.", addon.Namespace, addon.Name))
		return nil
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

	err = c.client.Status().Update(ctx, addon, &client.UpdateOptions{})
	if err != nil {
		switch {
		case errors.IsNotFound(err):
			msg := fmt.Sprintf("[updateAddonStatus] addon %s/%s is not found. %v", addon.Namespace, addon.Name, err)
			c.logger.Error(err, msg)
			return err
		case strings.Contains(err.Error(), "the object has been modified"):
			//c.logger.Error(err, fmt.Sprintf("[updateAddonStatus] retry updating %s/%s status coz objects has been modified", addon.Namespace, addon.Name))
			c.logger.WithValues("[updateAddonStatus]", fmt.Sprintf(" retry updating %s/%s status coz objects has been modified", addon.Namespace, addon.Name))
			if err := c.updateAddonStatus(ctx, addon); err != nil {
				//c.logger.Error(err, fmt.Sprintf("[updateAddonStatus] failed retry updating %s/%s status %#v", addon.Namespace, addon.Name, err))
				c.logger.WithValues("[updateAddonStatus] ", fmt.Sprintf("failed retry updating %s/%s status %#v", addon.Namespace, addon.Name, err))
			}
		default:
			c.logger.Error(err, fmt.Sprintf("[updateAddonStatus] failed updating %s/%s status ", addon.Namespace, addon.Name))
			panic(err)
		}
	}
	c.logger.WithValues("[updateAddonStatus]", fmt.Sprintf("add %s/%s into cache.", addon.Namespace, addon.Name))
	c.addAddonToCache(addon)
	c.logger.WithValues("[updateAddonStatus]", fmt.Sprintf("add %s/%s successfully.", addon.Namespace, addon.Name))
	return nil
}

// add or remove complete label according to new instance
func (c *Controller) mergeLabels(old, new map[string]string, merged map[string]string) {

	needAddComplete := false
	if _, ok := new[addonapiv1.AddonCompleteLabel]; ok {
		needAddComplete = true
	}

	if needAddComplete {
		if old != nil {
			if _, ok := old[addonapiv1.AddonCompleteLabel]; !ok {
				c.logger.Info("mergeLabels add complete label.")
				old[addonapiv1.AddonCompleteLabel] = addonapiv1.AddonCompleteTrueKey
			}

			for k, v := range old {
				merged[k] = v
			}
			return
		}
		merged[addonapiv1.AddonCompleteLabel] = addonapiv1.AddonCompleteTrueKey
		return
	}

	if _, ok := old[addonapiv1.AddonCompleteLabel]; ok {
		c.logger.Info("mergeLabels remove complete label.")
		delete(old, addonapiv1.AddonCompleteLabel)
	}
	for k, v := range old {
		merged[k] = v
	}
}

// add or remove addon finalizer according to new instance
func (c *Controller) mergeFinalizer(old, new []string) []string {
	tmpMap := make(map[string]int)
	for _, f := range old {
		tmpMap[f] = 1
	}

	addFinalize := false
	for _, f := range new {
		if f == addonapiv1.FinalizerName {
			// should add finalizer
			addFinalize = true
			break
		}
	}

	if addFinalize {
		tmpMap[addonapiv1.FinalizerName] = 1
	} else {
		_, ok := tmpMap[addonapiv1.FinalizerName]
		if ok {
			delete(tmpMap, addonapiv1.FinalizerName)
		}
	}

	res := []string{}
	for k := range tmpMap {
		res = append(res, k)
	}

	c.logger.WithValues("mergeFinalizer", fmt.Sprintf("mergeFinalizer after remove addon finalizer %#v", res))
	return res
}

func (c *Controller) updateAddon(ctx context.Context, updated *addonv1.Addon) error {
	var errs []error
	latest := &addonv1.Addon{}
	//latest, err := c.addoncli.AddonmgrV1alpha1().Addons(updated.Namespace).Get(ctx, updated.Name, metav1.GetOptions{})

	err := c.client.Get(ctx, types.NamespacedName{Namespace: updated.Namespace, Name: updated.Name}, latest)
	if err != nil || latest == nil {
		msg := fmt.Sprintf("[updateAddon] failed getting %s err %#v", updated.Name, err)
		c.logger.Error(err, msg)
		panic(err)
	} else {
		if reflect.DeepEqual(updated, latest) {
			c.logger.WithValues("[updateAddon]", fmt.Sprintf(" latest and updated %s/%s is the same, skip", updated.Namespace, updated.Name))
			return nil

		}
		// update object metata only
		updating := latest.DeepCopy()
		updating.Finalizers = c.mergeFinalizer(latest.Finalizers, updated.Finalizers)
		updating.ObjectMeta.Labels = map[string]string{}
		c.mergeLabels(latest.GetLabels(), updated.GetLabels(), updating.ObjectMeta.Labels)

		err := c.client.Update(ctx, updating, &client.UpdateOptions{})
		if err != nil {
			switch {
			case errors.IsNotFound(err):
				msg := fmt.Sprintf("[updateAddon] Addon %s/%s is not found. %v", updated.Namespace, updated.Name, err)
				c.logger.Error(err, msg)
				return err
			case strings.Contains(err.Error(), "the object has been modified"):
				errs = append(errs, err)
				//c.logger.Error(err, fmt.Sprintf("[updateAddon] retry updating object metadata %s/%s coz objects has been modified", updated.Namespace, updated.Name))
				c.logger.Info(fmt.Sprintf("[updateAddon] retry updating object metadata %s/%s coz objects has been modified", updated.Namespace, updated.Name))
				if err := c.updateAddon(ctx, updated); err != nil {
					//c.logger.Error(err, fmt.Sprintf("[updateAddon] retry updating %s/%s, coz err %#v", updated.Namespace, updated.Name, err))
					c.logger.Info(fmt.Sprintf("[updateAddon] retry updating %s/%s, coz err %#v", updated.Namespace, updated.Name, err))
				}
			default:
				c.logger.Error(err, fmt.Sprintf("[updateAddon] failed  %s/%s", updated.Namespace, updated.Name))
				return err
			}
		}
		err = c.updateAddonStatus(ctx, updated)
		if err != nil {
			c.logger.Error(err, fmt.Sprintf("[updateAddon] failed updating %s/%s status", updated.Namespace, updated.Name))
			errs = append(errs, err)
		}
	}

	if len(errs) == 0 {
		c.logger.WithValues("[updateAddon]", fmt.Sprintf("%s/%s succeed.", updated.Namespace, updated.Name))
		return nil
	}
	err = fmt.Errorf("%v", errs)
	c.logger.Error(err, fmt.Sprintf("[updateAddon] %s/%s failed.", updated.Namespace, updated.Name))
	return err
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
	c.logger.Info("updateAddonStatusResources %s resource %s", key, resource)

	updating, err := c.getExistingAddon(ctx, key)
	if err != nil || updating == nil {
		return err
	}

	c.logger.Info("updateAddonStatusResources  ", updating.Namespace, "/", updating.Name, " new resources -- ", resource, " existing resources -- ", updating.Status.Resources)
	newResources := []addonv1.ObjectStatus{resource}
	updating.Status.Resources = c.mergeResources(newResources, updating.Status.Resources)

	var errs []error
	if _, err = c.addoncli.AddonmgrV1alpha1().Addons(updating.Namespace).UpdateStatus(ctx, updating,
		metav1.UpdateOptions{}); err != nil {
		switch {
		case errors.IsNotFound(err):
			return err
		case strings.Contains(err.Error(), "the object has been modified"):
			c.logger.Error(err, "[updateAddonStatusResources] retry %s coz the object has been modified", resource)
			if err := c.updateAddonStatusResources(ctx, key, resource); err != nil {
				errs = append(errs, fmt.Errorf("[updateAddonStatusResources] failed to update addon %s/%s resources: %w", updating.Namespace,
					updating.Name, err))
			}
		default:
			errs = append(errs, fmt.Errorf("[updateAddonStatusResources] default failed to update addon %s/%s resources: %w", updating.Namespace,
				updating.Name, err))
		}
	}

	if len(errs) == 0 {
		c.logger.Info("updateAddonStatusResources %s resource %s successfully", key, resource)
		return nil
	}

	c.logger.Error(err, "updateAddonStatusResources failed processing %s resources %#v", key, errs)
	return fmt.Errorf("updateAddonStatusResources failed processing %s resources %#v", key, errs)
}

func (c *Controller) getExistingAddon(ctx context.Context, key string) (*addonv1.Addon, error) {
	info := strings.Split(key, "/")

	namespacedname := types.NamespacedName{
		Namespace: info[0],
		Name:      info[1],
	}
	updating := &addonv1.Addon{}
	err := c.client.Get(ctx, namespacedname, updating)
	if err != nil {
		//c.logger.WithValues("[getExistingAddon]", fmt.Sprintf(" failed getting addon %s from controller-runtime err %#v", key, err))
		updating, err := c.addoncli.AddonmgrV1alpha1().Addons(info[0]).Get(ctx, info[1], metav1.GetOptions{})
		if err != nil || updating == nil {
			//c.logger.WithValues("[getExistingAddon]", fmt.Sprintf("failed getting addon %s/%s through api. err %#v", info[0], info[1], err))

			item, existing, err := c.addoninformer.GetIndexer().GetByKey(key)
			if err != nil || !existing {
				//c.logger.WithValues("[getExistingAddon]", fmt.Sprintf("failed getting addon %s through informer. err %#v", key, err))

				_, name := c.namespacenameFromKey(key)
				un, err := c.dynCli.Resource(schema.GroupVersionResource{
					Group:    addonapiv1.Group,
					Version:  "v1alpha1",
					Resource: addonapiv1.AddonPlural,
				}).Get(ctx, name, metav1.GetOptions{})
				if err == nil && un != nil {
					updating, err := common.FromUnstructured(un)
					if err == nil || updating == nil {
						//c.logger.WithValues("[getExistingAddon]", fmt.Sprintf("[getExistingAddon] failed converting to addon %s from unstructure err %#v", key, err))
						return nil, fmt.Errorf("[getExistingAddon] failed converting to addon %s from unstructure err %#v", key, err)
					}
					//c.logger.WithValues("[getExistingAddon]", fmt.Sprintf("getting addon %s through unstructure.", key))
				} else {
					//c.logger.WithValues("[getExistingAddon]", fmt.Sprintf(" failed getting %s from unstructure err %#v", key, err))
					return nil, fmt.Errorf("[getExistingAddon] failed getting %s/%s from unstructure %#v", info[0], info[1], err)
				}
			}
			c.logger.WithValues("[getExistingAddon]", fmt.Sprintf("getting addon %s from informer.", key))
			updating, err := common.FromUnstructured(item.(*unstructured.Unstructured))
			if err != nil || updating == nil {
				//c.logger.Error(err, "[getExistingAddon] failed converting to addon %s from informer unstructure err %#v", key, err)
				return nil, fmt.Errorf("[getExistingAddon] failed converting to addon %s from informer unstructure err %#v", key, err)
			}
		}
	} else {
		c.logger.WithValues("[getExistingAddon]", fmt.Sprintf(" getting addon %s from controller-runntime client.", key))
	}
	return updating, nil
}
