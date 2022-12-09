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
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
	"time"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
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
	fakeCli  = fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx      = context.TODO()
)

func init() {
	_ = addonmgrv1alpha1.AddToScheme(scheme)
	_ = wfv1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
}

func TestUpdateAddonStatusLifecycle(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testNamespace := "default"
	testAddonName := "test-addon"

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

	err = updater.UpdateAddonStatusLifecycle(ctx, testNamespace, testAddonName, addonmgrv1alpha1.Install, addonmgrv1alpha1.Succeeded)
	g.Expect(err).ToNot(gomega.HaveOccurred())
}
