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

package controllers

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	//"github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/scheme"

	"github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/scheme"
	"github.com/keikoproj/addon-manager/pkg/common"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	// +kubebuilder:scaffold:imports
	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	k8sClient      client.Client
	testEnv        *envtest.Environment
	log            logr.Logger
	ctx            context.Context
	cancel         context.CancelFunc
	stopMgr        chan struct{}
	instance       *v1alpha1.Addon
	instance2      *v1alpha1.Addon
	instance1      *v1alpha1.Addon
	addonNamespace = "addon-manager-system"
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
		Scheme:            common.GetAddonMgrScheme(),
	}

	err := addonv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = workflowv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	cliCfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cliCfg).ToNot(BeNil())

	By("starting reconciler and manager")
	ctx, cancel = context.WithCancel(context.Background())
	stopMgr = make(chan struct{})

	mgr, err := ctrl.NewManager(cliCfg, ctrl.Options{
		Scheme:         common.GetAddonMgrScheme(),
		LeaderElection: false,
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(mgr).ToNot(BeNil())

	New(ctx, mgr, stopMgr, addonNamespace)
	wg := &sync.WaitGroup{}
	StartTestManager(mgr, wg)

	By("Build client")
	k8sClient, err = client.New(cliCfg, client.Options{
		Scheme: common.GetAddonMgrScheme(),
	})
	Expect(k8sClient).ToNot(BeNil())
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: addonNamespace,
		},
	}

	err = k8sClient.Create(ctx, ns, &client.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		err = nil
	}
	Expect(err).To(BeNil())

	close(done)
}, 60)

func StartTestManager(mgr manager.Manager, wg *sync.WaitGroup) {
	go func() {
		defer GinkgoRecover()
		defer wg.Done()
		Expect(mgr.Start(ctx)).ToNot(HaveOccurred(), "failed to run manager")
	}()
}

var _ = AfterSuite(func() {
	By("cleanup instance(s)")
	if instance != nil {
		k8sClient.Delete(ctx, instance)
	}
	if instance1 != nil {
		k8sClient.Delete(ctx, instance1)
	}
	if instance2 != nil {
		k8sClient.Delete(ctx, instance2)
	}

	By("stopping manager")
	close(stopMgr)
	defer cancel()

	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
