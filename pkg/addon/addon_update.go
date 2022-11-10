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
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sync"

	"github.com/go-logr/logr"
	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AddonUpdater struct {
	client       client.Client
	log          logr.Logger
	versionCache VersionCacheClient
	recorder     record.EventRecorder
	statusWGMap  map[string]*sync.WaitGroup
}

func NewAddonUpdater(mgr manager.Manager, cli client.Client, versionCache VersionCacheClient) *AddonUpdater {
	return &AddonUpdater{
		client:       cli,
		versionCache: versionCache,
		recorder:     mgr.GetEventRecorderFor("addons"),
		statusWGMap:  make(map[string]*sync.WaitGroup),
	}
}

func (c *AddonUpdater) UpdateStatus(ctx context.Context, log logr.Logger, addon *addonmgrv1alpha1.Addon) error {
	addonName := types.NamespacedName{Name: addon.Name, Namespace: addon.Namespace}
	wg := c.getStatusWaitGroup(addonName.String())
	// Wait to process addon updates until we have finished updating same addon
	wg.Wait()
	wg.Add(1)
	defer wg.Done()
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get the latest version of Addon before attempting update
		currentAddon := &addonmgrv1alpha1.Addon{}
		err := c.client.Get(ctx, addonName, currentAddon)
		if err != nil {
			return err
		}
		addon.Status.DeepCopyInto(&currentAddon.Status)
		return c.client.Status().Update(ctx, currentAddon, &client.UpdateOptions{})
	})
	if err != nil {
		log.Error(err, "Addon status could not be updated.")
		c.recorder.Event(addon, "Warning", "Failed", fmt.Sprintf("Addon %s/%s status could not be updated. %v", addon.Namespace, addon.Name, err))
		return err
	}

	// Always update the version cache
	c.addAddonToCache(log, addon)

	return nil
}

func (c *AddonUpdater) getStatusWaitGroup(addonName string) *sync.WaitGroup {
	wg, ok := c.statusWGMap[addonName]
	if !ok {
		wg = &sync.WaitGroup{}
		c.statusWGMap[addonName] = wg
	}
	return wg
}

func (c *AddonUpdater) removeStatusWaitGroup(addonName string) {
	delete(c.statusWGMap, addonName)
}

func (c *AddonUpdater) getExistingAddon(ctx context.Context, namespace, name string) (*addonmgrv1alpha1.Addon, error) {
	addonName := types.NamespacedName{Name: name, Namespace: namespace}
	currentAddon := &addonmgrv1alpha1.Addon{}
	err := c.client.Get(ctx, addonName, currentAddon)
	if err != nil {
		return nil, err
	}
	return currentAddon, nil
}

func (c *AddonUpdater) addAddonToCache(log logr.Logger, addon *addonmgrv1alpha1.Addon) {
	var version = Version{
		Name:        addon.GetName(),
		Namespace:   addon.GetNamespace(),
		PackageSpec: addon.GetPackageSpec(),
		PkgPhase:    addon.GetInstallStatus(),
	}
	c.versionCache.AddVersion(version)
	log.Info("Adding version cache", "phase", version.PkgPhase)
}

// UpdateAddonStatusLifecycle updates the status of the addon
func (c *AddonUpdater) UpdateAddonStatusLifecycle(ctx context.Context, namespace, name string, lifecycle addonmgrv1alpha1.LifecycleStep, phase addonmgrv1alpha1.ApplicationAssemblyPhase, reasons ...string) error {
	existingAddon, err := c.getExistingAddon(ctx, namespace, name)
	if err != nil {
		return err
	}

	if lifecycle == addonmgrv1alpha1.Prereqs {
		err := existingAddon.SetPrereqStatus(phase, reasons...)
		if err != nil {
			return fmt.Errorf("failed to update prereqs status. %w", err)
		}
	} else {
		existingAddon.SetInstallStatus(phase, reasons...)
	}

	return c.UpdateStatus(ctx, c.log, existingAddon)
	//key := fmt.Sprintf("%s/%s", namespace, name)
	//latest, err := c.getExistingAddon(ctx, key)
	//if err != nil || latest == nil {
	//	return err
	//}
	//updating := latest.DeepCopy()
	//prevStatus := latest.Status
	//
	//// addon being deletion, skip non-delete wf update
	//if lifecycle != string(addonmgrv1alpha1.Delete) &&
	//	prevStatus.Lifecycle.Installed == addonmgrv1alpha1.Deleting {
	//	return nil
	//}
	//
	//newStatus := addonmgrv1alpha1.AddonStatus{
	//	Lifecycle: addonmgrv1alpha1.AddonStatusLifecycle{},
	//	Resources: []addonmgrv1alpha1.ObjectStatus{},
	//}
	//newStatus.Reason = prevStatus.Reason
	//newStatus.Resources = append(newStatus.Resources, prevStatus.Resources...)
	//newStatus.Checksum = prevStatus.Checksum
	//newStatus.StartTime = prevStatus.StartTime
	//if lifecycle == string(addonmgrv1alpha1.Prereqs) {
	//	newStatus.Lifecycle.Prereqs = addonmgrv1alpha1.ApplicationAssemblyPhase(lifecyclestatus)
	//	newStatus.Lifecycle.Installed = prevStatus.Lifecycle.Installed
	//	if newStatus.Lifecycle.Prereqs == addonmgrv1alpha1.Failed {
	//		newStatus.Lifecycle.Installed = addonmgrv1alpha1.Failed
	//	}
	//} else if lifecycle == string(addonmgrv1alpha1.Install) || lifecycle == string(addonmgrv1alpha1.Delete) {
	//	newStatus.Lifecycle.Installed = addonmgrv1alpha1.ApplicationAssemblyPhase(lifecyclestatus)
	//	newStatus.Lifecycle.Prereqs = prevStatus.Lifecycle.Prereqs
	//	if addonmgrv1alpha1.ApplicationAssemblyPhase(lifecyclestatus) == addonmgrv1alpha1.Succeeded {
	//		newStatus.Reason = ""
	//	}
	//
	//	// check whether need patch complete
	//	if lifecycle == string(addonmgrv1alpha1.Install) && newStatus.Lifecycle.Installed.Completed() {
	//		updating.Status = newStatus
	//		if _, err := c.updateAddon(ctx, updating); err != nil {
	//			return err
	//		}
	//		return nil
	//	}
	//}
	//updating.Status = newStatus
	//
	//if lifecycle == string(addonmgrv1alpha1.Delete) && addonmgrv1alpha1.ApplicationAssemblyPhase(lifecyclestatus).Succeeded() {
	//	if prevStatus.Lifecycle.Installed.Completed() || prevStatus.Lifecycle.Installed.Deleting() {
	//		c.removeFinalizer(updating)
	//		if _, err := c.updateAddon(ctx, updating); err != nil {
	//			return err
	//		}
	//		c.RemoveFromCache(updating.Name)
	//		return nil
	//	}
	//}
	//
	//var afterupdating *addonmgrv1alpha1.Addon
	//patchLabel := false
	//if lifecycle == string(addonmgrv1alpha1.Install) && addonmgrv1alpha1.ApplicationAssemblyPhase(lifecyclestatus).Succeeded() {
	//	labels := updating.GetLabels()
	//	if labels == nil {
	//		labels = map[string]string{}
	//	}
	//	afterupdating, err = c.updateAddon(ctx, updating)
	//	if err != nil {
	//		return err
	//	}
	//	patchLabel = true
	//}
	//
	//if patchLabel {
	//	updating = afterupdating.DeepCopy()
	//}
	//
	//if reflect.DeepEqual(prevStatus, updating.Status) {
	//	return nil
	//}
	//
	//_, err = c.updateAddonStatus(ctx, updating)
	//if err != nil {
	//	return err
	//}
	//return nil
}

func (c *AddonUpdater) RemoveFromCache(addonName string) {
	// Remove version from cache
	if ok, v := c.versionCache.HasVersionName(addonName); ok {
		c.versionCache.RemoveVersion(v.PkgName, v.PkgVersion)
	}
	// Remove addon from waitgroup map
	c.removeStatusWaitGroup(addonName)
}
