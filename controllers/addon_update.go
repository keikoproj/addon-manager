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

func (c *Controller) getAddon(ctx context.Context, key string) (*addonv1.Addon, error) {
	// optimize me trigger addon informer events
	obj, exists, err := c.informer.GetIndexer().GetByKey(key)
	if err != nil || !exists {
		c.logger.Warnf("[getAddon] failed getting addon(key) %s err %#v", key, err)
		info := strings.Split(key, "/")
		ns, name := info[0], info[1]
		addon, err := c.addoncli.AddonmgrV1alpha1().Addons(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil || addon == nil {
			msg := fmt.Sprintf("[getAddon] failed getting addon(namespace/name) %s, err %v", key, err)
			c.logger.Error(msg)
			return nil, fmt.Errorf(msg)
		}
		return addon, nil
	}
	latest, err := common.FromUnstructured(obj.(*unstructured.Unstructured))
	if err != nil {
		msg := fmt.Sprintf("failed converting %s un to addon,  err %v", key, err)
		c.logger.Error(msg)
		return nil, fmt.Errorf(msg)
	}
	return latest, nil
}

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
		if prevStatus.Lifecycle.Installed.Completed() || prevStatus.Lifecycle.Installed.Deleting() {
			c.logger.Info("addon", namespace, "/", name, " installation completed previously. the deletion wf succeeded remove the addon finalizer for cleanup")
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
		}
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

func (c *Controller) updateAddonStatus(ctx context.Context, updating *addonv1.Addon) error {
	key := fmt.Sprintf("%s/%s", updating.Namespace, updating.Name)
	latest, err := c.getAddon(ctx, key)
	if err != nil {
		c.logger.Error("[updateAddonStatus] failed getting addon key", key, " err ", key, err)
		return err
	}
	if reflect.DeepEqual(latest.Status, updating.Status) {
		c.logger.Info("addon ", key, " status is the same. skip updating.")
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
			latest, err := c.getAddon(ctx, key)
			if err != nil {
				c.logger.Error("[updateAddonStatus] failed getting addon key", key, " err ", key, err)
				return err
			}
			newUpdating := latest.DeepCopy()
			newUpdating.Status = updating.Status
			if latest.Status.Lifecycle.Installed != addonv1.Deleting { // edge case: latest is in an error status, skip retry
				c.logger.Info("retry updating ", newUpdating.Namespace, "/", newUpdating.Name, " object coz objects has been modified")
				if err := c.updateAddonStatus(ctx, newUpdating); err != nil {
					c.logger.Error("failed retry updating ", updating.Namespace, updating.Name, " lifecycle status ", err)
					// workaround
					return nil
				}
			}
		default:
			c.logger.Error("failed updating ", updating.Namespace, updating.Name, " status err ", err)
			return err
		}
	}
	msg := fmt.Sprintf("successfully updated addon %s status", key)
	c.logger.Info(msg)
	return nil
}

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

func (c *Controller) updateAddonStatusOnly(ctx context.Context, updated *addonv1.Addon) error {
	_, err := c.addoncli.AddonmgrV1alpha1().Addons(updated.Namespace).UpdateStatus(ctx, updated,
		metav1.UpdateOptions{})
	if err != nil {
		msg := fmt.Sprintf("failed updating addon %s/%s status only. err %v", updated.Namespace, updated.Name, err)
		c.logger.Error(msg)
		return fmt.Errorf(msg)
	}
	return nil
}

func (c *Controller) updateAddonStatusResources(ctx context.Context, key string, resource addonv1.ObjectStatus) error {
	addon, err := c.getAddon(ctx, key)
	if err != nil {
		msg := fmt.Sprintf("failed finding addon %s err %v.", key, err)
		c.logger.Error(msg)
		return fmt.Errorf(msg)
	}

	// fix me : if the addon already completed, ignore resource update
	updating := addon.DeepCopy()

	c.logger.Info("existing resources --", addon.Status.Resources)
	newResources := []addonv1.ObjectStatus{resource}
	newResources = append(newResources, addon.Status.Resources...)
	updating.Status.Resources = newResources

	c.logger.Info("updated resources --", updating.Status.Resources)
	return c.updateAddonStatusOnly(ctx, addon)
}
