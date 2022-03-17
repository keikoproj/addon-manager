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
	"io/ioutil"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	// +kubebuilder:scaffold:imports

	wfclientsetfake "github.com/argoproj/argo-workflows/v3/pkg/client/clientset/versioned/fake"
	addonapiv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	addoninternal "github.com/keikoproj/addon-manager/pkg/addon"
	fakeAddonCli "github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/fake"
	"github.com/keikoproj/addon-manager/pkg/common"
	"github.com/keikoproj/addon-manager/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicFake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	addonNamespace = "default"
	addonName      = "cluster-autoscaler"
)

var (
	ctx             context.Context
	addonController *Controller
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"v1alpha1 Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx = context.TODO()

	addonYaml, err := ioutil.ReadFile("./tests/clusterautoscaler.yaml")
	Expect(err).To(BeNil())
	instance, err := parseAddonYaml(addonYaml)
	Expect(err).ToNot(HaveOccurred())
	instance.SetName(addonName)
	instance.SetNamespace(addonNamespace)
	Expect(instance).To(BeAssignableToTypeOf(&addonapiv1.Addon{}))

	addonController = newController(instance)
	Expect(addonController).NotTo(BeNil())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
})

func newController(options ...interface{}) *Controller {
	ctx := context.TODO()
	var objects []runtime.Object
	wfcli := wfclientsetfake.NewSimpleClientset(objects...)
	for _, opt := range options {
		switch v := opt.(type) {
		case *addonapiv1.Addon:
			objects = append(objects, v)
		case runtime.Object:
			objects = append(objects, v)
		}
	}

	kubecli := fake.NewSimpleClientset()
	addonCli := fakeAddonCli.NewSimpleClientset(objects...)
	dynCli := dynamicFake.NewSimpleDynamicClient(common.GetAddonMgrScheme(), objects...)
	k8sinformer := kubeinformers.NewSharedInformerFactory(kubecli, 0)

	controller := newResourceController(
		kubecli, dynCli, addonCli, wfcli,
		"addon", "default")

	controller.queue = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "addon-controller")
	controller.scheme = common.GetAddonMgrScheme()

	logger := logrus.WithField("controllers", "addon")
	controller.versionCache = addoninternal.NewAddonVersionCacheClient()
	controller.recorder = createEventRecorder(controller.namespace, controller.clientset, logger)
	controller.informer = newAddonInformer(ctx, controller.dynCli, controller.namespace)
	controller.wfinformer = utils.NewWorkflowInformer(controller.dynCli, controller.namespace, 0, cache.Indexers{}, utils.TweakListOptions)
	configureCRD(controller.dynCli)

	controller.nsinformer = k8sinformer.Core().V1().Namespaces().Informer()
	controller.deploymentinformer = k8sinformer.Apps().V1().Deployments().Informer()
	controller.srvinformer = k8sinformer.Core().V1().Services().Informer()
	controller.configMapinformer = k8sinformer.Core().V1().ConfigMaps().Informer()
	controller.clusterRoleinformer = k8sinformer.Rbac().V1().ClusterRoles().Informer()
	controller.clusterRoleBindingInformer = k8sinformer.Rbac().V1().ClusterRoleBindings().Informer()
	controller.jobinformer = k8sinformer.Batch().V1().Jobs().Informer()
	controller.cronjobinformer = k8sinformer.Core().V1().ServiceAccounts().Informer()
	controller.cronjobinformer = k8sinformer.Batch().V1().CronJobs().Informer()
	controller.daemonSetinformer = k8sinformer.Apps().V1().DaemonSets().Informer()
	controller.replicaSetinformer = k8sinformer.Apps().V1().ReplicaSets().Informer()
	controller.statefulSetinformer = k8sinformer.Apps().V1().StatefulSets().Informer()

	controller.setupaddonhandlers()
	controller.setupwfhandlers(ctx)
	controller.setupwfhandlers(ctx)

	stopCh := make(<-chan struct{})

	go controller.informer.Run(stopCh)
	go controller.wfinformer.Run(stopCh)
	go controller.nsinformer.Run(stopCh)
	go controller.srvinformer.Run(stopCh)
	go controller.configMapinformer.Run(stopCh)
	go controller.clusterRoleinformer.Run(stopCh)
	go controller.clusterRoleBindingInformer.Run(stopCh)
	go controller.jobinformer.Run(stopCh)
	go controller.cronjobinformer.Run(stopCh)
	go controller.daemonSetinformer.Run(stopCh)
	go controller.replicaSetinformer.Run(stopCh)
	go controller.statefulSetinformer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, controller.informer.HasSynced, controller.wfinformer.HasSynced) {
		fmt.Printf("failed wait for sync.")
	}

	return controller
}
