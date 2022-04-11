package controllers

import (
	"context"
	"fmt"
	"strings"

	//addonapiv1 "github.com/keikoproj/addon-manager/api/addon"
	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	//"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	AddonCompleteLabel   = "addons.addonmgr.keikoproj.io/completed"
	AddonCompleteTrueKey = "true"

	Group         string = "addonmgr.keikoproj.io"
	Version       string = "v1alpha1"
	APIVersion    string = Group + "/" + Version
	AddonKind     string = "Addon"
	AddonSingular string = "addon"
	AddonPlural   string = "addons"
)

// if labeled "addons.addonmgr.keikoproj.io/completed"
func (c *AddonReconciler) isAddonCompleted(addon *addonv1.Addon) bool {
	_, ok := addon.Labels[AddonCompleteLabel]
	return ok && addon.Labels[AddonCompleteLabel] == AddonCompleteTrueKey
}

func (c *AddonReconciler) resetAddonStatus(ctx context.Context, addon *addonv1.Addon) error {
	//c.logger.Info(fmt.Sprintf("resetAddonStatus %s/%s", addon.Namespace, addon.Name))
	addon.Status.StartTime = common.GetCurretTimestamp()
	addon.Status.Lifecycle.Prereqs = ""
	addon.Status.Lifecycle.Installed = ""
	addon.Status.Reason = ""
	addon.Status.Resources = []addonv1.ObjectStatus{}
	addon.Status.Checksum = addon.CalculateChecksum()

	// remove complete label also
	labels := addon.GetLabels()
	if labels != nil {
		delete(labels, string(AddonCompleteLabel))
	}
	addon.SetLabels(labels)

	err := c.updateAddon(ctx, addon)
	if err != nil {
		//c.logger.Error(err, "failed resetting status", addon.Namespace, addon.Name)
		return err
	}

	//c.logger.Info(fmt.Sprintf("successfully reset addon %s status", addon.Name))
	return nil
}

func (c *AddonReconciler) mergeResources(res1, res2 []addonv1.ObjectStatus) []addonv1.ObjectStatus {
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

func (c *AddonReconciler) updateAddonStatusResources(ctx context.Context, key string, resource addonv1.ObjectStatus) error {
	//c.logger.Info(fmt.Sprintf("updateAddonStatusResources %s resource %s", key, resource))

	updating, err := c.getExistingAddon(ctx, key)
	if err != nil || updating == nil {
		return err
	}

	//c.logger.Info("updateAddonStatusResources  ", "--", updating.Namespace, "--", updating.Name, " new resources -- ", resource, " existing resources -- ", updating.Status.Resources)
	newResources := []addonv1.ObjectStatus{resource}
	updating.Status.Resources = c.mergeResources(newResources, updating.Status.Resources)

	var errs []error
	if _, err = c.addoncli.AddonmgrV1alpha1().Addons(updating.Namespace).UpdateStatus(ctx, updating,
		metav1.UpdateOptions{}); err != nil {
		switch {
		case errors.IsNotFound(err):
			return err
		case strings.Contains(err.Error(), "the object has been modified"):
			//c.logger.Error(err, fmt.Sprintf("[updateAddonStatusResources] retry %s coz the object has been modified", resource))
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
		//c.logger.Info("updateAddonStatusResources %s resource %s successfully", key, resource)
		return nil
	}

	//c.logger.Error(err, "updateAddonStatusResources failed processing %s resources %#v", key, errs)
	return fmt.Errorf("updateAddonStatusResources failed processing %s resources %#v", key, errs)
}

func (c *AddonReconciler) getExistingAddon(ctx context.Context, key string) (*addonv1.Addon, error) {
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
				un, err := c.dynClient.Resource(schema.GroupVersionResource{
					Group:    Group,
					Version:  "v1alpha1",
					Resource: AddonPlural,
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
			//c.logger.WithValues("[getExistingAddon]", fmt.Sprintf("getting addon %s from informer.", key))
			updating, err := common.FromUnstructured(item.(*unstructured.Unstructured))
			if err != nil || updating == nil {
				//c.logger.Error(err, "[getExistingAddon] failed converting to addon %s from informer unstructure err %#v", key, err)
				return nil, fmt.Errorf("[getExistingAddon] failed converting to addon %s from informer unstructure err %#v", key, err)
			}
		}
	} else {
		//c.logger.WithValues("[getExistingAddon]", fmt.Sprintf(" getting addon %s from controller-runntime client.", key))
	}
	return updating, nil
}
