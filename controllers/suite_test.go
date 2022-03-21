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

	//"github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/scheme"

	"github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	k8sClient client.Client
	testEnv   *envtest.Environment
	log       logr.Logger
	ctx       context.Context
	cancel    context.CancelFunc
	stopMgr   chan struct{}
	instance  *v1alpha1.Addon
	instance2 *v1alpha1.Addon
	instance1 *v1alpha1.Addon
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = AfterSuite(func() {
	By("cleanup instance(s)")
	if instance != nil {
		err := k8sClient.Delete(context.TODO(), instance)
		Expect(err).To(BeNil())
	}
	if instance1 != nil {
		err := k8sClient.Delete(context.TODO(), instance1)
		Expect(err).To(BeNil())
	}
	if instance2 != nil {
		err := k8sClient.Delete(context.TODO(), instance2)
		Expect(err).To(BeNil())
	}

	By("stopping manager")
	close(stopMgr)
	cancel()

	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())

})

func StartTestManager(mgr manager.Manager) {
	wg := &sync.WaitGroup{}
	go func() {
		defer GinkgoRecover()
		wg.Add(1)
		Expect(mgr.Start(ctx)).ToNot(HaveOccurred(), "failed to run manager")
		wg.Done()
	}()
}

var _ = BeforeSuite(func(done Done) {
	log = zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	logf.SetLogger(log)

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		Scheme:                common.GetAddonMgrScheme(),
	}

	cfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	By("starting reconciler and manager")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		NewClient:               NewCachingClient,
		Scheme:                  common.GetAddonMgrScheme(),
		MetricsBindAddress:      ":8080",
		LeaderElection:          true,
		LeaderElectionID:        "addonmgr.keikoproj.io",
		LeaderElectionNamespace: addonNamespace,
	})

	if err != nil {
		panic(err)
	}
	k8sClient = mgr.GetClient()

	stopMgr = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	New(ctx, mgr, stopMgr)
	StartTestManager(mgr)
	close(done)
}, 60)
