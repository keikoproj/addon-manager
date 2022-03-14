package controllers

// controllers functional tests
import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	"io/ioutil"
	"testing"

	wfclientsetfake "github.com/argoproj/argo-workflows/v3/pkg/client/clientset/versioned/fake"
	addoninternal "github.com/keikoproj/addon-manager/pkg/addon"
	fakeAddonCli "github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/fake"
	"github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/scheme"
	"github.com/keikoproj/addon-manager/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicFake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	addonv1 "github.com/keikoproj/addon-manager/api/addon"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	addonapiv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/utils"
)

var (
	testNamespace = "default"
	testAddon     = "cluster-autoscaler"
)

func configureCRD(dyCli dynamic.Interface) error {
	var addonCRD = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1beta1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"name": "addons.addonmgr.keikoproj.io",
				"spec": map[string]interface{}{
					"group": addonv1.Group,
					"names": map[string]interface{}{
						"kind":   addonv1.AddonKind,
						"plural": addonv1.AddonPlural,
					},
					"scope":   "Namespaced",
					"version": "v1alpha1",
				},
			},
		}}

	CRDSchema := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1beta1", Resource: "customresourcedefinitions"}
	_, err := dyCli.Resource(CRDSchema).Create(context.Background(), addonCRD, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("failed creating Addon CRDs.")
		return err
	}
	return nil
}

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

	stopCh := make(<-chan struct{})
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

	if !cache.WaitForCacheSync(stopCh, controller.HasSynced) {
		fmt.Printf("failed wait for sync.")
	}
	return controller
}

func TestAddonInstall(t *testing.T) {
	RegisterFailHandler(Fail)
	logger := logrus.WithField("test-controllers", "addon")
	ctx := context.TODO()

	addonYaml, err := ioutil.ReadFile("./tests/clusterautoscaler.yaml")
	Expect(err).To(BeNil())

	instance, err := parseAddonYaml(addonYaml)
	Expect(err).ToNot(HaveOccurred())
	instance.SetName(testAddon)
	instance.SetNamespace(testNamespace)
	Expect(instance).To(BeAssignableToTypeOf(&addonapiv1.Addon{}))

	// install addon
	logger.Info("installing addon")
	addonController := newController(instance)

	logger.Info("prcoessing addon")
	processed := addonController.processNextItem(ctx)
	Expect(processed).To(BeTrue())

	// verify addon checksum
	logger.Info("fetching Addon")
	fetchedAddon, err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, testAddon, metav1.GetOptions{})
	Expect(err).To(BeNil())
	Expect(fetchedAddon).NotTo(BeNil())
	logger.Info("verifying Addon finalizer and checksum")
	// verify finalizer is set
	Expect(fetchedAddon.ObjectMeta.Finalizers).NotTo(BeZero())
	// verify checksum
	Expect(fetchedAddon.Status.Checksum).ShouldNot(BeEmpty())

	logger.Info("verifying Addon checksum changes.")
	oldCheckSum := fetchedAddon.Status.Checksum
	//Update instance params for checksum validation
	fetchedAddon.Spec.Params.Context.ClusterRegion = "us-east-2"
	err = addonController.handleAddonUpdate(ctx, fetchedAddon)
	fmt.Printf("\n err %#v\n", err)
	Expect(err).To(BeNil())

	updated, err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, testAddon, metav1.GetOptions{})
	Expect(err).To(BeNil())
	Expect(updated).NotTo(BeNil())
	Expect(updated.Status.Checksum).ShouldNot(BeIdenticalTo(oldCheckSum))

	logger.Info("verifying workflow generated.")
	wfName := updated.GetFormattedWorkflowName(addonapiv1.Prereqs)
	fetchedwf, err := addonController.wfcli.ArgoprojV1alpha1().Workflows(testNamespace).Get(ctx, wfName, metav1.GetOptions{})
	Expect(err).To(BeNil())
	Expect(fetchedwf.GetName()).Should(Equal(wfName))

	logger.Info("verifying workflow deleted.")
	err = addonController.wfcli.ArgoprojV1alpha1().Workflows(testNamespace).Delete(ctx, wfName, metav1.DeleteOptions{})
	Expect(err).To(BeNil())
	wf, err := addonController.wfcli.ArgoprojV1alpha1().Workflows(testNamespace).Get(ctx, wfName, metav1.GetOptions{})
	Expect(err).NotTo(BeNil())
	Expect(wf).To(BeNil())
}

func parseAddonYaml(data []byte) (*addonapiv1.Addon, error) {
	var err error
	o := &unstructured.Unstructured{}
	err = yaml.Unmarshal(data, &o.Object)
	if err != nil {
		return nil, err
	}
	a := &addonapiv1.Addon{}
	err = scheme.Scheme.Convert(o, a, 0)
	if err != nil {
		return nil, err
	}

	return a, nil
}

func TestAddonWF(t *testing.T) {

}
