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
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	//"github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/scheme"

	"github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	addoninternal "github.com/keikoproj/addon-manager/pkg/addon"
	"github.com/keikoproj/addon-manager/pkg/common"
	"github.com/keikoproj/addon-manager/pkg/utils"
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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		//setupLog.Error(err, "problem running manager")
		panic(err)
	}
}

var _ = BeforeSuite(func(done Done) {
	log = zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	logf.SetLogger(log)

	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
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

	stopMgr = make(chan struct{})
	New(ctx, mgr, stopMgr)
	StartTestManager(mgr)

	close(done)
}, 60)

func NewController(ctx context.Context, mgr manager.Manager, stopCh chan struct{}) *Controller {
	kubeClient := kubernetes.NewForConfigOrDie(mgr.GetConfig())
	if kubeClient == nil {
		panic("kubeClient could not be nil")
	}
	dynCli := dynamic.NewForConfigOrDie(mgr.GetConfig())
	if dynCli == nil {
		panic("dynCli could not be nil")
	}
	wfcli := common.NewWFClient(mgr.GetConfig())
	if wfcli == nil {
		panic("wfcli could not be nil")
	}
	addoncli := common.NewAddonClient(mgr.GetConfig())
	if addoncli == nil {
		panic("addoncli could not be nil")
	}

	c := &Controller{
		logger:       mgr.GetLogger().WithName("addon-manager-controller"),
		clientset:    kubeClient,
		dynCli:       dynCli,
		addoncli:     addoncli,
		wfcli:        wfcli,
		client:       mgr.GetClient(),
		namespace:    addonNamespace,
		recorder:     mgr.GetEventRecorderFor("addon-manager-controller"),
		config:       mgr.GetConfig(),
		runtimecache: mgr.GetCache(),
	}
	c.queue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	c.wftlock = sync.Mutex{}

	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	// ctx, cancel := context.WithCancel(ctx)
	// defer cancel()

	c.logger.Info("Starting keiko addon-manager controller")
	c.scheme = common.GetAddonMgrScheme()
	c.versionCache = addoninternal.NewAddonVersionCacheClient()

	c.addoninformer = newAddonInformer(ctx, c.dynCli, c.namespace)
	c.wfinformer = utils.NewWorkflowInformer(c.dynCli, c.namespace, workflowResyncPeriod, cache.Indexers{}, utils.TweakListOptions)

	resourceInformers := NewResourceInformers(ctx, c.clientset, c.namespace)
	c.nsinformer = resourceInformers["namespace"]
	c.deploymentinformer = resourceInformers["deployment"]
	c.srvinformer = resourceInformers["service"]
	c.configMapinformer = resourceInformers["configmap"]
	c.clusterRoleinformer = resourceInformers["clusterrole"]
	c.clusterRoleBindingInformer = resourceInformers["clusterRoleBinding"]
	c.jobinformer = resourceInformers["job"]
	c.srvAcntinformer = resourceInformers["serviceAccount"]
	c.cronjobinformer = resourceInformers["cronjob"]
	c.daemonSetinformer = resourceInformers["daemonSet"]
	c.replicaSetinformer = resourceInformers["replicaSet"]
	c.statefulSetinformer = resourceInformers["statefulSet"]

	c.setupaddonhandlers()
	c.setupwfhandlers(ctx)
	c.setupresourcehandlers(ctx)

	go c.addoninformer.Run(ctx.Done())
	go c.wfinformer.Run(ctx.Done())

	go c.nsinformer.Run(ctx.Done())
	go c.deploymentinformer.Run(ctx.Done())
	go c.srvAcntinformer.Run(ctx.Done())
	go c.configMapinformer.Run(ctx.Done())
	go c.clusterRoleinformer.Run(ctx.Done())
	go c.clusterRoleBindingInformer.Run(ctx.Done())
	go c.jobinformer.Run(ctx.Done())
	go c.cronjobinformer.Run(ctx.Done())
	go c.replicaSetinformer.Run(ctx.Done())
	go c.daemonSetinformer.Run(ctx.Done())
	go c.srvinformer.Run(ctx.Done())
	go c.replicaSetinformer.Run(ctx.Done())
	go c.srvinformer.Run(ctx.Done())

	// if !toolscache.WaitForCacheSync(stopCh, c.addoninformer.HasSynced, c.wfinformer.HasSynced) {
	// 	utilruntime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
	// 	panic("failed sync cache.")
	// }
	if !c.runtimecache.WaitForCacheSync(ctx) {
		utilruntime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		panic("failed sync cache.")
	}

	c.logger.Info("Keiko addon-manager controller synced and ready")

	go wait.Until(c.runWorker, time.Second, stopCh)
	return c
}
