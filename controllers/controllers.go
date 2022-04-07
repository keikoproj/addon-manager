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
)

func (c *AddonReconciler) removeFromCache(addonName string) {
	// Remove version from cache
	if ok, v := c.versionCache.HasVersionName(addonName); ok {
		c.versionCache.RemoveVersion(v.PkgName, v.PkgVersion)
	}
}

func (c *AddonReconciler) namespacenameFromKey(key string) (string, string) {
	info := strings.Split(key, "/")
	ns, name := info[0], info[1]
	return ns, name
}

func (c *AddonReconciler) updateAddonStatusLifecycle(ctx context.Context, namespace, name string, lifecycle string, lifecyclestatus wfv1.WorkflowPhase) error {
	//c.logger.Info(fmt.Sprintf("[updateAddonStatusLifecycle] updating addon %s/%s step %s status to %s", namespace, name, lifecycle, lifecyclestatus))

	key := fmt.Sprintf("%s/%s", namespace, name)
	latest, err := c.getExistingAddon(ctx, key)
	if err != nil || latest == nil {
		return err
	}
	updating := latest.DeepCopy()
	prevStatus := latest.Status
	if c.isAddonCompleted(updating) && prevStatus.Lifecycle.Installed != addonv1.Deleting {
		//c.logger.Info(fmt.Sprintf("[updateAddonStatusLifecycle] addon %s/%s completed, but not deleting. skip.", namespace, name))
		return nil
	}

	// addon being deletion, skip non-delete wf update
	if lifecycle != "delete" &&
		prevStatus.Lifecycle.Installed == addonv1.Deleting {
		//c.logger.Info(fmt.Sprintf("[updateAddonStatusLifecycle] %s/%s is being deleting. skip non-delete wf update.", namespace, name))
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
			//c.logger.Info("[updateAddonStatusLifecycle] %s/%s prereq failed. mark addon failure also", namespace, name)
		}
	} else if lifecycle == "install" || lifecycle == "delete" {
		newStatus.Lifecycle.Installed = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
		newStatus.Lifecycle.Prereqs = prevStatus.Lifecycle.Prereqs
		if addonv1.ApplicationAssemblyPhase(lifecyclestatus) == addonv1.Succeeded {
			newStatus.Reason = ""
		}

		// check whether need patch complete
		if lifecycle == "install" && newStatus.Lifecycle.Installed.Completed() {
			//c.logger.Info("[updateAddonStatusLifecycle] %s/%s completed. patch complete label.",
			//updating.Namespace, updating.Name)
			labels := updating.GetLabels()
			if labels == nil {
				labels = map[string]string{}
			}
			labels[addonapiv1.AddonCompleteLabel] = addonapiv1.AddonCompleteTrueKey
			updating.SetLabels(labels)

			updating.Status = newStatus
			if err := c.updateAddon(ctx, updating); err != nil {
				//c.logger.Error(err, "updateAddonStatusLifecycle %s/%s update complete failed.", namespace, name)
				return err
			}
			return nil
		}
	}
	updating.Status = newStatus

	if lifecycle == "delete" && addonv1.ApplicationAssemblyPhase(lifecyclestatus).Succeeded() {
		//c.logger.Info(fmt.Sprintf("addon %s/%s installation completed or addon being deleting. the deletion wf completed.", namespace, name))
		if prevStatus.Lifecycle.Installed.Completed() || prevStatus.Lifecycle.Installed.Deleting() {
			c.removeFinalizer(updating)
			if err := c.updateAddon(ctx, updating); err != nil {
				////c.logger.Error(err, "updateAddonStatusLifecycle failed updating ", updating.Namespace, updating.Name, " lifecycle status err ", err)
				return err
			}
			c.removeFromCache(updating.Name)
			return nil
		}
	}

	if lifecycle == "install" && addonv1.ApplicationAssemblyPhase(lifecyclestatus).Succeeded() {
		//c.logger.Info(fmt.Sprintf("addon %s/%s completed. patch complete label.", namespace, name))
		labels := updating.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[addonapiv1.AddonCompleteLabel] = addonapiv1.AddonCompleteTrueKey
		updating.SetLabels(labels)
		err = c.updateAddon(ctx, updating)
		if err != nil {
			////c.logger.Error(err, "updateAddonStatusLifecycle failed label addon %s/%s completed err %#v", updating.Namespace, updating.Name, err)
			return err
		}
	}

	if reflect.DeepEqual(prevStatus, updating.Status) {
		//msg := fmt.Sprintf("updateAddonStatusLifecycle addon %s/%s status the same. skip update.", updating.Namespace, updating.Name)
		//c.logger.Info(msg)
		return nil
	}

	err = c.updateAddonStatus(ctx, c.Log, updating)
	if err != nil {
		////c.logger.Error(err, "updateAddonStatusLifecycle failed updating ", updating.Namespace, "/", updating.Name, " status ", err)
		return err
	}
	//c.logger.Info(fmt.Sprintf("updateAddonStatusLifecycle successfully update addon %s/%s step %s status to %s", namespace, name, lifecycle, lifecyclestatus))
	return nil

}

func (c *AddonReconciler) removeFinalizer(addon *addonv1.Addon) {
	if common.ContainsString(addon.ObjectMeta.Finalizers, addonapiv1.FinalizerName) {
		addon.ObjectMeta.Finalizers = common.RemoveString(addon.ObjectMeta.Finalizers, addonapiv1.FinalizerName)
	}
}

func (c *AddonReconciler) unLabelComplete(addon *addonv1.Addon) {
	if _, ok := addon.GetLabels()[addonapiv1.AddonCompleteLabel]; ok {
		labels := addon.GetLabels()
		delete(labels, addonapiv1.AddonCompleteLabel)
		addon.SetLabels(labels)
	}
}

// add or remove complete label according to new instance
func (c *AddonReconciler) mergeLabels(old, new map[string]string, merged map[string]string) {

	needAddComplete := false
	if _, ok := new[addonapiv1.AddonCompleteLabel]; ok {
		needAddComplete = true
	}

	if needAddComplete {
		if old != nil {
			if _, ok := old[addonapiv1.AddonCompleteLabel]; !ok {
				//c.logger.Info("mergeLabels add complete label.")
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
		//c.logger.Info("mergeLabels remove complete label.")
		delete(old, addonapiv1.AddonCompleteLabel)
	}
	for k, v := range old {
		merged[k] = v
	}
}

// add or remove addon finalizer according to new instance
func (c *AddonReconciler) mergeFinalizer(old, new []string) []string {
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

	//c.logger.WithValues("mergeFinalizer", fmt.Sprintf("mergeFinalizer after remove addon finalizer %#v", res))
	return res
}

func (c *AddonReconciler) updateAddon(ctx context.Context, updated *addonv1.Addon) error {
	//c.logger.Info(fmt.Sprintf("updateAddon %s/%s", updated.Namespace, updated.Name))
	latest, err := c.addoncli.AddonmgrV1alpha1().Addons(updated.Namespace).Get(ctx, updated.Name, metav1.GetOptions{})
	if err != nil || latest == nil {
		// msg := fmt.Sprintf("[updateAddon] failed getting %s err %#v", updated.Name, err)
		// c.logger.Error(err, msg)
		return err
	} else {
		if reflect.DeepEqual(updated, latest) {
			//c.logger.WithValues("[updateAddon]", fmt.Sprintf(" latest and updated %s/%s is the same, skip", updated.Namespace, updated.Name))
			return nil

		}
		// update object metata only
		updating := latest.DeepCopy()
		updating.Finalizers = c.mergeFinalizer(latest.Finalizers, updated.Finalizers)
		updating.ObjectMeta.Labels = map[string]string{}
		c.mergeLabels(latest.GetLabels(), updated.GetLabels(), updating.ObjectMeta.Labels)

		_, err := c.addoncli.AddonmgrV1alpha1().Addons(updated.Namespace).Update(ctx, updating, metav1.UpdateOptions{})
		if err != nil {
			switch {
			case errors.IsNotFound(err):
				//msg := fmt.Sprintf("[updateAddon] Addon %s/%s is not found. %v", updated.Namespace, updated.Name, err)
				//c.logger.Error(err, msg)
				return err
			case strings.Contains(err.Error(), "the object has been modified"):
				//errs = append(errs, err)
				//c.logger.Error(err, fmt.Sprintf("[updateAddon] retry updating object metadata %s/%s coz objects has been modified", updated.Namespace, updated.Name))
				//c.logger.Info(fmt.Sprintf("[updateAddon] retry updating object metadata %s/%s coz objects has been modified", updated.Namespace, updated.Name))
				if err := c.updateAddon(ctx, updated); err != nil {
					//c.logger.Error(err, fmt.Sprintf("[updateAddon] retry updating %s/%s, coz err %#v", updated.Namespace, updated.Name, err))
					//c.logger.Info(fmt.Sprintf("[updateAddon] retry updating %s/%s, coz err %#v", updated.Namespace, updated.Name, err))
				}
			default:
				//c.logger.Error(err, fmt.Sprintf("[updateAddon] failed  %s/%s", updated.Namespace, updated.Name))
				return err
			}
		}
		err = c.updateAddonStatus(ctx, c.Log, updated)
		if err != nil {
			//c.logger.Error(err, fmt.Sprintf("[updateAddon] failed updating %s/%s status", updated.Namespace, updated.Name))
			return err
		}
	}

	//c.logger.Info(fmt.Sprintf("[updateAddon] %s/%s succeed.", updated.Namespace, updated.Name))
	return nil
}
