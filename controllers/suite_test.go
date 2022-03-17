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
	"testing"
	"time"

	addoninternal "github.com/keikoproj/addon-manager/pkg/addon"
	"github.com/keikoproj/addon-manager/pkg/common"
	"github.com/keikoproj/addon-manager/pkg/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var (
	testEnv         *envtest.Environment
	ctx             context.Context
	addonController *Controller
	stopCh          chan struct{}
)

func getTestController() *Controller {
	ctx = context.TODO()
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	wfcli := common.NewWFClient(cfg)
	if wfcli == nil {
		panic("failed init wf client.")
	}
	kubecli, err := common.NewKubernetesClient(cfg)
	if err != nil || kubecli == nil {
		panic(err)
	}
	addonCli := common.NewAddonClient(cfg)
	if addonCli == nil {
		panic("failed init addon cli")
	}
	dynCli, err := dynamic.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	if dynCli == nil {
		panic("failed init dynCli")
	}

	addonController = newResourceController(kubecli, dynCli, addonCli, wfcli, "addon", "default")
	Expect(addonController).NotTo(BeNil())

	stopCh = make(chan struct{})
	logger := logrus.WithField("controllers", "addon")

	addonController.scheme = common.GetAddonMgrScheme()
	addonController.versionCache = addoninternal.NewAddonVersionCacheClient()
	addonController.recorder = createEventRecorder(addonController.namespace, addonController.clientset, logger)
	addonController.informer = newAddonInformer(ctx, addonController.dynCli, addonController.namespace)
	addonController.wfinformer = utils.NewWorkflowInformer(addonController.dynCli, addonController.namespace, 0, cache.Indexers{}, utils.TweakListOptions)
	k8sinformer := kubeinformers.NewSharedInformerFactory(kubecli, 0)

	addonController.nsinformer = k8sinformer.Core().V1().Namespaces().Informer()
	addonController.deploymentinformer = k8sinformer.Apps().V1().Deployments().Informer()
	addonController.srvinformer = k8sinformer.Core().V1().Services().Informer()
	addonController.configMapinformer = k8sinformer.Core().V1().ConfigMaps().Informer()
	addonController.clusterRoleinformer = k8sinformer.Rbac().V1().ClusterRoles().Informer()
	addonController.clusterRoleBindingInformer = k8sinformer.Rbac().V1().ClusterRoleBindings().Informer()
	addonController.jobinformer = k8sinformer.Batch().V1().Jobs().Informer()
	addonController.cronjobinformer = k8sinformer.Core().V1().ServiceAccounts().Informer()
	addonController.cronjobinformer = k8sinformer.Batch().V1().CronJobs().Informer()
	addonController.daemonSetinformer = k8sinformer.Apps().V1().DaemonSets().Informer()
	addonController.replicaSetinformer = k8sinformer.Apps().V1().ReplicaSets().Informer()
	addonController.statefulSetinformer = k8sinformer.Apps().V1().StatefulSets().Informer()

	addonController.setupaddonhandlers()
	addonController.setupwfhandlers(ctx)
	// addonController.setupresourcehandlers(ctx)

	stopCh := make(<-chan struct{})
	go addonController.informer.Run(stopCh)
	go addonController.wfinformer.Run(stopCh)
	go addonController.nsinformer.Run(stopCh)
	go addonController.srvinformer.Run(stopCh)
	go addonController.configMapinformer.Run(stopCh)
	go addonController.clusterRoleinformer.Run(stopCh)
	go addonController.clusterRoleBindingInformer.Run(stopCh)
	go addonController.jobinformer.Run(stopCh)
	go addonController.cronjobinformer.Run(stopCh)
	go addonController.daemonSetinformer.Run(stopCh)
	go addonController.replicaSetinformer.Run(stopCh)
	go addonController.statefulSetinformer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, addonController.informer.HasSynced, addonController.wfinformer.HasSynced) {
		fmt.Printf("failed wait for sync.")
	}
	go wait.Until(addonController.runWorker, time.Second, stopCh)
	return addonController
}
