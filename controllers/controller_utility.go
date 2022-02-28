package controllers

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	informers "github.com/argoproj/argo-workflows/v3/pkg/client/informers/externalversions"
	v1alpha1 "github.com/argoproj/argo-workflows/v3/pkg/client/informers/externalversions/workflow/v1alpha1"

	"github.com/keikoproj/addon-manager/api/addon"
	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	addonv1versioned "github.com/keikoproj/addon-manager/pkg/client/clientset/versioned"
	"github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/scheme"
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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"

	addonwfutility "github.com/keikoproj/addon-manager/pkg/workflows"

	apiv1 "k8s.io/api/core/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
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

func (wfinfo *WfInformers) Start(ctx context.Context) error {
	cfg, _ := common.InClusterConfig()
	addoncli := common.NewAddonClient(cfg)
	if addoncli == nil {
		return fmt.Errorf("failed to create addon client")
	}
	wfinfo.apiclientset = addoncli

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

func ResourcetweakListOptions() string {
	req, _ := labels.NewRequirement(addon.ResourceDefaultManageByLabel, selection.Equals, []string{addon.ResourceDefaultManageByValue})
	return labels.NewSelector().Add(*req).String()

}

func tweakListOptions(options *metav1.ListOptions) {
	// currently, I do not have any label
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
		wfinfo.log.Info("skip workflow ", wfobj.GetNamespace(), "/", wfobj.GetName(), "empty status update.")
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
	prevStatus := addonobj.Status
	cycle := prevStatus.Lifecycle
	msg = fmt.Sprintf("addon %s/%s cycle %v", updating.Namespace, updating.Name, cycle)
	wfinfo.log.Info(msg)

	newStatus := addonv1.AddonStatus{
		Lifecycle: addonv1.AddonStatusLifecycle{},
		Resources: []addonv1.ObjectStatus{},
	}
	if lifecycle == "prereqs" {
		newStatus.Lifecycle.Prereqs = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
		newStatus.Lifecycle.Installed = prevStatus.Lifecycle.Installed
	} else if lifecycle == "install" || lifecycle == "delete" {
		newStatus.Lifecycle.Installed = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
		newStatus.Lifecycle.Prereqs = prevStatus.Lifecycle.Prereqs
	}
	newStatus.Resources = append(newStatus.Resources, prevStatus.Resources...)
	newStatus.Checksum = prevStatus.Checksum
	newStatus.Reason = prevStatus.Reason
	newStatus.StartTime = prevStatus.StartTime

	if reflect.DeepEqual(newStatus, prevStatus) {
		msg := fmt.Sprintf("addon %s/%s lifecycle the same. skip update.", updating.Namespace, updating.Name)
		wfinfo.log.Info(msg)
		return nil
	}
	updating.Status = newStatus
	updated, err := wfinfo.apiclientset.AddonmgrV1alpha1().Addons(updating.Namespace).UpdateStatus(context.Background(), updating,
		metav1.UpdateOptions{})
	if err != nil {
		msg := fmt.Sprintf("Addon %s/%s status could not be updated. %v", updating.Namespace, updating.Name, err)
		fmt.Print(msg)
	}
	cycle = updated.Status.Lifecycle
	msg = fmt.Sprintf("addon %s/%s updated cycle %v", updating.Namespace, updating.Namespace, cycle)
	wfinfo.log.Info(msg)

	if lifecycle == "delete" && updating.Status.Lifecycle.Installed.Completed() {
		wfinfo.delAddon(updating.Namespace, updating.Namespace)
	}

	msg = fmt.Sprintf("successfully update addon %s/%s step %s status to %s", namespace, name, lifecycle, lifecyclestatus)
	wfinfo.log.Info(msg)
	return nil
}

func (wfinfo *WfInformers) delAddon(namespace, name string) error {
	delseconds := int64(0)
	err := wfinfo.apiclientset.AddonmgrV1alpha1().Addons(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{GracePeriodSeconds: &delseconds})
	if err != nil {
		msg := fmt.Sprintf("failed deleting addon %s/%s, err %v", namespace, name, err)
		fmt.Print(msg)
		return fmt.Errorf(msg)
	}
	msg := fmt.Sprintf("\n Successfully deleting addon %s/%s\n", namespace, name)
	fmt.Print(msg)
	return nil
}

const defaultSpamBurst = 10000

func createEventRecorder(namespace string, ks8cli kubernetes.Interface, logger *logrus.Entry) record.EventRecorder {
	eventCorrelationOption := record.CorrelatorOptions{BurstSize: defaultSpamBurst}
	eventBroadcaster := record.NewBroadcasterWithCorrelatorOptions(eventCorrelationOption)
	eventBroadcaster.StartLogging(logger.Debugf)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: ks8cli.CoreV1().Events(namespace)})
	return eventBroadcaster.NewRecorder(scheme.Scheme, apiv1.EventSource{Component: "addon-manager-controller"})
}

// filter completed addons
func UnstructuredHasCompletedLabel(obj interface{}) bool {
	if un, ok := obj.(*unstructured.Unstructured); ok {
		lifecycle, f, err := unstructured.NestedMap(un.UnstructuredContent(), "status", "lifecycle")
		if err == nil && f {
			// for updating/applying existing addons
			if lifecycle["installed"] == "Succeeded" || lifecycle["installed"] == "Failed" {
				return true
			}
			// check label : completed/true
		}
	}
	return false
}
