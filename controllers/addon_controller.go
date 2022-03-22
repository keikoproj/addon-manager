package controllers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	ctrlruntimecache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/keikoproj/addon-manager/api/addon"
	addonapiv1 "github.com/keikoproj/addon-manager/api/addon"
	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	addonv1versioned "github.com/keikoproj/addon-manager/pkg/client/clientset/versioned"

	"github.com/keikoproj/addon-manager/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache" // tools-cache
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	"github.com/keikoproj/addon-manager/pkg/common"

	wfclientset "github.com/argoproj/argo-workflows/v3/pkg/client/clientset/versioned"
	addoninternal "github.com/keikoproj/addon-manager/pkg/addon"
	batch_v1 "k8s.io/api/batch/v1"
	batch_v1beta1 "k8s.io/api/batch/v1beta1"

	rbac_v1 "k8s.io/api/rbac/v1"
)

const (
	maxRetries           = 5
	workflowResyncPeriod = 20 * time.Minute
)

var serverStartTime time.Time

// Event indicate the informerEvent
type Event struct {
	key       string
	eventType string
}

// Controller object
type Controller struct {
	//logger    *logrus.Entry
	logger       logr.Logger
	clientset    kubernetes.Interface
	queue        workqueue.RateLimitingInterface
	config       *rest.Config
	runtimecache ctrlruntimecache.Cache

	addoninformer      cache.SharedIndexInformer
	wfinformer         cache.SharedIndexInformer
	nsinformer         cache.SharedIndexInformer
	deploymentinformer cache.SharedIndexInformer

	srvinformer                cache.SharedIndexInformer
	configMapinformer          cache.SharedIndexInformer
	clusterRoleinformer        cache.SharedIndexInformer
	clusterRoleBindingInformer cache.SharedIndexInformer
	jobinformer                cache.SharedIndexInformer
	srvAcntinformer            cache.SharedIndexInformer
	cronjobinformer            cache.SharedIndexInformer
	daemonSetinformer          cache.SharedIndexInformer
	replicaSetinformer         cache.SharedIndexInformer
	statefulSetinformer        cache.SharedIndexInformer

	dynCli   dynamic.Interface
	addoncli addonv1versioned.Interface
	wfcli    wfclientset.Interface

	client client.Client
	mgr    manager.Manager

	recorder record.EventRecorder

	namespace string
	scheme    *runtime.Scheme

	versionCache addoninternal.VersionCacheClient
	wftlock      sync.Mutex
}

//compose addon informer
func newAddonInformer(ctx context.Context, dynCli dynamic.Interface, namespace string) cache.SharedIndexInformer {
	resource := schema.GroupVersionResource{
		Group:    addonapiv1.Group,
		Version:  "v1alpha1",
		Resource: addonapiv1.AddonPlural,
	}
	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return dynCli.Resource(resource).Namespace(namespace).List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return dynCli.Resource(resource).Namespace(namespace).Watch(ctx, options)
			},
		},
		&unstructured.Unstructured{},
		0, //Skip resync
		cache.Indexers{},
	)
	return informer
}

// func newAddonInformer(ctx context.Context, dynCli dynamic.Interface, namespace string, config *rest.Config) cache.SharedIndexInformer {
// 	addongvk := schema.GroupVersionKind{
// 		Group:   addonapiv1.Group,
// 		Version: "v1alpha1",
// 		Kind:    addonapiv1.AddonKind,
// 	}
// 	mapper, err := apiutil.NewDiscoveryRESTMapper(config)
// 	if err != nil {
// 		panic(err)
// 	}
// 	mapping, err := mapper.RESTMapping(addongvk.GroupKind(), addongvk.Version)
// 	if err != nil {
// 		panic(err)
// 	}

// 	informer := cache.NewSharedIndexInformer(
// 		&cache.ListWatch{
// 			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
// 				return dynCli.Resource(mapping.Resource).Namespace(namespace).List(ctx, options)

// 			},
// 			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
// 				return dynCli.Resource(mapping.Resource).Namespace(namespace).Watch(ctx, options)
// 			},
// 		},
// 		&unstructured.Unstructured{},
// 		0, //Skip resync
// 		cache.Indexers{},
// 	)
// 	return informer
// }

// addon dependent resources informers
func NewResourceInformers(ctx context.Context, kubeClient kubernetes.Interface, namespace string) map[string]cache.SharedIndexInformer {
	resourceInformers := make(map[string]cache.SharedIndexInformer)
	// addon resources namespace informers
	nsinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kubeClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kubeClient.CoreV1().Namespaces().Watch(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&v1.Namespace{},
		0, //Skip resync
		cache.Indexers{},
	)

	deploymentinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kubeClient.AppsV1().Deployments("").List(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kubeClient.AppsV1().Deployments("").Watch(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&appsv1.Deployment{},
		0, //Skip resync
		cache.Indexers{},
	)

	srvinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kubeClient.CoreV1().Services("").List(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kubeClient.CoreV1().Services("").Watch(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&v1.Service{},
		0, //Skip resync
		cache.Indexers{},
	)

	configMapinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kubeClient.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kubeClient.CoreV1().ConfigMaps("").Watch(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&v1.ConfigMap{},
		0, //Skip resync
		cache.Indexers{},
	)

	clusterRoleinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kubeClient.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kubeClient.RbacV1().ClusterRoles().Watch(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&rbac_v1.ClusterRole{},
		0, //Skip resync
		cache.Indexers{},
	)

	clusterRoleBindingInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kubeClient.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kubeClient.RbacV1().ClusterRoleBindings().Watch(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&rbac_v1.ClusterRoleBinding{},
		0, //Skip resync
		cache.Indexers{},
	)

	srvAcntinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kubeClient.CoreV1().ServiceAccounts(namespace).List(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kubeClient.CoreV1().ServiceAccounts(namespace).Watch(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&v1.ServiceAccount{},
		0, //Skip resync
		cache.Indexers{},
	)

	jobinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kubeClient.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kubeClient.BatchV1().Jobs(namespace).Watch(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&batch_v1.Job{},
		0, //Skip resync
		cache.Indexers{},
	)

	cronjobinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kubeClient.BatchV1beta1().CronJobs(namespace).List(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kubeClient.BatchV1beta1().CronJobs(namespace).Watch(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&batch_v1beta1.CronJob{},
		0, //Skip resync
		cache.Indexers{},
	)

	daemonSetinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kubeClient.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kubeClient.AppsV1().DaemonSets(namespace).Watch(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&appsv1.DaemonSet{},
		0, //Skip resync
		cache.Indexers{},
	)

	replicaSetinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kubeClient.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kubeClient.AppsV1().ReplicaSets(namespace).Watch(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&appsv1.ReplicaSet{},
		0, //Skip resync
		cache.Indexers{},
	)

	statefulSetinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kubeClient.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kubeClient.AppsV1().StatefulSets(namespace).Watch(ctx, metav1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&appsv1.StatefulSet{},
		0, //Skip resync
		cache.Indexers{},
	)
	resourceInformers["namespace"] = nsinformer
	resourceInformers["deployment"] = deploymentinformer
	resourceInformers["service"] = srvinformer
	resourceInformers["configmap"] = configMapinformer
	resourceInformers["clusterrole"] = clusterRoleinformer
	resourceInformers["clusterRoleBinding"] = clusterRoleBindingInformer
	resourceInformers["serviceAccount"] = srvAcntinformer
	resourceInformers["job"] = jobinformer
	resourceInformers["cronjob"] = cronjobinformer
	resourceInformers["daemonSet"] = daemonSetinformer
	resourceInformers["replicaSet"] = replicaSetinformer
	resourceInformers["statefulSet"] = statefulSetinformer
	return resourceInformers
}

func New(ctx context.Context, mgr manager.Manager, stopChan <-chan struct{}) {
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

	ctrlruntimeclient := mgr.GetClient()
	logger := mgr.GetLogger().WithName("addon-manager-controller")
	eventRecorder := mgr.GetEventRecorderFor("addon-manager-controller")
	c := newResourceController(kubeClient, dynCli, addoncli, wfcli, ctrlruntimeclient, "addon", "addon-manager-system",
		logger, eventRecorder, mgr.GetConfig(), mgr.GetCache())
	mgr.Add(c)
}

func newResourceController(kubeClient kubernetes.Interface, dynCli dynamic.Interface,
	addoncli addonv1versioned.Interface, wfcli wfclientset.Interface,
	ctrlruntimeclient client.Client,
	resourceType, namespace string,
	logger logr.Logger,
	eventRecorder record.EventRecorder,
	config *rest.Config,
	runtimecache ctrlruntimecache.Cache) *Controller {
	c := &Controller{
		logger:       logger,
		clientset:    kubeClient,
		dynCli:       dynCli,
		addoncli:     addoncli,
		wfcli:        wfcli,
		client:       ctrlruntimeclient,
		namespace:    namespace,
		recorder:     eventRecorder,
		config:       config,
		runtimecache: runtimecache,
	}
	c.queue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	c.wftlock = sync.Mutex{}
	return c
}

func (c *Controller) setupaddonhandlers() {
	var newEvent Event
	var err error
	resourceType := "addon"

	c.addoninformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			newEvent.key, err = cache.MetaNamespaceKeyFunc(obj)
			newEvent.eventType = "create"

			if err == nil {
				logrus.WithField("controllers", "addon").Infof("Processing add to %v: %s", resourceType, newEvent.key)
				c.queue.Add(newEvent)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			oldaddon, newaddon := old.(*unstructured.Unstructured), new.(*unstructured.Unstructured)
			if oldaddon.GetResourceVersion() == newaddon.GetResourceVersion() {
				return
			}
			newEvent.key, err = cache.MetaNamespaceKeyFunc(new)
			newEvent.eventType = "update"

			if err == nil {
				logrus.WithField("controllers", "addon").Infof("Processing update to %v: %s", resourceType, newEvent.key)
				c.queue.Add(newEvent)
			}
		},
		DeleteFunc: func(obj interface{}) {
			newEvent.key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			newEvent.eventType = "delete"

			if err == nil {
				logrus.WithField("controllers", "addon").Infof("Processing delete to %v: %s", resourceType, newEvent.key)
				c.queue.Add(newEvent)
			}
		},
	})
}

func (c *Controller) setupwfhandlers(ctx context.Context) {
	var newEvent Event
	resourceType := "workflow"

	c.wfinformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "create"

				logrus.WithField("controllers", "workflow").Infof("Processing add to %v: %s", resourceType, newEvent.key)
				c.handleWorkFlowAdd(ctx, obj)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "update"

				logrus.WithField("controllers", "workflow").Infof("Processing update to %v: %s", resourceType, newEvent.key)
				c.handleWorkFlowUpdate(ctx, new)
			}
		},
	})
}

func (c *Controller) setupresourcehandlers(ctx context.Context) {
	var newEvent Event

	resourceType := "namespace"
	c.nsinformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				logrus.WithField("controllers", "namespace").Infof("Processing add to %v: %s", resourceType, newEvent.key)
				c.handleNamespaceAdd(ctx, obj)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "update"

				logrus.WithField("controllers", "namespace").Infof("Processing update to %v: %s", resourceType, newEvent.key)
				c.handleNamespaceUpdate(ctx, new)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "delete"
				c.handleNamespaceDeletion(ctx, obj)
			}
		},
	})

	resourceType = "deployment"
	c.deploymentinformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "create"

				logrus.WithField("controllers", "deployment").Infof("Processing add to %v: %s", resourceType, newEvent.key)
				c.handleDeploymentAdd(ctx, obj)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "update"

				logrus.WithField("controllers", "deployment").Infof("Processing update to %v: %s", resourceType, newEvent.key)
				c.handleDeploymentUpdate(ctx, new)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "delete"
				c.handleDeploymentDeletion(ctx, obj)
			}
		},
	})

	resourceType = "ServiceAccount"
	c.srvAcntinformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "create"

				logrus.WithField("controllers", "ServiceAccount").Infof("Processing add to %v: %s", resourceType, newEvent.key)
				c.handleServiceAccountAdd(ctx, obj)
			}

		},
		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "update"

				logrus.WithField("controllers", "ServiceAccount").Infof("Processing update to %v: %s", resourceType, newEvent.key)
				c.handleServiceAccountUpdate(ctx, new)
			}

		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "delete"
				c.handleServiceAccountDeletion(ctx, obj)
			}
		},
	})

	resourceType = "ConfigMap"
	c.configMapinformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "create"

				logrus.WithField("controllers", "ConfigMap").Infof("Processing add to %v: %s", resourceType, newEvent.key)
				c.handleConfigMapAdd(ctx, obj)
			}

		},
		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "update"

				logrus.WithField("controllers", "ConfigMap").Infof("Processing update to %v: %s", resourceType, newEvent.key)
				c.handleConfigMapUpdate(ctx, new)
			}

		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "delete"
				logrus.WithField("controllers", "ConfigMap").Infof("Processing delete to %v: %s", resourceType, newEvent.key)
				c.handleConfigMapDeletion(ctx, obj)
			}
		},
	})

	resourceType = "ClusterRole"
	c.clusterRoleinformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "create"

				logrus.WithField("controllers", "ClusterRole").Infof("Processing add to %v: %s", resourceType, newEvent.key)
				c.handleClusterRoleAdd(ctx, obj)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "update"

				logrus.WithField("controllers", "ClusterRole").Infof("Processing update to %v: %s", resourceType, newEvent.key)
				c.handleClusterRoleUpdate(ctx, new)
			}

		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "delete"
				logrus.WithField("controllers", "ClusterRole").Infof("Processing delete to %v: %s", resourceType, newEvent.key)
				c.handleClusterRoleDeletion(ctx, obj)
			}
		},
	})

	resourceType = "ClusterRoleBinding"
	c.clusterRoleBindingInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "create"

				logrus.WithField("controllers", "ClusterRoleBinding").Infof("Processing add to %v: %s", resourceType, newEvent.key)
				c.handleClusterRoleBindingAdd(ctx, obj)
			}

		},
		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "update"

				logrus.WithField("controllers", "ClusterRoleBinding").Infof("Processing update to %v: %s", resourceType, newEvent.key)
				c.handleClusterRoleBindingUpdate(ctx, new)
			}

		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "delete"
				logrus.WithField("controllers", "ClusterRoleBinding").Infof("Processing delete to %v: %s", resourceType, newEvent.key)
				c.handleClusterRoleBindingDeletion(ctx, obj)
			}
		},
	})

	resourceType = "Job"
	c.jobinformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "create"

				logrus.WithField("controllers", "Job").Infof("Processing add to %v: %s", resourceType, newEvent.key)
				c.handleJobAdd(ctx, obj)
			}

		},
		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "update"

				logrus.WithField("controllers", "Job").Infof("Processing update to %v: %s", resourceType, newEvent.key)
				c.handleJobAdd(ctx, new)
			}
		},
	})

	resourceType = "CronJob"
	c.cronjobinformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "create"

				logrus.WithField("controllers", "CronJob").Infof("Processing add to %v: %s", resourceType, newEvent.key)
				c.handleCronJobAdd(ctx, obj)
			}

		},
		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "update"

				logrus.WithField("controllers", "CronJob").Infof("Processing update to %v: %s", resourceType, newEvent.key)
				c.handleCronJobAdd(ctx, new)
			}
		},
	})

	resourceType = "DaemonSet"
	c.daemonSetinformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "create"

				logrus.WithField("controllers", "DaemonSet").Infof("Processing add to %v: %s", resourceType, newEvent.key)
				c.handleDaemonSet(ctx, obj)
			}

		},
		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "update"

				logrus.WithField("controllers", "DaemonSet").Infof("Processing update to %v: %s", resourceType, newEvent.key)
				c.handleDaemonSet(ctx, new)
			}
		},
	})

	resourceType = "ReplicaSet"
	c.replicaSetinformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "create"

				logrus.WithField("controllers", "ReplicaSet").Infof("Processing add to %v: %s", resourceType, newEvent.key)
				c.handleReplicaSet(ctx, obj)
			}

		},
		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "update"

				logrus.WithField("controllers", "ReplicaSet").Infof("Processing update to %v: %s", resourceType, newEvent.key)
				c.handleReplicaSet(ctx, new)
			}
		},
	})

	resourceType = "StatefulSet"
	c.statefulSetinformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "create"

				logrus.WithField("controllers", "StatefulSet").Infof("Processing add to %v: %s", resourceType, newEvent.key)
				c.handleStatefulSet(ctx, obj)
			}

		},
		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "update"

				logrus.WithField("controllers", "StatefulSet").Infof("Processing update to %v: %s", resourceType, newEvent.key)
				c.handleStatefulSet(ctx, new)
			}
		},
	})
}

// HasSynced is required for the cache.Controller interface.
func (c *Controller) HasSynced() bool {
	return c.addoninformer.HasSynced()
}

func (c *Controller) runWorker() {
	defer utilruntime.HandleCrash()

	ctx := context.Background()
	for c.processNextItem(ctx) {
		// continue looping
	}
}

func (c *Controller) processNextItem(ctx context.Context) bool {
	newEvent, quit := c.queue.Get()
	if quit {
		c.logger.Info("received shutdown message.")
		return false
	}
	defer c.queue.Done(newEvent)

	err := c.processItem(ctx, newEvent.(Event))
	if err == nil {
		c.queue.Forget(newEvent)
	} else if c.queue.NumRequeues(newEvent) < maxRetries {
		c.logger.Error(err, "Error processing %s (will retry): %v", newEvent.(Event).key, err)
		c.queue.AddRateLimited(newEvent)
	} else {
		c.logger.Error(err, "Error processing %s (giving up): %v", newEvent.(Event).key, err)
		c.queue.Forget(newEvent)
		c.queue.Done(newEvent)
		utilruntime.HandleError(err)
	}

	return true
}

func (c *Controller) Start(ctx context.Context) error {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c.logger.Info("Starting keiko addon-manager controller")
	serverStartTime = time.Now().Local()

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

	if err := c.initController(ctx); err != nil {
		c.logger.Error(err, "[Run] pre-process addon(s)")
	}

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

	if !c.runtimecache.WaitForCacheSync(ctx) {
		utilruntime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		panic("failed sync cache.")
	}
	c.logger.Info("Keiko addon-manager controller synced and ready")

	stopCh := make(chan struct{})
	for i := 0; i < 5; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}
	<-ctx.Done()
	return nil
}

func (c *Controller) processItem(ctx context.Context, newEvent Event) error {
	obj, exists, err := c.addoninformer.GetIndexer().GetByKey(newEvent.key)
	if err != nil {
		msg := fmt.Sprintf("failed fetching key %s from cache, err %v ", newEvent.key, err)
		c.logger.Error(err, msg)
		return fmt.Errorf(msg)
	} else if !exists {
		if newEvent.eventType == "delete" {
			return nil
		}
		msg := fmt.Sprintf("event %s obj %s does not exist", newEvent.eventType, newEvent.key)
		_, addonName := c.namespacenameFromKey(newEvent.key)
		c.removeFromCache(addonName)
		c.logger.Error(err, msg)
		return fmt.Errorf(msg)
	}

	addon, err := common.FromUnstructured(obj.(*unstructured.Unstructured))
	if err != nil {
		msg := fmt.Sprintf("obj %s is not addon, err %v", newEvent.key, err)
		c.logger.Error(err, msg)
		return fmt.Errorf(msg)
	}

	// process events based on its type
	switch newEvent.eventType {
	case "create":
		return c.handleAddonCreation(ctx, addon)
	case "update":
		return c.handleAddonUpdate(ctx, addon)
	case "delete":
		return c.handleAddonDeletion(ctx, addon)
	}
	return nil
}

func (c *Controller) initController(ctx context.Context) error {
	c.logger.Info("initController pre-process addon every restart.")

	addonList, err := c.addoncli.AddonmgrV1alpha1().Addons(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		c.logger.Error(err, " failed list addons")
		panic(err)
	}

	for _, item := range addonList.Items {
		// retrieve status from install wf
		if item.Spec.Lifecycle.Install.Template != "" {
			wfIdentifierName := fmt.Sprintf("%s-%s-%s-wf", item.Name, "install", item.CalculateChecksum())
			wf, err := c.wfcli.ArgoprojV1alpha1().Workflows(item.Namespace).Get(ctx, wfIdentifierName, metav1.GetOptions{})
			if err == nil && wf != nil {
				// align status

				item.Status.Lifecycle.Installed = addonv1.ApplicationAssemblyPhase(wf.Status.Phase)
				if item.Status.Lifecycle.Installed.Succeeded() {
					item.Status.Reason = ""
				}
				// mark complete label
				if item.Status.Lifecycle.Installed.Completed() {
					labels := item.GetLabels()
					if labels == nil {
						labels = map[string]string{}
					}
					labels[addonapiv1.AddonCompleteLabel] = addonapiv1.AddonCompleteTrueKey
					item.SetLabels(labels)
				}
				c.logger.Info(fmt.Sprintf("[initController] addon %s/%s install status %s", item.Namespace, item.Name, item.Status.Lifecycle.Installed))
			} else {
				// error case the install wf does not exist
				c.logger.WithValues("[initController]", fmt.Sprintf("failed get addon %s/%s install wf", item.Namespace, item.Name))
				if item.Status.Lifecycle.Installed.Completed() {
					labels := item.GetLabels()
					if labels == nil {
						labels = map[string]string{}
					}
					labels[addonapiv1.AddonCompleteLabel] = addonapiv1.AddonCompleteTrueKey
					item.SetLabels(labels)
				}
			}
		}

		if item.Spec.Lifecycle.Prereqs.Template != "" {
			wfIdentifierName := fmt.Sprintf("%s-%s-%s-wf", item.Name, "prereqs", item.CalculateChecksum())
			wf, err := c.wfcli.ArgoprojV1alpha1().Workflows(item.Namespace).Get(ctx, wfIdentifierName, metav1.GetOptions{})
			if err == nil && wf != nil {
				// reset lifecycle status
				item.Status.Lifecycle.Prereqs = addonv1.ApplicationAssemblyPhase(wf.Status.Phase)
				c.logger.Info(fmt.Sprintf("[initController] addon %s/%s prereqs status %s", item.Namespace, item.Name, item.Status.Lifecycle.Prereqs))
			} else {
				c.logger.Info(fmt.Sprintf("[initController] failed get addon %s/%s prereqs wf", item.Namespace, item.Name))
			}
		}

		if item.Spec.Lifecycle.Prereqs.Template == "" && item.Spec.Lifecycle.Install.Template == "" {
			c.logger.Info(fmt.Sprintf("[initController] addon %s/%s does not have lifecycle.", item.Namespace, item.Name))
			item.Status.Lifecycle.Installed = addonv1.Succeeded
			item.Status.Reason = ""

			labels := item.GetLabels()
			if labels == nil {
				labels = map[string]string{}
			}
			labels[addonapiv1.AddonCompleteLabel] = addonapiv1.AddonCompleteTrueKey
			item.SetLabels(labels)
		}

		// might stuck on dependency
		if item.Status.Lifecycle.Installed == addonv1.DepPending || item.Status.Lifecycle.Installed == addonv1.ValidationFailed {
			c.logger.Info(fmt.Sprintf("[initController] addon %s/%s stuck on dependency.", item.Namespace, item.Name))
		}

		item.Finalizers = c.mergeFinalizer(item.Finalizers, []string{addonapiv1.FinalizerName})
		err := c.updateAddon(ctx, &item)
		if err != nil {
			c.logger.Error(err, fmt.Sprintf("[initController] failed patch addon %s/%s labels.", item.Namespace, item.Name))
		}
		c.logger.Info(fmt.Sprintf("[initController] pre-press addon %s/%s successfully. adding into cache.", item.Namespace, item.Name))
		c.addAddonToCache(&item)
	}
	c.logger.Info("initController pre-process end.")
	return nil
}

func NewCachingClient(cache ctrlruntimecache.Cache, config *rest.Config, options client.Options, uncachedObjects ...client.Object) (client.Client, error) {
	c, err := client.New(config, options)
	if err != nil {
		return nil, err
	}

	return client.NewDelegatingClient(client.NewDelegatingClientInput{
		CacheReader:     cache,
		Client:          c,
		UncachedObjects: uncachedObjects,

		CacheUnstructured: true,
	})
}

func ResourcetweakListOptions() string {
	req, _ := labels.NewRequirement(addon.ResourceDefaultManageByLabel, selection.Equals, []string{addon.ResourceDefaultManageByValue})
	return labels.NewSelector().Add(*req).String()

}
