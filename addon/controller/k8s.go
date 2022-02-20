package controller

import (
	"context"
	"os"
	"time"

	addonv1 "github.com/keikoproj/addon-manager/pkg/apis/addon"
	"github.com/keikoproj/addon-manager/pkg/client/informers/externalversions/internalinterfaces"
	"github.com/keikoproj/addon-manager/pkg/common"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
)

const (
	Group            string = "argoproj.io"
	Version          string = "v1alpha1"
	APIVersion       string = Group + "/" + Version
	WorkflowKind     string = "Workflow"
	WorkflowPlural   string = "workflows"
	WorkflowFullName string = WorkflowPlural + "." + Group

	LabelKeyCompleted               = WorkflowFullName + "/completed"
	LabelKeyWorkflowArchivingStatus = WorkflowFullName + "/workflow-archiving-status"
	LabelKeyControllerInstanceID    = WorkflowFullName + "/controller-instanceid"

	LabelKeyManagedBy = "app.kubernetes.io/managed-by"
	LabelKeyAddonName = "app.kubernetes.io/name"
	LabelKeyAppName   = "app"
)

func LookupEnvDurationOr(key string, o time.Duration) time.Duration {
	v, found := os.LookupEnv(key)
	if found && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			log.WithField(key, v).WithError(err).Panic("failed to parse")
		} else {
			return d
		}
	}
	return o
}

func NewWorkflowInformer(dclient dynamic.Interface, ns string, resyncPeriod time.Duration, tweakListOptions internalinterfaces.TweakListOptionsFunc, indexers cache.Indexers) cache.SharedIndexInformer {
	resource := schema.GroupVersionResource{
		Group:    Group,
		Version:  "v1alpha1",
		Resource: WorkflowPlural,
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

func NewAddonInformer(dclient dynamic.Interface, ns string, resyncPeriod time.Duration, tweakListOptions internalinterfaces.TweakListOptionsFunc, indexers cache.Indexers) cache.SharedIndexInformer {
	resource := schema.GroupVersionResource{
		Group:    addonv1.Group,
		Version:  "v1alpha1",
		Resource: addonv1.AddonPlural,
	}
	informer := NewFilteredUnstructuredInformer(
		resource,
		dclient,
		ns,
		resyncPeriod,
		indexers,
		tweakListOptions,
	)
	if informer == nil {
		panic("addon informer is nil")
	}
	return informer
}

func NewServiceInformer(dclient dynamic.Interface, ns string, resyncPeriod time.Duration, tweakListOptions internalinterfaces.TweakListOptionsFunc, indexers cache.Indexers) cache.SharedIndexInformer {
	resource := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "Services",
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

func NewUnstructuredInformer(resource schema.GroupVersionResource, client dynamic.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredUnstructuredInformer(resource, client, namespace, resyncPeriod, indexers, nil)
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

func UnstructuredHasCompletedLabel(obj interface{}) bool {
	if un, ok := obj.(*unstructured.Unstructured); ok {
		return un.GetLabels()[addonv1.LabelKeyCompleted] == "true"
	}
	return false
}

func UnstructuredHasFinalizer(obj interface{}) bool {
	if addon, ok := obj.(*unstructured.Unstructured); ok {
		return common.ContainsString(addon.GetFinalizers(), finalizerName)
	}
	return false
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
