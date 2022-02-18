package controllers

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/go-logr/logr"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	informers "github.com/argoproj/argo-workflows/v3/pkg/client/informers/externalversions"
	v1alpha1 "github.com/argoproj/argo-workflows/v3/pkg/client/informers/externalversions/workflow/v1alpha1"

	"github.com/keikoproj/addon-manager/pkg/apis/addon"
	addonv1 "github.com/keikoproj/addon-manager/pkg/apis/addon/v1alpha1"
	addonv1versioned "github.com/keikoproj/addon-manager/pkg/client/clientset/versioned"
	addonv1informers "github.com/keikoproj/addon-manager/pkg/client/informers/externalversions"
	addonv1listers "github.com/keikoproj/addon-manager/pkg/client/listers/addon/v1alpha1"
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
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"

	addonwfutility "github.com/keikoproj/addon-manager/pkg/workflows"
)

const (
	workflowResyncPeriod         = 20 * time.Minute
	LabelKeyControllerInstanceID = "workflows.argoproj.io/controller-instanceid"
)

type WfInformers struct {
	stopCh <-chan struct{}
	log    logr.Logger

	k8sclient client.Client
	dynClient dynamic.Interface
	recorder  record.EventRecorder

	nsInformers cache.SharedIndexInformer // workflow event

	apiclientset   addonv1versioned.Interface //addon api client
	addonlister    addonv1listers.AddonLister //addon lister
	addonInformers cache.SharedIndexInformer  // addon dynamic informer
}

func NewWfInformers(nsInfo cache.SharedIndexInformer, stopCh <-chan struct{}, log logr.Logger, k8scli client.Client, dynClient dynamic.Interface) *WfInformers {
	return &WfInformers{
		nsInformers: nsInfo,
		stopCh:      stopCh,
		log:         log,
		k8sclient:   k8scli,
		dynClient:   dynClient,
	}

}

func NewAddonInformer(dclient dynamic.Interface, ns string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	resource := schema.GroupVersionResource{
		Group:    addon.Group,
		Version:  addon.Version,
		Resource: addon.AddonPlural,
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

func (wfinfo *WfInformers) startAddonInformers(cfg *rest.Config) {
	addoncli := common.NewAddonClient(cfg)
	if addoncli == nil {
		panic("addon cli is empty")
	}
	addonInformFactory := addonv1informers.NewSharedInformerFactory(addoncli, time.Second*30)
	wfinfo.addonlister = addonInformFactory.Addonmgr().V1alpha1().Addons().Lister()
	wfinfo.addonInformers = NewAddonInformer(wfinfo.dynClient, addon.ManagedNameSpace,
		addon.AddonResyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		internalinterfaces.TweakListOptionsFunc(func(x *metav1.ListOptions) {
		}),
	)

	go wfinfo.addonInformers.Run(wfinfo.stopCh)
	addonInformFactory.Start(wfinfo.stopCh)
}

func (wfinfo *WfInformers) Start(ctx context.Context) error {
	cfg, err := common.InClusterConfig()
	if err != nil {
		panic(err)
	}

	addoncli := common.NewAddonClient(cfg)
	if addoncli == nil {
		return fmt.Errorf("failed to create addon client")
	}
	wfinfo.apiclientset = addoncli
	wfinfo.startAddonInformers(cfg)

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

// dedicated workflow add/update event handler
func (wfinfo *WfInformers) handleWorkFlowUpdate(obj interface{}, informers v1alpha1.WorkflowInformer) {
	if err := addonwfutility.IsValidV1WorkFlow(obj); err != nil {
		fmt.Printf("not an expected workflow object %v", err)
		return
	}
	wfobj := &wfv1.Workflow{}
	_ = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.(*unstructured.Unstructured).UnstructuredContent(), wfobj)

	// check the associated addons and update its status
	msg := fmt.Sprintf("workflow %s/%s update status.phase %s", wfobj.GetNamespace(), wfobj.GetName(), wfobj.Status.Phase)
	wfinfo.log.Info(msg)

	if len(string(wfobj.Status.Phase)) == 0 {
		wfinfo.log.Info("skip workflow %s/%s empty status update.", wfobj.GetNamespace(), wfobj.GetName())
		return
	}

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

func (wfinfo *WfInformers) handleWorkFlowAdd(obj interface{}, informers v1alpha1.WorkflowInformer) {
	if err := addonwfutility.IsValidV1WorkFlow(obj); err != nil {
		fmt.Printf("not an expected workflow object %v", err)
		return
	}
	wfobj := &wfv1.Workflow{}
	_ = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.(*unstructured.Unstructured).UnstructuredContent(), wfobj)

	// check the associated addons and update its status
	msg := fmt.Sprintf("workflow %s/%s update status.phase %s", wfobj.GetNamespace(), wfobj.GetName(), wfobj.Status.Phase)
	wfinfo.log.Info(msg)
	if len(string(wfobj.Status.Phase)) == 0 {
		msg := fmt.Sprintf("skip %s/%s workflow empty status.", wfobj.GetNamespace(),
			wfobj.GetName())
		wfinfo.log.Info(msg)
		return
	}
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

func (wfinfo *WfInformers) updateAddonStatus(namespace, name, lifecycle string, lifecyclestatus wfv1.WorkflowPhase, wg *sync.WaitGroup) error {
	defer wg.Done()

	msg := fmt.Sprintf("updating addon %s/%s step %s status to %s\n", namespace, name, lifecycle, lifecyclestatus)
	wfinfo.log.Info(msg)

	addonobj, err := wfinfo.addonlister.Addons(namespace).Get(name)
	if err != nil || addonobj == nil {
		msg := fmt.Sprintf("failed getting addon %s/%s, err %v", namespace, name, err)
		fmt.Print(msg)
		return fmt.Errorf(msg)

	}
	updating := addonobj.DeepCopy()
	cycle := updating.Status.Lifecycle
	msg = fmt.Sprintf("addon %s/%s cycle %v", updating.Namespace, updating.Name, cycle)
	wfinfo.log.Info(msg)
	updating.Status.Lifecycle = addonv1.AddonStatusLifecycle{}
	if lifecycle == "prereqs" {
		updating.Status.Lifecycle.Prereqs = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
		updating.Status.Lifecycle.Installed = addonobj.Status.Lifecycle.Installed
	} else if lifecycle == "install" {
		updating.Status.Lifecycle.Installed = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
		updating.Status.Lifecycle.Prereqs = addonobj.Status.Lifecycle.Prereqs
	}

	if reflect.DeepEqual(updating.Status.Lifecycle, addonobj.Status.Lifecycle) {
		msg := fmt.Sprintf("addon %s/%s lifecycle the same. skip update.", updating.Namespace, updating.Name)
		wfinfo.log.Info(msg)
		return nil
	}
	updated, err := wfinfo.apiclientset.AddonmgrV1alpha1().Addons(updating.Namespace).UpdateStatus(context.Background(), updating,
		metav1.UpdateOptions{})
	if err != nil {
		msg := fmt.Sprintf("Addon %s/%s status could not be updated. %v", updating.Namespace, updating.Name, err)
		fmt.Print(msg)
	}
	cycle = updated.Status.Lifecycle
	msg = fmt.Sprintf("addon %s/%s updated cycle %v", updating.Namespace, updating.Name, cycle)
	wfinfo.log.Info(msg)
	msg = fmt.Sprintf("successfully update addon %s/%s step %s status to %s", namespace, name, lifecycle, lifecyclestatus)
	wfinfo.log.Info(msg)
	return nil
}
