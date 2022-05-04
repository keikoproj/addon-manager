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
	"path/filepath"
	"testing"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = addonmgrv1alpha1.AddToScheme(scheme)
	_ = wfv1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
}

func setup(testEnv *envtest.Environment) *rest.Config {
	cfg, err := testEnv.Start()
	if err != nil || cfg == nil {
		panic(err)
	}
	return cfg
}

func cleanup(testEnv *envtest.Environment) {
	defer GinkgoRecover()
	err := testEnv.Stop()
	if err != nil {
		panic(err)
	}

}

func newClient(cfg *rest.Config) (client.Client, error) {
	opts := client.Options{
		Scheme: scheme,
	}
	k8sclient, err := client.New(cfg, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create kube client: %#v", err)
	}
	return k8sclient, nil
}

func TestUpdateAddonStatusLifecycle(t *testing.T) {
	RegisterFailHandler(Fail)

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	testNamespace := "default"
	testAddonName := "test-addon"

	cfg := setup(testEnv)
	cl, err := newClient(cfg)
	if err != nil || cl == nil {
		t.Fatalf("failed to create client %#v", err)
	}
	log := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	updater := NewAddonUpdate(cl, log, NewAddonVersionCacheClient())
	ctx := context.TODO()
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
	err = updater.client.Create(ctx, testAddon, &client.CreateOptions{})
	Expect(err).To(BeNil())

	existingAddon, err := updater.getExistingAddon(ctx, fmt.Sprintf("%s/%s", testNamespace, testAddonName))
	Expect(err).To(BeNil())
	Expect(existingAddon).NotTo(BeNil())

	err = updater.UpdateAddonStatusLifecycle(ctx, testNamespace, testAddonName, "install", "Succeeded")
	if err != nil {
		fmt.Printf(" update addon status err %#v", err)
	}
	Expect(err).To(BeNil())
	cleanup(testEnv)
}
