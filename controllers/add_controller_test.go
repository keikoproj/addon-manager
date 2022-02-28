package controllers

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"

	"io/ioutil"
	"testing"

	wfclientsetfake "github.com/argoproj/argo-workflows/v3/pkg/client/clientset/versioned/fake"
	fakeAddonCli "github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/fake"
	"github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/scheme"
	"github.com/keikoproj/addon-manager/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	dynamicFake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/pointer"

	"k8s.io/client-go/kubernetes/fake"

	kubeinformers "k8s.io/client-go/informers"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	addonv1 "github.com/keikoproj/addon-manager/api/addon"

	addonapiv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	wfutility "github.com/keikoproj/addon-manager/pkg/workflows"
)

var (
	addonNamespace = "default"
	addonName      = "cluster-autoscaler"
	addonKey       = types.NamespacedName{Name: addonName, Namespace: addonNamespace}
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

	controller.informer = newAddonInformer(ctx, controller.dynCli, controller.namespace)
	controller.wfinformer = NewWorkflowInformer(controller.dynCli, controller.namespace, 0, cache.Indexers{}, tweakListOptions)
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
	addonNamespace = "default"
	addonName = "cluster-autoscaler"

	ctx := context.TODO()

	addonYaml, err := ioutil.ReadFile("clusterautoscaler.yaml")
	Expect(err).To(BeNil())

	instance, err := parseAddonYaml(addonYaml)
	Expect(err).ToNot(HaveOccurred())
	instance.SetName(addonName)
	instance.SetNamespace(addonNamespace)
	Expect(instance).To(BeAssignableToTypeOf(&addonapiv1.Addon{}))

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(instance)
	Expect(err).To(BeNil())
	un := &unstructured.Unstructured{Object: obj}
	un.SetAPIVersion("addonmgr.keikoproj.io/v1alpha1")
	un.SetKind("Addon")
	Expect(un.GetName()).To(Equal(addonName))
	Expect(un.GetNamespace()).To(Equal(addonNamespace))
	// install addon
	fmt.Printf("\ninstalling addon\n")
	addonController := newController(un)

	fmt.Printf("\nprcoessing addon\n")
	processed := addonController.processNextItem(ctx)
	Expect(processed).To(BeTrue())

	// generate workflow and update addon status
	var objects []runtime.Object
	wfcli := wfclientsetfake.NewSimpleClientset(objects...)
	err = generateWorkflow(wfcli, addonNamespace)
	Expect(err).To(BeNil())

	// verify addon checksum
	fmt.Printf("\nfetching Addon\n")
	fetchedAddon, err := addonController.addoncli.AddonmgrV1alpha1().Addons(addonNamespace).Get(ctx, addonName, metav1.GetOptions{})
	Expect(err).To(BeNil())
	Expect(fetchedAddon).NotTo(BeNil())
	fmt.Printf("\nverifying Addon\n")
	// verify finalizer is set
	Expect(fetchedAddon.ObjectMeta.Finalizers).To(BeZero())
	// verify checksum
	Expect(fetchedAddon.Status.Checksum).ShouldNot(BeEmpty())

	oldCheckSum := fetchedAddon.Status.Checksum
	//Update instance params for checksum validation
	fetchedAddon.Spec.Params.Context.ClusterRegion = "us-east-2"
	updated, err := addonController.addoncli.AddonmgrV1alpha1().Addons(addonNamespace).Update(ctx, fetchedAddon, metav1.UpdateOptions{})
	Expect(err).To(BeNil())
	Expect(updated).NotTo(BeNil())
	Expect(updated.Status.Checksum).ShouldNot(BeIdenticalTo(oldCheckSum))

	wfName := updated.GetFormattedWorkflowName(addonapiv1.Prereqs)
	fetchedwf, err := wfcli.ArgoprojV1alpha1().Workflows("default").Get(ctx, wfName, metav1.GetOptions{})
	Expect(err).To(BeNil())
	Expect(fetchedwf.GetName()).Should(Equal(wfName))

	err = wfcli.ArgoprojV1alpha1().Workflows("default").Delete(ctx, wfName, metav1.DeleteOptions{})
	Expect(err).To(BeNil())
	wf, err := wfcli.ArgoprojV1alpha1().Workflows("default").Get(ctx, wfName, metav1.GetOptions{})
	Expect(err).To(BeNil())
	Expect(wf).To(BeNil())
}

func generateWorkflow(wfcli *wfclientsetfake.Clientset, namespace string) error {
	prereqswf := &wfv1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "event-router-2-prereqs-5eecd98e-wf",
			Namespace: namespace,
			Annotations: map[string]string{
				"annotation": "value",
			},
			Labels: map[string]string{
				wfutility.WfInstanceIdLabelKey:    wfutility.WfInstanceId,
				"workflows.argoproj.io/completed": "true",
				"workflows.argoproj.io/phase":     "Succeeded",
			},
		},
		TypeMeta: metav1.TypeMeta{},
		Spec: wfv1.WorkflowSpec{
			HostNetwork:        pointer.BoolPtr(true),
			Entrypoint:         "good_entrypoint",
			ServiceAccountName: "my_service_account",
			TTLStrategy: &wfv1.TTLStrategy{
				SecondsAfterCompletion: pointer.Int32Ptr(10),
				SecondsAfterSuccess:    pointer.Int32Ptr(10),
				SecondsAfterFailure:    pointer.Int32Ptr(10),
			},
		},
		Status: wfv1.WorkflowStatus{
			Phase: wfv1.WorkflowSucceeded,
		},
	}
	wf, err := wfcli.ArgoprojV1alpha1().Workflows(namespace).Create(context.TODO(), prereqswf, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	if wf == nil {
		return fmt.Errorf("failed creating prereqs wf")
	}

	installwf := &wfv1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "event-router-2-install-5eecd98e-wf",
			Namespace: namespace,
			Annotations: map[string]string{
				"annotation": "value",
			},
			Labels: map[string]string{
				wfutility.WfInstanceIdLabelKey:    wfutility.WfInstanceId,
				"workflows.argoproj.io/completed": "true",
				"workflows.argoproj.io/phase":     "Succeeded",
			},
		},
		TypeMeta: metav1.TypeMeta{},
		Spec: wfv1.WorkflowSpec{
			HostNetwork:        pointer.BoolPtr(true),
			Entrypoint:         "good_entrypoint",
			ServiceAccountName: "my_service_account",
			TTLStrategy: &wfv1.TTLStrategy{
				SecondsAfterCompletion: pointer.Int32Ptr(10),
				SecondsAfterSuccess:    pointer.Int32Ptr(10),
				SecondsAfterFailure:    pointer.Int32Ptr(10),
			},
		},
		Status: wfv1.WorkflowStatus{
			Phase: wfv1.WorkflowSucceeded,
		},
	}
	installwf, err = wfcli.ArgoprojV1alpha1().Workflows(namespace).Create(context.TODO(), installwf, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	if installwf == nil {
		return fmt.Errorf("failed creating install wf")
	}
	return nil
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

func parseUnAddonYaml(data []byte) (*unstructured.Unstructured, error) {
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

	return o, nil
}
