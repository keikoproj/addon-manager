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
	"sync"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

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
	statusMap    map[string]*sync.Mutex
}

func NewAddonUpdater(recorder record.EventRecorder, cli client.Client, versionCache VersionCacheClient, logger logr.Logger) *AddonUpdater {
	return &AddonUpdater{
		client:       cli,
		versionCache: versionCache,
		recorder:     recorder,
		statusMap:    make(map[string]*sync.Mutex),
		log:          logger.WithName("addon-updater"),
	}
}

func (c *AddonUpdater) UpdateStatus(ctx context.Context, log logr.Logger, addon *addonmgrv1alpha1.Addon) error {
	addonName := types.NamespacedName{Name: addon.Name, Namespace: addon.Namespace}
	m := c.getStatusMutex(addonName.Name)
	m.Lock()
	defer m.Unlock()

	// guardFired signals that the race guard blocked the write; used after the loop
	// to skip the cache update and emit the requeue error instead of the success log.
	guardFired := false
	var guardAPIInstalled, guardAPIPrereqs addonmgrv1alpha1.ApplicationAssemblyPhase

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get the latest version of Addon before attempting update
		currentAddon := &addonmgrv1alpha1.Addon{}
		err := c.client.Get(ctx, addonName, currentAddon)
		if err != nil {
			return err
		}

		// Race guard: skip the write if the API already holds a terminal (Completed) status
		// and the incoming write is non-terminal (Running — Pending or ValidationFailed) for
		// the same spec cycle (same checksum). A different checksum means a new install cycle
		// triggered by a spec change and is allowed through.
		// The version cache is not updated from the stale incoming addon when the guard fires.
		//
		// Known gap: terminal-over-terminal (e.g. stale Failed over Succeeded) is not guarded;
		// blocking that requires a stronger discriminator. See: #285
		if currentAddon.Status.Lifecycle.Installed.Completed() &&
			addon.Status.Lifecycle.Installed.Running() &&
			currentAddon.Status.Checksum == addon.Status.Checksum {
			log.Info("skipping non-terminal write: addon already reached a terminal status and spec is unchanged",
				"addon", addonName,
				"current_status", currentAddon.Status.Lifecycle.Installed,
				"skipped_status", addon.Status.Lifecycle.Installed)
			guardFired = true
			guardAPIInstalled = currentAddon.Status.Lifecycle.Installed
			guardAPIPrereqs = currentAddon.Status.Lifecycle.Prereqs
			return nil
		}

		addon.Status.DeepCopyInto(&currentAddon.Status)
		return c.client.Status().Update(ctx, currentAddon)
	})
	if err != nil {
		log.Error(err, "Addon status could not be updated.")
		c.recorder.Event(addon, "Warning", "Failed", fmt.Sprintf("Addon %s/%s status could not be updated. %v", addon.Namespace, addon.Name, err))
		return err
	}

	if guardFired {
		log.Info("race guard fired: stale non-terminal write blocked over terminal API state — requeuing for fresh reconcile",
			"addon", addonName,
			"api_prereqs_status", guardAPIPrereqs,
			"api_installed_status", guardAPIInstalled,
			"skipped_incoming_installed_status", addon.GetInstallStatus())
		// Return an error to trigger controller-runtime's exponential-backoff requeue.
		// The next reconcile re-fetches current API state and processes it correctly.
		return fmt.Errorf("race guard: stale %q write over terminal %q API state blocked for addon %s/%s (checksum: %s); requeuing with fresh state",
			addon.GetInstallStatus(), guardAPIInstalled, addon.Namespace, addon.Name, addon.Status.Checksum)
	}

	log.Info("successfully updated addon statuses", "prereqs_status", addon.GetPrereqStatus(), "installed_status", addon.GetInstallStatus())

	// Always update the version cache
	c.addAddonToCache(log, addon)

	return nil
}

func (c *AddonUpdater) getStatusMutex(addonName string) *sync.Mutex {
	m, ok := c.statusMap[addonName]
	if !ok {
		m = &sync.Mutex{}
		c.statusMap[addonName] = m
	}
	return m
}

func (c *AddonUpdater) removeStatusWaitGroup(addonName string) {
	delete(c.statusMap, addonName)
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

// UpdateAddonStatusLifecycleFromWorkflow updates the status of the addon
func (c *AddonUpdater) UpdateAddonStatusLifecycleFromWorkflow(ctx context.Context, namespace, addonName string, wf *wfv1.Workflow) error {
	existingAddon, err := c.getExistingAddon(ctx, namespace, addonName)
	if err != nil {
		return err
	}

	if existingAddon.Status.Lifecycle.Installed.Completed() {
		// If the addon is already installed, we don't want to update the status
		return nil
	}

	checksum, lifecycle, err := common.ExtractChecksumAndLifecycleStep(wf.GetName())
	if err != nil {
		return err
	}

	if existingAddon.GetFormattedWorkflowName(lifecycle) != wf.GetName() {
		return nil
	}

	if existingAddon.CalculateChecksum() != checksum {
		return nil
	}

	phase := common.ConvertWorkflowPhaseToAddonPhase(lifecycle, wf.Status.Phase)
	reason := ""

	if phase == "" {
		return nil
	}

	if phase.Failed() {
		reason = wf.Status.Message
	}

	if err := existingAddon.SetStatusByLifecyleStep(lifecycle, phase, reason); err != nil {
		return fmt.Errorf("failed to update prereqs status. %w", err)
	}

	return c.UpdateStatus(ctx, c.log, existingAddon)
}

func (c *AddonUpdater) RemoveFromCache(addonName string) {
	// Remove version from cache
	if ok, v := c.versionCache.HasVersionName(addonName); ok {
		c.versionCache.RemoveVersion(v.PkgName, v.PkgVersion)
	}
	// Remove addon from waitgroup map
	c.removeStatusWaitGroup(addonName)
}
