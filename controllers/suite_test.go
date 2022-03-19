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
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/scheme"
	"github.com/keikoproj/addon-manager/pkg/common"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
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
	wg        *sync.WaitGroup
	log       logr.Logger
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = AfterSuite(func() {
	By("stopping manager")
	close(stopMgr)
	cancel()

	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())

})

func StartTestManager(mgr manager.Manager) (chan struct{}, *sync.WaitGroup) {
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	go func() {
		defer GinkgoRecover()
		wg.Add(1)
		Expect(mgr.Start(ctx)).ToNot(HaveOccurred(), "failed to run manager")
		wg.Done()
	}()
	return stop, wg
}

var _ = BeforeSuite(func() {
	log = zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	logf.SetLogger(log)

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = addonmgrv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("starting reconciler and manager")
	k8sClient, err = client.New(cfg, client.Options{Scheme: common.GetAddonMgrScheme()})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             common.GetAddonMgrScheme(),
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(mgr).ToNot(BeNil())

	stopMgr, wg = StartTestManager(mgr)
	kubeClient := kubernetes.NewForConfigOrDie(cfg)
	dynCli, err := dynamic.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	wfcli := common.NewWFClient(cfg)
	if wfcli == nil {
		panic("workflow client could not be nil")
	}
	addoncli := common.NewAddonClient(cfg)
	ctx, cancel = context.WithCancel(context.Background())

	ns := "addon-manager-system"
	_, err = kubeClient.CoreV1().Namespaces().Create(ctx, &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}
	addonController = newResourceController(kubeClient, dynCli, addoncli, wfcli, "addon", ns)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer GinkgoRecover()
		defer wg.Done()
		addonController.Run(ctx, stopMgr)
	}()
	Expect(addonController).ToNot(BeNil())
})
