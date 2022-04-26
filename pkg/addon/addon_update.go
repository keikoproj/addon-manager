/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package addon

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"github.com/go-logr/logr"
	addonapiv1 "github.com/keikoproj/addon-manager/api/addon"
	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AddonUpdate struct {
	client       client.Client
	log          logr.Logger
	versionCache VersionCacheClient
}

func NewAddonUpdate(cli client.Client, log logr.Logger, versionCache VersionCacheClient) *AddonUpdate {
	return &AddonUpdate{
		client:       cli,
		log:          log,
		versionCache: versionCache,
	}
}

// update addon meta ojbect first, then update status
func (c *AddonUpdate) updateAddon(ctx context.Context, updated *addonv1.Addon) (*addonv1.Addon, error) {
	var errs []error

	latest := &addonv1.Addon{}
	err := c.client.Get(ctx, types.NamespacedName{Namespace: updated.Namespace, Name: updated.Name}, latest)
	if err != nil || latest == nil {
		return nil, err
	} else {

		if reflect.DeepEqual(updated, latest) {
			c.log.Info(fmt.Sprintf("[updateAddon] latest and updated %s/%s is the same, skip", updated.Namespace, updated.Name))
			return nil, nil

		}

		// update object metata only
		updating := latest.DeepCopy()
		updating.Finalizers = c.mergeFinalizer(latest.Finalizers, updated.Finalizers)
		updating.ObjectMeta.Labels = map[string]string{}

		err := c.client.Update(ctx, updating, &client.UpdateOptions{})
		if err != nil {
			switch {
			case errors.IsNotFound(err):
				msg := fmt.Sprintf("[updateAddon] Addon %s/%s is not found. %v", updated.Namespace, updated.Name, err)
				//c.logger.Error(msg)
				return nil, fmt.Errorf(msg)
			case strings.Contains(err.Error(), "the object has been modified"):
				errs = append(errs, err)
				c.log.Info(fmt.Sprintf("[updateAddon] retry updating object metadata %s/%s coz objects has been modified", updated.Namespace, updated.Name))
				if _, err := c.updateAddon(ctx, updated); err != nil {
					errs = append(errs, err)
				}
			default:
				return nil, err
			}
		}
		_, err = c.updateAddonStatus(ctx, updated)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return updated, nil
	}
	c.log.Error(err, fmt.Sprintf("[updateAddon] %s/%s failed.", updated.Namespace, updated.Name))
	return nil, fmt.Errorf("%v", errs)
}

func (c *AddonUpdate) getExistingAddon(ctx context.Context, key string) (*addonv1.Addon, error) {
	info := strings.Split(key, "/")
	updating := &addonv1.Addon{}
	err := c.client.Get(ctx, types.NamespacedName{Namespace: info[0], Name: info[1]}, updating)
	if err != nil || updating == nil {
		return nil, fmt.Errorf("[getExistingAddon] failed converting to addon %s from client err %#v", key, err)
	}
	return updating, nil
}

func (c *AddonUpdate) addAddonToCache(addon *addonv1.Addon) {
	var version = Version{
		Name:        addon.GetName(),
		Namespace:   addon.GetNamespace(),
		PackageSpec: addon.GetPackageSpec(),
		PkgPhase:    addon.GetInstallStatus(),
	}
	c.versionCache.AddVersion(version)
	c.log.WithValues("addAddonToCache", fmt.Sprintf("Adding %s/%s package %s version cache phase %s", addon.GetNamespace(), addon.GetName(), addon.GetPackageSpec(), version.PkgPhase))
}

// UpdateAddonStatusLifecycle
func (c *AddonUpdate) UpdateAddonStatusLifecycle(ctx context.Context, namespace, name string, lifecycle string, lifecyclestatus wfv1.WorkflowPhase) error {

	key := fmt.Sprintf("%s/%s", namespace, name)
	latest, err := c.getExistingAddon(ctx, key)
	if err != nil || latest == nil {
		return err
	}
	updating := latest.DeepCopy()
	prevStatus := latest.Status

	// addon being deletion, skip non-delete wf update
	if lifecycle != string(addonv1.Delete) &&
		prevStatus.Lifecycle.Installed == addonv1.Deleting {
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
	if lifecycle == string(addonv1.Prereqs) {
		newStatus.Lifecycle.Prereqs = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
		newStatus.Lifecycle.Installed = prevStatus.Lifecycle.Installed
		if newStatus.Lifecycle.Prereqs == addonv1.Failed {
			newStatus.Lifecycle.Installed = addonv1.Failed
		}
	} else if lifecycle == string(addonv1.Install) || lifecycle == string(addonv1.Delete) {
		newStatus.Lifecycle.Installed = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
		newStatus.Lifecycle.Prereqs = prevStatus.Lifecycle.Prereqs
		if addonv1.ApplicationAssemblyPhase(lifecyclestatus) == addonv1.Succeeded {
			newStatus.Reason = ""
		}

		// check whether need patch complete
		if lifecycle == string(addonv1.Install) && newStatus.Lifecycle.Installed.Completed() {
			updating.Status = newStatus
			if _, err := c.updateAddon(ctx, updating); err != nil {
				return err
			}
			return nil
		}
	}
	updating.Status = newStatus

	if lifecycle == string(addonv1.Delete) && addonv1.ApplicationAssemblyPhase(lifecyclestatus).Succeeded() {
		if prevStatus.Lifecycle.Installed.Completed() || prevStatus.Lifecycle.Installed.Deleting() {
			c.removeFinalizer(updating)
			if _, err := c.updateAddon(ctx, updating); err != nil {
				return err
			}
			c.removeFromCache(updating.Name)
			return nil
		}
	}

	var afterupdating *addonv1.Addon
	patchLabel := false
	if lifecycle == string(addonv1.Install) && addonv1.ApplicationAssemblyPhase(lifecyclestatus).Succeeded() {
		labels := updating.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		afterupdating, err = c.updateAddon(ctx, updating)
		if err != nil {
			return err
		}
		patchLabel = true
	}

	if patchLabel {
		updating = afterupdating.DeepCopy()
	}

	if reflect.DeepEqual(prevStatus, updating.Status) {
		return nil
	}

	_, err = c.updateAddonStatus(ctx, updating)
	if err != nil {
		return err
	}
	return nil

}

func (c *AddonUpdate) updateAddonStatus(ctx context.Context, addon *addonv1.Addon) (*addonv1.Addon, error) {
	latest := &addonv1.Addon{}
	err := c.client.Get(ctx, types.NamespacedName{Namespace: addon.Namespace, Name: addon.Name}, latest)
	if err != nil {
		msg := fmt.Sprintf("updateAddonStatus failed finding addon %s err %v.", addon.Name, err)
		return nil, fmt.Errorf(msg)
	}
	updating := latest.DeepCopy()
	if reflect.DeepEqual(updating.Status, addon.Status) {
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

	//updated, err := c.addoncli.AddonmgrV1alpha1().Addons(updating.Namespace).UpdateStatus(ctx, updating, metav1.UpdateOptions{})
	err = c.client.Status().Update(ctx, updating, &client.UpdateOptions{})
	if err != nil {
		switch {
		case errors.IsNotFound(err):
			msg := fmt.Sprintf("[updateAddonStatus] addon %s/%s is not found. %v", updating.Namespace, updating.Name, err)
			return nil, fmt.Errorf(msg)
		case strings.Contains(err.Error(), "the object has been modified"):
			if _, err := c.updateAddonStatus(ctx, addon); err != nil {
				c.log.Error(err, fmt.Sprintf("[updateAddonStatus] failed retry updating %s/%s status", updating.Namespace, updating.Name))
			}
		default:
			return nil, err
		}
	}
	c.addAddonToCache(updating)

	return updating, nil
}

// add or remove addon finalizer according to new instance
func (c *AddonUpdate) mergeFinalizer(old, new []string) []string {
	addFinalize := false
	for _, f := range new {
		if f == addonapiv1.FinalizerName {
			// should add finalizer
			addFinalize = true
			break
		}
	}

	if addFinalize {
		needappend := true
		for _, f := range old {
			if f == addonapiv1.FinalizerName {
				needappend = false
			}
		}
		if needappend {
			old = append(old, addonapiv1.FinalizerName)
		}
		return old
	}

	// otherwise, remove finalizer
	ret := []string{}
	for _, f := range old {
		if f == addonapiv1.FinalizerName {
			continue
		}
		ret = append(ret, f)
	}
	return ret
}

func (c *AddonUpdate) mergeResources(res1, res2 []addonv1.ObjectStatus) []addonv1.ObjectStatus {
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

func (c *AddonUpdate) removeFinalizer(addon *addonv1.Addon) {
	if common.ContainsString(addon.ObjectMeta.Finalizers, addonapiv1.FinalizerName) {
		addon.ObjectMeta.Finalizers = common.RemoveString(addon.ObjectMeta.Finalizers, addonapiv1.FinalizerName)
	}
}

func (c *AddonUpdate) removeFromCache(addonName string) {
	// Remove version from cache
	if ok, v := c.versionCache.HasVersionName(addonName); ok {
		c.versionCache.RemoveVersion(v.PkgName, v.PkgVersion)
	}
}
