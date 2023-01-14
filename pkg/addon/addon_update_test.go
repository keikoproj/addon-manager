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
