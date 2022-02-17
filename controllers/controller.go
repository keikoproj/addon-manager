package controllers

import (
	"context"
	"fmt"
	"sync"
	"time"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	informers "github.com/argoproj/argo-workflows/v3/pkg/client/informers/externalversions"
	v1alpha1 "github.com/argoproj/argo-workflows/v3/pkg/client/informers/externalversions/workflow/v1alpha1"
	addonv1 "github.com/keikoproj/addon-manager/pkg/apis/addon/v1alpha1"
	addonv1versioned "github.com/keikoproj/addon-manager/pkg/client/clientset/versioned"
	"github.com/keikoproj/addon-manager/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers/internalinterfaces"
	"k8s.io/client-go/tools/cache"

	addonwfutility "github.com/keikoproj/addon-manager/pkg/workflows"
)

const (
	workflowResyncPeriod         = 20 * time.Minute
	LabelKeyControllerInstanceID = "workflows.argoproj.io/controller-instanceid"
)

type WfInformers struct {
	//nsInformers dynamicinformer.DynamicSharedInformerFactory
	nsInformers  cache.SharedIndexInformer
	config       CtrlConfig
	stopCh       <-chan struct{}
	apiclientset addonv1versioned.Interface
}

func NewWfInformers(nsInfo cache.SharedIndexInformer, ctrlConfig CtrlConfig, stopCh <-chan struct{}) *WfInformers {
	return &WfInformers{
		nsInformers: nsInfo,
		config:      ctrlConfig,
		stopCh:      stopCh,
	}

}

func (wfinfo *WfInformers) Start(ctx context.Context) error {
	cfg, err := common.InClusterConfig()
	if err != nil {
		return err
	}
	//cfg, _ := clientcmd.BuildConfigFromFlags("", "")
	wfcli := common.NewWFClient(cfg)
	if wfcli == nil {
		return fmt.Errorf("failed to create workflow client")
	}
	wfinformfactory := informers.NewSharedInformerFactory(wfcli, time.Second*30)
	wfinfo.apiclientset = common.NewAddonClient(cfg)
	wfinfo.nsInformers.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(oldObj, newObj interface{}) {
				wfinfo.handleWorkFlowUpdate(newObj, wfinformfactory.Argoproj().V1alpha1().Workflows())
			},

			AddFunc: func(obj interface{}) {
				wfinfo.handleWorkFlowAdd(obj, wfinformfactory.Argoproj().V1alpha1().Workflows())
			},
		},
	)

	go wfinfo.nsInformers.Run(wfinfo.stopCh)
	if ok := cache.WaitForCacheSync(wfinfo.stopCh, wfinfo.nsInformers.HasSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	<-wfinfo.stopCh
	return nil
}

func NewWorkflowInformer(dclient dynamic.Interface, ns string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	resource := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "workflows",
	}
	informer := NewFilteredUnstructuredInformer(
		resource,
		dclient,
		ns,
		resyncPeriod,
		indexers,
		tweakListOptions,
	)
	return informer
}

func NewFilteredUnstructuredInformer(resource schema.GroupVersionResource, client dynamic.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	ctx := context.Background()
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.Resource(resource).Namespace(namespace).List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.Resource(resource).Namespace(namespace).Watch(ctx, options)
			},
		},
		&unstructured.Unstructured{},
		resyncPeriod,
		indexers,
	)
}

func InstanceIDRequirement(instanceID string) labels.Requirement {
	var instanceIDReq *labels.Requirement
	var err error
	if instanceID != "" {
		instanceIDReq, err = labels.NewRequirement(LabelKeyControllerInstanceID, selection.Equals, []string{instanceID})
	} else {
		instanceIDReq, err = labels.NewRequirement(LabelKeyControllerInstanceID, selection.DoesNotExist, nil)
	}
	if err != nil {
		panic(err)
	}
	return *instanceIDReq
}

func (wfinfo *WfInformers) handleWorkFlowUpdate(obj interface{}, informers v1alpha1.WorkflowInformer) {
	if err := addonwfutility.IsValidV1WorkFlow(obj); err != nil {
		fmt.Printf("not an expected workflow object %v", err)
		return
	}
	wfobj := &wfv1.Workflow{}
	_ = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.(*unstructured.Unstructured).UnstructuredContent(), wfobj)
	if wfobj.Status.Phase.Completed() {
		// check the associated addons and update its status
		fmt.Printf("\n %s/%s  WorkFlowUpdate event status.phase <%s>\n",
			wfobj.GetNamespace(),
			wfobj.GetName(),
			wfobj.Status.Phase)

		// find the Addon from the namespace and update its status accordingly
		addonName, lifecycle, err := addonwfutility.ExtractAddOnNameAndLifecycleStep(wfobj.GetName())
		if err != nil {
			return
		}
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			wfinfo.updateAddonStatus(wfobj.GetNamespace(), addonName, lifecycle, wfobj.Status.Phase, wg)
		}()
	}
}

func (wfinfo *WfInformers) handleWorkFlowAdd(obj interface{}, informers v1alpha1.WorkflowInformer) {
	if err := addonwfutility.IsValidV1WorkFlow(obj); err != nil {
		fmt.Printf("not an expected workflow object %v", err)
		return
	}
	wfobj := &wfv1.Workflow{}
	_ = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.(*unstructured.Unstructured).UnstructuredContent(), wfobj)

	// check the associated addons and update its status
	fmt.Printf("\n %s/%s  WorkFlowAdd event status.phase <%s>\n",
		wfobj.GetNamespace(),
		wfobj.GetName(),
		wfobj.Status.Phase)
	addonName, lifecycle, err := addonwfutility.ExtractAddOnNameAndLifecycleStep(wfobj.GetName())
	if err != nil {
		return
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		wfinfo.updateAddonStatus(wfobj.GetNamespace(), addonName, lifecycle, "Pending", wg)
	}()
}

func (wfinfo *WfInformers) updateAddonStatus(namespace, name, lifecycle string, lifecyclestatus wfv1.WorkflowPhase, wg *sync.WaitGroup) error {
	defer wg.Done()

	fmt.Printf("\n updating ns/addon %s/%s step %s status to %s\n", namespace, name, lifecycle, lifecyclestatus)
	// retry is needed
	addonobj, err := wfinfo.apiclientset.AddonmgrV1alpha1().Addons(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf(" failed to get the interested addon %s from ns %s, err %v", namespace, name, err)

	}

	if lifecycle == "prereqs" {
		addonobj.Status.Lifecycle.Prereqs = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
	} else if lifecycle == "install" {
		addonobj.Status.Lifecycle.Installed = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
	}
	_, err = wfinfo.apiclientset.AddonmgrV1alpha1().Addons(namespace).Update(context.TODO(), addonobj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf(" failed to update ns/addon %s/%s status, err %v", namespace, name, err)
	}
	fmt.Printf("\n successfully update ns/addon %s/%s step %s status to %s\n", namespace, name, lifecycle, lifecyclestatus)
	return nil
}
