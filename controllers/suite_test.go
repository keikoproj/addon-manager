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
	"fmt"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	k8sClient client.Client
	testEnv   *envtest.Environment
	stopMgr   chan struct{}
	wg        *sync.WaitGroup
	log       logr.Logger
	ctx       context.Context
	cancel    context.CancelFunc

	addonController *Controller
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

// var _ = BeforeSuite(func(done Done) {
// 	log = zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
// 	logf.SetLogger(log)

// 	ctx, cancel = context.WithCancel(context.TODO())

// 	By("bootstrapping test environment")
// 	testEnv = &envtest.Environment{
// 		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
// 		ErrorIfCRDPathMissing: true,
// 	}

// 	cfg, err := testEnv.Start()
// 	Expect(err).ToNot(HaveOccurred())
// 	Expect(cfg).ToNot(BeNil())

// 	err = addonmgrv1alpha1.AddToScheme(scheme.Scheme)
// 	Expect(err).NotTo(HaveOccurred())

// 	By("starting reconciler and manager")
// 	k8sClient, err = client.New(cfg, client.Options{Scheme: common.GetAddonMgrScheme()})
// 	Expect(err).ToNot(HaveOccurred())
// 	Expect(k8sClient).ToNot(BeNil())

// 	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
// 		Scheme:         common.GetAddonMgrScheme(),
// 		LeaderElection: false,
// 	})
// 	Expect(err).ToNot(HaveOccurred())
// 	Expect(mgr).ToNot(BeNil())

// 	go func(c *Controller) {
// 		New(mgr, stopMgr)
// 	}(addonController)

// 	stopMgr, wg = StartTestManager(mgr)

// 	close(done)
// }, 60)

var _ = AfterSuite(func() {
	cancel()
	By("stopping manager")
	close(stopMgr)
	//wg.Wait()

	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

func StartController(mgr manager.Manager, stop chan struct{}, wg *sync.WaitGroup) {
	go func() {
		New(mgr, stopMgr)
		fmt.Printf("start controller successfully.")
	}()
}

func StartTestManager(mgr manager.Manager, wg *sync.WaitGroup) {
	go func() {
		if err := mgr.Start(ctx); err != nil {
			fmt.Printf("failed start test manager")
			panic(err)
		}
		fmt.Printf("start test manager successfully.")
	}()
}
