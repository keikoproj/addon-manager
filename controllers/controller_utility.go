package controllers

import (
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"

	"github.com/keikoproj/addon-manager/api/addon"
	addonv1versioned "github.com/keikoproj/addon-manager/pkg/client/clientset/versioned"
	"github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/scheme"
	addonv1listers "github.com/keikoproj/addon-manager/pkg/client/listers/addon/v1alpha1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"

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

func ResourcetweakListOptions() string {
	req, _ := labels.NewRequirement(addon.ResourceDefaultManageByLabel, selection.Equals, []string{addon.ResourceDefaultManageByValue})
	return labels.NewSelector().Add(*req).String()

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
