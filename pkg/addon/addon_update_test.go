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
	"testing"
	"time"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/workflows"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme   = runtime.NewScheme()
	fakeRcdr = record.NewBroadcasterForTests(1*time.Second).NewRecorder(scheme, v1.EventSource{Component: "addons"})
	ctx      = context.TODO()
)

func init() {
	_ = addonmgrv1alpha1.AddToScheme(scheme)
	_ = wfv1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
}

func TestUpdateAddonStatusLifecycleFromWorkflow(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testNamespace := "default"
	testAddonName := "test-addon"

	testAddon := &addonmgrv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAddonName,
			Namespace: testNamespace,
		},
		Spec: addonmgrv1alpha1.AddonSpec{
			PackageSpec: addonmgrv1alpha1.PackageSpec{
				PkgName: "test-addon",
			},
			Params: addonmgrv1alpha1.AddonParams{
				Namespace: "test-addon-ns",
			},
		},
		Status: addonmgrv1alpha1.AddonStatus{
			Lifecycle: addonmgrv1alpha1.AddonStatusLifecycle{},
			Resources: []addonmgrv1alpha1.ObjectStatus{},
		},
	}

	fakeCli := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(testAddon).Build()
	updater := NewAddonUpdater(fakeRcdr, fakeCli, NewAddonVersionCacheClient(), ctrl.Log.WithName("test"))

	err := updater.client.Create(ctx, testAddon, &client.CreateOptions{})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	existingAddon, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(existingAddon).ToNot(gomega.BeNil())

	var wfSucceeded = &wfv1.Workflow{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Workflow",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-addon-install-9375dca7-wf",
			Namespace: testNamespace,
			Labels: map[string]string{
				workflows.WfInstanceIdLabelKey: workflows.WfInstanceId,
			},
		},
		Status: wfv1.WorkflowStatus{
			Phase: wfv1.WorkflowSucceeded,
		},
	}

	err = updater.UpdateAddonStatusLifecycleFromWorkflow(ctx, testNamespace, testAddonName, wfSucceeded)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	err = updater.client.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testAddonName}, testAddon)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(testAddon.Status.Lifecycle.Installed).To(gomega.Equal(addonmgrv1alpha1.Succeeded))
}

func TestUpdateAddonStatusLifecycleFromWorkflow_InvalidChecksum(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testNamespace := "default"
	testAddonName := "test-addon"

	fakeCli := fake.NewClientBuilder().WithScheme(scheme).Build()

	updater := NewAddonUpdater(fakeRcdr, fakeCli, NewAddonVersionCacheClient(), ctrl.Log.WithName("test"))
	testAddon := &addonmgrv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAddonName,
			Namespace: testNamespace,
		},
		Spec: addonmgrv1alpha1.AddonSpec{
			PackageSpec: addonmgrv1alpha1.PackageSpec{
				PkgName: "test-addon",
			},
			Params: addonmgrv1alpha1.AddonParams{
				Namespace: "test-addon-ns",
			},
		},
		Status: addonmgrv1alpha1.AddonStatus{
			Lifecycle: addonmgrv1alpha1.AddonStatusLifecycle{},
			Resources: []addonmgrv1alpha1.ObjectStatus{},
		},
	}
	err := updater.client.Create(ctx, testAddon, &client.CreateOptions{})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	existingAddon, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(existingAddon).ToNot(gomega.BeNil())

	var wfSucceeded = &wfv1.Workflow{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Workflow",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-addon-prereqs-123456-wf",
			Namespace: testNamespace,
			Labels: map[string]string{
				workflows.WfInstanceIdLabelKey: workflows.WfInstanceId,
			},
		},
		Status: wfv1.WorkflowStatus{
			Phase: wfv1.WorkflowSucceeded,
		},
	}

	err = updater.UpdateAddonStatusLifecycleFromWorkflow(ctx, testNamespace, testAddonName, wfSucceeded)
	g.Expect(err).ToNot(gomega.HaveOccurred())
}

func TestUpdateAddonStatusLifecycleFromWorkflow_DeleteFailed(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testNamespace := "default"
	testAddonName := "test-addon"

	testAddon := &addonmgrv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAddonName,
			Namespace: testNamespace,
		},
		Spec: addonmgrv1alpha1.AddonSpec{
			PackageSpec: addonmgrv1alpha1.PackageSpec{
				PkgName: "test-addon",
			},
			Params: addonmgrv1alpha1.AddonParams{
				Namespace: "test-addon-ns",
			},
		},
		Status: addonmgrv1alpha1.AddonStatus{
			Lifecycle: addonmgrv1alpha1.AddonStatusLifecycle{},
			Resources: []addonmgrv1alpha1.ObjectStatus{},
		},
	}

	fakeCli := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(testAddon).Build()
	updater := NewAddonUpdater(fakeRcdr, fakeCli, NewAddonVersionCacheClient(), ctrl.Log.WithName("test"))

	err := updater.client.Create(ctx, testAddon, &client.CreateOptions{})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	existingAddon, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(existingAddon).ToNot(gomega.BeNil())

	var wfDelete = &wfv1.Workflow{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Workflow",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-addon-delete-9375dca7-wf",
			Namespace: testNamespace,
			Labels: map[string]string{
				workflows.WfInstanceIdLabelKey: workflows.WfInstanceId,
			},
		},
		Status: wfv1.WorkflowStatus{
			Phase: wfv1.WorkflowError,
		},
	}

	err = updater.UpdateAddonStatusLifecycleFromWorkflow(ctx, testNamespace, testAddonName, wfDelete)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	err = updater.client.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testAddonName}, testAddon)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(testAddon.Status.Lifecycle.Installed).To(gomega.Equal(addonmgrv1alpha1.DeleteFailed))
}

// TestUpdateStatus_PreservesCompletedStatusOnPeriodicReconcile tests that when wf-controller
// has already written a terminal (Completed) status and addon-controller's periodic reconcile
// subsequently calls UpdateStatus with Pending (same checksum — no spec change), the Completed
// status is preserved. This is the core race condition guard.
func TestUpdateStatus_PreservesCompletedStatusOnPeriodicReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testNamespace := "default"
	testAddonName := "test-addon-race-guard"

	testAddon := &addonmgrv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAddonName,
			Namespace: testNamespace,
		},
		Spec: addonmgrv1alpha1.AddonSpec{
			PackageSpec: addonmgrv1alpha1.PackageSpec{
				PkgName: "test-addon-race-guard",
			},
			Params: addonmgrv1alpha1.AddonParams{
				Namespace: "test-ns",
			},
		},
		Status: addonmgrv1alpha1.AddonStatus{
			Lifecycle: addonmgrv1alpha1.AddonStatusLifecycle{},
			Resources: []addonmgrv1alpha1.ObjectStatus{},
		},
	}

	fakeCli := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(testAddon).Build()
	updater := NewAddonUpdater(fakeRcdr, fakeCli, NewAddonVersionCacheClient(), ctrl.Log.WithName("test"))

	err := updater.client.Create(ctx, testAddon, &client.CreateOptions{})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	// Compute checksum — same value both controllers use when the spec has not changed.
	checksum := testAddon.CalculateChecksum()

	// Step 1: wf-controller writes Succeeded — simulates the install workflow completing.
	addonSucceeded, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonSucceeded.Status.Checksum = checksum
	addonSucceeded.Status.Lifecycle.Installed = addonmgrv1alpha1.Succeeded
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonSucceeded)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	// Confirm the addon is now Succeeded in the API server.
	current := &addonmgrv1alpha1.Addon{}
	err = updater.client.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testAddonName}, current)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(current.Status.Lifecycle.Installed).To(gomega.Equal(addonmgrv1alpha1.Succeeded))

	// Step 2: addon-controller's periodic reconcile fires. It read the addon as Pending before
	// wf-controller's write landed (stale cache), sets Pending in memory, and calls UpdateStatus.
	// Same checksum — spec has not changed.
	addonPending, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonPending.Status.Lifecycle.Installed = addonmgrv1alpha1.Pending
	addonPending.Status.Checksum = checksum
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonPending)
	// Guard fires → returns an error so controller-runtime requeues with exponential backoff.
	// The next reconcile will re-fetch fresh state and see Succeeded.
	g.Expect(err).To(gomega.HaveOccurred(), "guard must return an error to trigger a fresh reconcile")
	g.Expect(err.Error()).To(gomega.ContainSubstring("race guard"), "error must identify itself as a race guard requeue, not a real API failure")

	// Assert: Succeeded must be preserved — the Pending write must be skipped by the guard.
	current = &addonmgrv1alpha1.Addon{}
	err = updater.client.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testAddonName}, current)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(current.Status.Lifecycle.Installed).To(gomega.Equal(addonmgrv1alpha1.Succeeded),
		"addon-controller must not overwrite a Succeeded status with Pending when the spec (checksum) has not changed")

	// Assert: version cache must also reflect Succeeded, not the stale Pending.
	// The cache is read by dependency validation; a stale Pending here would cause
	// dependents to fail validation unnecessarily.
	ok, cachedVersion := updater.versionCache.HasVersionName(testAddonName)
	g.Expect(ok).To(gomega.BeTrue(), "addon must be present in the version cache after wf-controller's Succeeded write")
	g.Expect(cachedVersion.PkgPhase).To(gomega.Equal(addonmgrv1alpha1.Succeeded),
		"version cache must not be poisoned with Pending when the race guard skips the write")
}

// TestUpdateStatus_AllowsPendingOnSpecChange tests that UpdateStatus allows writing Pending when
// the addon's spec has changed (new checksum). This is the legitimate new-install-cycle case and
// must not be blocked by the race guard.
func TestUpdateStatus_AllowsPendingOnSpecChange(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testNamespace := "default"
	testAddonName := "test-addon-spec-change"

	testAddon := &addonmgrv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAddonName,
			Namespace: testNamespace,
		},
		Spec: addonmgrv1alpha1.AddonSpec{
			PackageSpec: addonmgrv1alpha1.PackageSpec{
				PkgName: "test-addon-spec-change",
			},
			Params: addonmgrv1alpha1.AddonParams{
				Namespace: "test-ns",
			},
		},
	}

	fakeCli := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(testAddon).Build()
	updater := NewAddonUpdater(fakeRcdr, fakeCli, NewAddonVersionCacheClient(), ctrl.Log.WithName("test"))

	err := updater.client.Create(ctx, testAddon, &client.CreateOptions{})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	oldChecksum := testAddon.CalculateChecksum()

	// Step 1: write Succeeded with old checksum.
	addonSucceeded, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonSucceeded.Status.Checksum = oldChecksum
	addonSucceeded.Status.Lifecycle.Installed = addonmgrv1alpha1.Succeeded
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonSucceeded)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	// Step 2: spec changes — addon-controller computes a new checksum and writes Pending.
	// Simulate by changing the spec field and recalculating the real checksum, mirroring
	// what validateChecksum() does in the reconciler.
	addonNewSpec, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonNewSpec.Spec.PackageSpec.PkgVersion = "v2.0.0" // spec change → different checksum
	newChecksum := addonNewSpec.CalculateChecksum()
	g.Expect(newChecksum).ToNot(gomega.Equal(oldChecksum), "spec change must produce a different checksum")
	addonNewSpec.Status.Lifecycle.Installed = addonmgrv1alpha1.Pending
	addonNewSpec.Status.Checksum = newChecksum
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonNewSpec)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	// Assert: Pending must be written — spec changed so this is a legitimate new cycle.
	current := &addonmgrv1alpha1.Addon{}
	err = updater.client.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testAddonName}, current)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(current.Status.Lifecycle.Installed).To(gomega.Equal(addonmgrv1alpha1.Pending),
		"addon-controller must be allowed to write Pending when the spec (checksum) has changed")
}

// TestUpdateStatus_PreservesCompletedStatusOnValidationFailedWrite tests that when
// wf-controller has already written a terminal (Completed) status and addon-controller's
// stale reconcile subsequently calls UpdateStatus with ValidationFailed (same checksum),
// the Completed status is preserved. ValidationFailed is a Running() phase and is not a
// designed post-Completed transition within the same install cycle.
func TestUpdateStatus_PreservesCompletedStatusOnValidationFailedWrite(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testNamespace := "default"
	testAddonName := "test-addon-validation-failed-guard"

	testAddon := &addonmgrv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAddonName,
			Namespace: testNamespace,
		},
		Spec: addonmgrv1alpha1.AddonSpec{
			PackageSpec: addonmgrv1alpha1.PackageSpec{
				PkgName: "test-addon-validation-failed-guard",
			},
			Params: addonmgrv1alpha1.AddonParams{
				Namespace: "test-ns",
			},
		},
		Status: addonmgrv1alpha1.AddonStatus{
			Lifecycle: addonmgrv1alpha1.AddonStatusLifecycle{},
			Resources: []addonmgrv1alpha1.ObjectStatus{},
		},
	}

	fakeCli := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(testAddon).Build()
	updater := NewAddonUpdater(fakeRcdr, fakeCli, NewAddonVersionCacheClient(), ctrl.Log.WithName("test"))

	err := updater.client.Create(ctx, testAddon, &client.CreateOptions{})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	checksum := testAddon.CalculateChecksum()

	// Step 1: wf-controller writes Succeeded — install workflow completed.
	addonSucceeded, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonSucceeded.Status.Checksum = checksum
	addonSucceeded.Status.Lifecycle.Installed = addonmgrv1alpha1.Succeeded
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonSucceeded)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	// Step 2: addon-controller's stale reconcile still in flight — it ran validation
	// which failed (e.g. a dependency was in Pending state at read time), sets
	// ValidationFailed, and calls UpdateStatus. Same checksum — spec unchanged.
	addonValFailed, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonValFailed.Status.Lifecycle.Installed = addonmgrv1alpha1.ValidationFailed
	addonValFailed.Status.Checksum = checksum
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonValFailed)
	// Guard fires → returns an error so controller-runtime requeues with exponential backoff.
	g.Expect(err).To(gomega.HaveOccurred(), "guard must return an error to trigger a fresh reconcile")
	g.Expect(err.Error()).To(gomega.ContainSubstring("race guard"))

	// Assert: Succeeded must be preserved.
	current := &addonmgrv1alpha1.Addon{}
	err = updater.client.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testAddonName}, current)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(current.Status.Lifecycle.Installed).To(gomega.Equal(addonmgrv1alpha1.Succeeded),
		"addon-controller must not overwrite Succeeded with ValidationFailed when spec (checksum) is unchanged")

	// Assert: version cache must also reflect Succeeded.
	ok, cachedVersion := updater.versionCache.HasVersionName(testAddonName)
	g.Expect(ok).To(gomega.BeTrue())
	g.Expect(cachedVersion.PkgPhase).To(gomega.Equal(addonmgrv1alpha1.Succeeded),
		"version cache must not be poisoned with ValidationFailed when the race guard skips the write")
}

// TestUpdateStatus_PreservesFailedStatusOnValidationFailedWrite tests the same guard
// fires when the API already has Failed (not just Succeeded) — both are Completed().
func TestUpdateStatus_PreservesFailedStatusOnValidationFailedWrite(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testNamespace := "default"
	testAddonName := "test-addon-failed-vf-guard"

	testAddon := &addonmgrv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAddonName,
			Namespace: testNamespace,
		},
		Spec: addonmgrv1alpha1.AddonSpec{
			PackageSpec: addonmgrv1alpha1.PackageSpec{
				PkgName: "test-addon-failed-vf-guard",
			},
			Params: addonmgrv1alpha1.AddonParams{
				Namespace: "test-ns",
			},
		},
	}

	fakeCli := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(testAddon).Build()
	updater := NewAddonUpdater(fakeRcdr, fakeCli, NewAddonVersionCacheClient(), ctrl.Log.WithName("test"))

	err := updater.client.Create(ctx, testAddon, &client.CreateOptions{})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	checksum := testAddon.CalculateChecksum()

	// Step 1: API has Failed (e.g. a previous install genuinely failed).
	addonFailed, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonFailed.Status.Checksum = checksum
	addonFailed.Status.Lifecycle.Installed = addonmgrv1alpha1.Failed
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonFailed)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	// Step 2: stale reconcile fires ValidationFailed with same checksum.
	addonValFailed, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonValFailed.Status.Lifecycle.Installed = addonmgrv1alpha1.ValidationFailed
	addonValFailed.Status.Checksum = checksum
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonValFailed)
	// Guard fires → returns an error so controller-runtime requeues with exponential backoff.
	g.Expect(err).To(gomega.HaveOccurred(), "guard must return an error to trigger a fresh reconcile")
	g.Expect(err.Error()).To(gomega.ContainSubstring("race guard"))

	// Assert: Failed must be preserved — ValidationFailed must not overwrite a terminal status.
	current := &addonmgrv1alpha1.Addon{}
	err = updater.client.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testAddonName}, current)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(current.Status.Lifecycle.Installed).To(gomega.Equal(addonmgrv1alpha1.Failed),
		"ValidationFailed must not overwrite a terminal Failed status for the same spec cycle")
}

// TestUpdateStatus_PreservesFailedStatusOnPendingWrite tests that the guard fires when
// the API already has Failed and a stale reconcile tries to write Pending with the same
// checksum. Failed is Completed() so the guard must protect it just like Succeeded.
func TestUpdateStatus_PreservesFailedStatusOnPendingWrite(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testNamespace := "default"
	testAddonName := "test-addon-failed-pending-guard"

	testAddon := &addonmgrv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAddonName,
			Namespace: testNamespace,
		},
		Spec: addonmgrv1alpha1.AddonSpec{
			PackageSpec: addonmgrv1alpha1.PackageSpec{
				PkgName: "test-addon-failed-pending-guard",
			},
			Params: addonmgrv1alpha1.AddonParams{
				Namespace: "test-ns",
			},
		},
	}

	fakeCli := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(testAddon).Build()
	updater := NewAddonUpdater(fakeRcdr, fakeCli, NewAddonVersionCacheClient(), ctrl.Log.WithName("test"))

	err := updater.client.Create(ctx, testAddon, &client.CreateOptions{})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	checksum := testAddon.CalculateChecksum()

	// Step 1: API has Failed — the install genuinely failed.
	addonFailed, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonFailed.Status.Checksum = checksum
	addonFailed.Status.Lifecycle.Installed = addonmgrv1alpha1.Failed
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonFailed)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	// Step 2: stale reconcile still holds Pending from before the failure was written.
	addonPending, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonPending.Status.Lifecycle.Installed = addonmgrv1alpha1.Pending
	addonPending.Status.Checksum = checksum
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonPending)
	g.Expect(err).To(gomega.HaveOccurred(), "guard must fire")
	g.Expect(err.Error()).To(gomega.ContainSubstring("race guard"))

	current := &addonmgrv1alpha1.Addon{}
	err = updater.client.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testAddonName}, current)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(current.Status.Lifecycle.Installed).To(gomega.Equal(addonmgrv1alpha1.Failed),
		"stale Pending must not overwrite a terminal Failed status")

	ok, cachedVersion := updater.versionCache.HasVersionName(testAddonName)
	g.Expect(ok).To(gomega.BeTrue())
	g.Expect(cachedVersion.PkgPhase).To(gomega.Equal(addonmgrv1alpha1.Failed),
		"version cache must not be poisoned with stale Pending when guard fires")
}

// TestUpdateStatus_PreservesDeleteSucceededStatusOnPendingWrite tests that the guard
// fires when the API has DeleteSucceeded and a stale reconcile tries to write Pending.
// DeleteSucceeded is Completed() via Succeeded().
func TestUpdateStatus_PreservesDeleteSucceededStatusOnPendingWrite(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testNamespace := "default"
	testAddonName := "test-addon-deletesucceeded-pending-guard"

	testAddon := &addonmgrv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAddonName,
			Namespace: testNamespace,
		},
		Spec: addonmgrv1alpha1.AddonSpec{
			PackageSpec: addonmgrv1alpha1.PackageSpec{
				PkgName: "test-addon-deletesucceeded-pending-guard",
			},
			Params: addonmgrv1alpha1.AddonParams{
				Namespace: "test-ns",
			},
		},
	}

	fakeCli := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(testAddon).Build()
	updater := NewAddonUpdater(fakeRcdr, fakeCli, NewAddonVersionCacheClient(), ctrl.Log.WithName("test"))

	err := updater.client.Create(ctx, testAddon, &client.CreateOptions{})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	checksum := testAddon.CalculateChecksum()

	addonDeleteSucceeded, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonDeleteSucceeded.Status.Checksum = checksum
	addonDeleteSucceeded.Status.Lifecycle.Installed = addonmgrv1alpha1.DeleteSucceeded
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonDeleteSucceeded)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	addonPending, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonPending.Status.Lifecycle.Installed = addonmgrv1alpha1.Pending
	addonPending.Status.Checksum = checksum
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonPending)
	g.Expect(err).To(gomega.HaveOccurred(), "guard must fire")
	g.Expect(err.Error()).To(gomega.ContainSubstring("race guard"))

	current := &addonmgrv1alpha1.Addon{}
	err = updater.client.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testAddonName}, current)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(current.Status.Lifecycle.Installed).To(gomega.Equal(addonmgrv1alpha1.DeleteSucceeded),
		"stale Pending must not overwrite a terminal DeleteSucceeded status")
}

// TestUpdateStatus_PreservesDeleteSucceededStatusOnValidationFailedWrite tests that the
// guard fires when the API has DeleteSucceeded and a stale reconcile tries to write
// ValidationFailed with the same checksum.
func TestUpdateStatus_PreservesDeleteSucceededStatusOnValidationFailedWrite(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testNamespace := "default"
	testAddonName := "test-addon-deletesucceeded-vf-guard"

	testAddon := &addonmgrv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAddonName,
			Namespace: testNamespace,
		},
		Spec: addonmgrv1alpha1.AddonSpec{
			PackageSpec: addonmgrv1alpha1.PackageSpec{
				PkgName: "test-addon-deletesucceeded-vf-guard",
			},
			Params: addonmgrv1alpha1.AddonParams{
				Namespace: "test-ns",
			},
		},
	}

	fakeCli := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(testAddon).Build()
	updater := NewAddonUpdater(fakeRcdr, fakeCli, NewAddonVersionCacheClient(), ctrl.Log.WithName("test"))

	err := updater.client.Create(ctx, testAddon, &client.CreateOptions{})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	checksum := testAddon.CalculateChecksum()

	addonDeleteSucceeded, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonDeleteSucceeded.Status.Checksum = checksum
	addonDeleteSucceeded.Status.Lifecycle.Installed = addonmgrv1alpha1.DeleteSucceeded
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonDeleteSucceeded)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	addonValFailed, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonValFailed.Status.Lifecycle.Installed = addonmgrv1alpha1.ValidationFailed
	addonValFailed.Status.Checksum = checksum
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonValFailed)
	g.Expect(err).To(gomega.HaveOccurred(), "guard must fire")
	g.Expect(err.Error()).To(gomega.ContainSubstring("race guard"))

	current := &addonmgrv1alpha1.Addon{}
	err = updater.client.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testAddonName}, current)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(current.Status.Lifecycle.Installed).To(gomega.Equal(addonmgrv1alpha1.DeleteSucceeded),
		"stale ValidationFailed must not overwrite a terminal DeleteSucceeded status")
}

// TestUpdateStatus_PreservesDeleteFailedStatusOnPendingWrite tests that the guard fires
// when the API has DeleteFailed and a stale reconcile tries to write Pending with the
// same checksum. DeleteFailed is Completed() via Failed().
func TestUpdateStatus_PreservesDeleteFailedStatusOnPendingWrite(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testNamespace := "default"
	testAddonName := "test-addon-deletefailed-pending-guard"

	testAddon := &addonmgrv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAddonName,
			Namespace: testNamespace,
		},
		Spec: addonmgrv1alpha1.AddonSpec{
			PackageSpec: addonmgrv1alpha1.PackageSpec{
				PkgName: "test-addon-deletefailed-pending-guard",
			},
			Params: addonmgrv1alpha1.AddonParams{
				Namespace: "test-ns",
			},
		},
	}

	fakeCli := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(testAddon).Build()
	updater := NewAddonUpdater(fakeRcdr, fakeCli, NewAddonVersionCacheClient(), ctrl.Log.WithName("test"))

	err := updater.client.Create(ctx, testAddon, &client.CreateOptions{})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	checksum := testAddon.CalculateChecksum()

	addonDeleteFailed, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonDeleteFailed.Status.Checksum = checksum
	addonDeleteFailed.Status.Lifecycle.Installed = addonmgrv1alpha1.DeleteFailed
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonDeleteFailed)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	addonPending, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonPending.Status.Lifecycle.Installed = addonmgrv1alpha1.Pending
	addonPending.Status.Checksum = checksum
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonPending)
	g.Expect(err).To(gomega.HaveOccurred(), "guard must fire")
	g.Expect(err.Error()).To(gomega.ContainSubstring("race guard"))

	current := &addonmgrv1alpha1.Addon{}
	err = updater.client.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testAddonName}, current)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(current.Status.Lifecycle.Installed).To(gomega.Equal(addonmgrv1alpha1.DeleteFailed),
		"stale Pending must not overwrite a terminal DeleteFailed status")
}

// TestUpdateStatus_PreservesDeleteFailedStatusOnValidationFailedWrite tests that the
// guard fires when the API has DeleteFailed and a stale reconcile tries to write
// ValidationFailed with the same checksum.
func TestUpdateStatus_PreservesDeleteFailedStatusOnValidationFailedWrite(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testNamespace := "default"
	testAddonName := "test-addon-deletefailed-vf-guard"

	testAddon := &addonmgrv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAddonName,
			Namespace: testNamespace,
		},
		Spec: addonmgrv1alpha1.AddonSpec{
			PackageSpec: addonmgrv1alpha1.PackageSpec{
				PkgName: "test-addon-deletefailed-vf-guard",
			},
			Params: addonmgrv1alpha1.AddonParams{
				Namespace: "test-ns",
			},
		},
	}

	fakeCli := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(testAddon).Build()
	updater := NewAddonUpdater(fakeRcdr, fakeCli, NewAddonVersionCacheClient(), ctrl.Log.WithName("test"))

	err := updater.client.Create(ctx, testAddon, &client.CreateOptions{})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	checksum := testAddon.CalculateChecksum()

	addonDeleteFailed, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonDeleteFailed.Status.Checksum = checksum
	addonDeleteFailed.Status.Lifecycle.Installed = addonmgrv1alpha1.DeleteFailed
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonDeleteFailed)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	addonValFailed, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonValFailed.Status.Lifecycle.Installed = addonmgrv1alpha1.ValidationFailed
	addonValFailed.Status.Checksum = checksum
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonValFailed)
	g.Expect(err).To(gomega.HaveOccurred(), "guard must fire")
	g.Expect(err.Error()).To(gomega.ContainSubstring("race guard"))

	current := &addonmgrv1alpha1.Addon{}
	err = updater.client.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testAddonName}, current)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(current.Status.Lifecycle.Installed).To(gomega.Equal(addonmgrv1alpha1.DeleteFailed),
		"stale ValidationFailed must not overwrite a terminal DeleteFailed status")
}

// TestUpdateStatus_AllowsValidationFailedOnSpecChange tests that ValidationFailed IS
// written when the checksum differs (new spec cycle), matching the Pending behaviour.
func TestUpdateStatus_AllowsValidationFailedOnSpecChange(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testNamespace := "default"
	testAddonName := "test-addon-vf-spec-change"

	testAddon := &addonmgrv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAddonName,
			Namespace: testNamespace,
		},
		Spec: addonmgrv1alpha1.AddonSpec{
			PackageSpec: addonmgrv1alpha1.PackageSpec{
				PkgName: "test-addon-vf-spec-change",
			},
			Params: addonmgrv1alpha1.AddonParams{
				Namespace: "test-ns",
			},
		},
	}

	fakeCli := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(testAddon).Build()
	updater := NewAddonUpdater(fakeRcdr, fakeCli, NewAddonVersionCacheClient(), ctrl.Log.WithName("test"))

	err := updater.client.Create(ctx, testAddon, &client.CreateOptions{})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	oldChecksum := testAddon.CalculateChecksum()

	// Step 1: Succeeded with old spec.
	addonSucceeded, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonSucceeded.Status.Checksum = oldChecksum
	addonSucceeded.Status.Lifecycle.Installed = addonmgrv1alpha1.Succeeded
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonSucceeded)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	// Step 2: spec changes, new reconcile runs validation which fails → ValidationFailed
	// with a new checksum. The guard must NOT fire here.
	addonNewSpec, err := updater.getExistingAddon(ctx, testNamespace, testAddonName)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	addonNewSpec.Spec.PackageSpec.PkgVersion = "v3.0.0" // spec change
	newChecksum := addonNewSpec.CalculateChecksum()
	g.Expect(newChecksum).ToNot(gomega.Equal(oldChecksum))
	addonNewSpec.Status.Lifecycle.Installed = addonmgrv1alpha1.ValidationFailed
	addonNewSpec.Status.Checksum = newChecksum
	err = updater.UpdateStatus(ctx, ctrl.Log.WithName("test"), addonNewSpec)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	// Assert: ValidationFailed must be written — new spec cycle.
	current := &addonmgrv1alpha1.Addon{}
	err = updater.client.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testAddonName}, current)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(current.Status.Lifecycle.Installed).To(gomega.Equal(addonmgrv1alpha1.ValidationFailed),
		"ValidationFailed must be written when the spec (checksum) has changed — guard must not block new-cycle writes")
}
