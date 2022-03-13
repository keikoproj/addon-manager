package controllers

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	addonapiv1 "github.com/keikoproj/addon-manager/api/addon"
	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	addonv1versioned "github.com/keikoproj/addon-manager/pkg/client/clientset/versioned"

	"github.com/keikoproj/addon-manager/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
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
	logger             *logrus.Entry
	clientset          kubernetes.Interface
	queue              workqueue.RateLimitingInterface
	informer           cache.SharedIndexInformer
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

	recorder record.EventRecorder

	namespace string
	scheme    *runtime.Scheme

	versionCache addoninternal.VersionCacheClient
}

// compose addon informer
func newAddonInformer(ctx context.Context, dynCli dynamic.Interface, namespace string) cache.SharedIndexInformer {
	resource := schema.GroupVersionResource{
		Group:    addonapiv1.Group,
		Version:  "v1alpha1",
		Resource: addonapiv1.AddonPlural,
	}
	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return dynCli.Resource(resource).Namespace(namespace).List(ctx, options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return dynCli.Resource(resource).Namespace(namespace).Watch(ctx, options)
			},
		},
		&unstructured.Unstructured{},
		0, //Skip resync
		cache.Indexers{},
	)
	return informer
}

// addon dependent resources informers
func NewResourceInformers(ctx context.Context, kubeClient kubernetes.Interface, namespace string) map[string]cache.SharedIndexInformer {
	resourceInformers := make(map[string]cache.SharedIndexInformer)
	// addon resources namespace informers
	nsinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.CoreV1().Namespaces().List(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.CoreV1().Namespaces().Watch(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&v1.Namespace{},
		0, //Skip resync
		cache.Indexers{},
	)

	deploymentinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.AppsV1().Deployments("").List(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.AppsV1().Deployments("").Watch(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&appsv1.Deployment{},
		0, //Skip resync
		cache.Indexers{},
	)

	srvinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.CoreV1().Services("").List(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.CoreV1().Services("").Watch(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&v1.Service{},
		0, //Skip resync
		cache.Indexers{},
	)

	configMapinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.CoreV1().ConfigMaps("").List(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.CoreV1().ConfigMaps("").Watch(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&v1.ConfigMap{},
		0, //Skip resync
		cache.Indexers{},
	)

	clusterRoleinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.RbacV1().ClusterRoles().List(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.RbacV1().ClusterRoles().Watch(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&rbac_v1.ClusterRole{},
		0, //Skip resync
		cache.Indexers{},
	)

	clusterRoleBindingInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.RbacV1().ClusterRoleBindings().List(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.RbacV1().ClusterRoleBindings().Watch(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&rbac_v1.ClusterRoleBinding{},
		0, //Skip resync
		cache.Indexers{},
	)

	srvAcntinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.CoreV1().ServiceAccounts(namespace).List(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.CoreV1().ServiceAccounts(namespace).Watch(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&v1.ServiceAccount{},
		0, //Skip resync
		cache.Indexers{},
	)

	jobinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.BatchV1().Jobs(namespace).List(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.BatchV1().Jobs(namespace).Watch(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&batch_v1.Job{},
		0, //Skip resync
		cache.Indexers{},
	)

	cronjobinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.BatchV1beta1().CronJobs(namespace).List(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.BatchV1beta1().CronJobs(namespace).Watch(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&batch_v1beta1.CronJob{},
		0, //Skip resync
		cache.Indexers{},
	)

	daemonSetinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.AppsV1().DaemonSets(namespace).List(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.AppsV1().DaemonSets(namespace).Watch(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&appsv1.DaemonSet{},
		0, //Skip resync
		cache.Indexers{},
	)

	replicaSetinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.AppsV1().ReplicaSets(namespace).List(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.AppsV1().ReplicaSets(namespace).Watch(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
		},
		&appsv1.ReplicaSet{},
		0, //Skip resync
		cache.Indexers{},
	)

	statefulSetinformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.AppsV1().StatefulSets(namespace).List(ctx, meta_v1.ListOptions{
					LabelSelector: ResourcetweakListOptions()})
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.AppsV1().StatefulSets(namespace).Watch(ctx, meta_v1.ListOptions{
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

// Start prepares watchers and run their controllers, then waits for process termination signals
func Start(ctx context.Context, namespace string, kubeClient kubernetes.Interface, dynCli dynamic.Interface, addoncli addonv1versioned.Interface, wfcli wfclientset.Interface) {

	c := newResourceController(kubeClient, dynCli, addoncli, wfcli, "addon", namespace)

	stopCh := make(chan struct{})
	defer close(stopCh)

	c.Run(ctx, stopCh)

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)
	signal.Notify(sigterm, syscall.SIGINT)
	<-sigterm
}

func newResourceController(kubeClient kubernetes.Interface, dynCli dynamic.Interface, addoncli addonv1versioned.Interface, wfcli wfclientset.Interface, resourceType, namespace string) *Controller {
	c := &Controller{
		logger:    logrus.WithField("controllers", resourceType),
		clientset: kubeClient,
		dynCli:    dynCli,
		addoncli:  addoncli,
		wfcli:     wfcli,
		namespace: namespace,
	}
	c.queue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	return c
}

func (c *Controller) setupaddonhandlers() {
	var newEvent Event
	var err error
	resourceType := "addon"

	c.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			newEvent.key, err = cache.MetaNamespaceKeyFunc(obj)
			newEvent.eventType = "create"

			if err == nil {
				logrus.WithField("controllers", "addon").Infof("Processing add to %v: %s", resourceType, newEvent.key)
				c.queue.Add(newEvent)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			newEvent.key, err = cache.MetaNamespaceKeyFunc(old)
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
			key, err := cache.MetaNamespaceKeyFunc(old)
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
			key, err := cache.MetaNamespaceKeyFunc(old)
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
			key, err := cache.MetaNamespaceKeyFunc(old)
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
			key, err := cache.MetaNamespaceKeyFunc(old)
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
			key, err := cache.MetaNamespaceKeyFunc(old)
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
			key, err := cache.MetaNamespaceKeyFunc(old)
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
			key, err := cache.MetaNamespaceKeyFunc(old)
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
			key, err := cache.MetaNamespaceKeyFunc(old)
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
			key, err := cache.MetaNamespaceKeyFunc(old)
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
			key, err := cache.MetaNamespaceKeyFunc(old)
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
			key, err := cache.MetaNamespaceKeyFunc(old)
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
			key, err := cache.MetaNamespaceKeyFunc(old)
			if err == nil {
				newEvent.key = key
				newEvent.eventType = "update"

				logrus.WithField("controllers", "StatefulSet").Infof("Processing update to %v: %s", resourceType, newEvent.key)
				c.handleStatefulSet(ctx, new)
			}
		},
	})
}

// Run starts the addon-controllers controller
func (c *Controller) Run(ctx context.Context, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.logger.Info("Starting keiko addon-manager controller")
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	serverStartTime = time.Now().Local()

	c.recorder = createEventRecorder(c.namespace, c.clientset, c.logger)
	c.scheme = common.GetAddonMgrScheme()
	c.versionCache = addoninternal.NewAddonVersionCacheClient()

	c.informer = newAddonInformer(ctx, c.dynCli, c.namespace)
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
		c.logger.Errorf("[Run] pre-process addon(s) err %#v", err)
	}

	c.setupaddonhandlers()
	c.setupwfhandlers(ctx)
	c.setupresourcehandlers(ctx)

	go c.informer.Run(stopCh)
	go c.wfinformer.Run(stopCh)
	go c.nsinformer.Run(stopCh)
	go c.deploymentinformer.Run(stopCh)
	go c.srvAcntinformer.Run(stopCh)
	go c.configMapinformer.Run(stopCh)
	go c.clusterRoleinformer.Run(stopCh)
	go c.clusterRoleBindingInformer.Run(stopCh)
	go c.jobinformer.Run(stopCh)
	go c.cronjobinformer.Run(stopCh)
	go c.replicaSetinformer.Run(stopCh)
	go c.daemonSetinformer.Run(stopCh)
	go c.srvinformer.Run(ctx.Done())
	go c.replicaSetinformer.Run(stopCh)
	go c.srvinformer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.HasSynced, c.wfinformer.HasSynced, c.nsinformer.HasSynced, c.deploymentinformer.HasSynced,
		c.srvAcntinformer.HasSynced, c.configMapinformer.HasSynced, c.clusterRoleinformer.HasSynced, c.clusterRoleBindingInformer.HasSynced,
		c.replicaSetinformer.HasSynced, c.daemonSetinformer.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	c.logger.Info("Keiko addon-manager controller synced and ready")
	for i := 0; i < 5; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}
	<-stopCh
}

// HasSynced is required for the cache.Controller interface.
func (c *Controller) HasSynced() bool {
	return c.informer.HasSynced()
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
		c.logger.Debugf("received shutdown message.")
		return false
	}
	defer c.queue.Done(newEvent)

	err := c.processItem(ctx, newEvent.(Event))
	if err == nil {
		c.queue.Forget(newEvent)
	} else if c.queue.NumRequeues(newEvent) < maxRetries {
		c.logger.Errorf("Error processing %s (will retry): %v", newEvent.(Event).key, err)
		c.queue.AddRateLimited(newEvent)
	} else {
		c.logger.Errorf("Error processing %s (giving up): %v", newEvent.(Event).key, err)
		c.queue.Forget(newEvent)
		utilruntime.HandleError(err)
	}

	return true
}

func (c *Controller) processItem(ctx context.Context, newEvent Event) error {
	obj, exists, err := c.informer.GetIndexer().GetByKey(newEvent.key)
	if err != nil {
		msg := fmt.Sprintf("failed fetching key %s from cache, err %v ", newEvent.key, err)
		c.logger.Error(msg)
		return fmt.Errorf(msg)
	} else if !exists {
		if newEvent.eventType == "delete" {
			return nil
		}
		msg := fmt.Sprintf("event %s obj %s does not exist", newEvent.eventType, newEvent.key)
		_, addonName := c.namespacenameFromKey(newEvent.key)
		c.removeFromCache(addonName)
		c.logger.Error(msg)
		return fmt.Errorf(msg)
	}

	addon, err := common.FromUnstructured(obj.(*unstructured.Unstructured))
	if err != nil {
		msg := fmt.Sprintf("obj %s is not addon, err %v", newEvent.key, err)
		c.logger.Error(msg)
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
	c.logger.Infof("initController pre-process addon every restart.")
	addonList, err := c.addoncli.AddonmgrV1alpha1().Addons(c.namespace).List(ctx, meta_v1.ListOptions{})
	if err != nil {
		c.logger.Fatalf("failed list %s addons ", c.namespace)
	}
	for _, item := range addonList.Items {

		// retrieve status from install wf
		if item.Spec.Lifecycle.Install.Template != "" {
			wfIdentifierName := fmt.Sprintf("%s-%s-%s-wf", item.Name, "install", item.CalculateChecksum())
			wf, err := c.wfcli.ArgoprojV1alpha1().Workflows(item.Namespace).Get(ctx, wfIdentifierName, meta_v1.GetOptions{})
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
				c.logger.Infof("[initController] addon %s/%s install status %s", item.Namespace, item.Name, item.Status.Lifecycle.Installed)
			} else {
				// error case the install wf does not exist
				c.logger.Warnf("[initController] failed get addon %s/%s install wf", item.Namespace, item.Name)
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
			wf, err := c.wfcli.ArgoprojV1alpha1().Workflows(item.Namespace).Get(ctx, wfIdentifierName, meta_v1.GetOptions{})
			if err == nil && wf != nil {
				// reset lifecycle status
				item.Status.Lifecycle.Prereqs = addonv1.ApplicationAssemblyPhase(wf.Status.Phase)
				c.logger.Infof("[initController] addon %s/%s prereqs status %s", item.Namespace, item.Name, item.Status.Lifecycle.Prereqs)
			} else {
				c.logger.Warnf("[initController] failed get addon %s/%s prereqs wf", item.Namespace, item.Name)
			}
		}

		if item.Spec.Lifecycle.Prereqs.Template == "" && item.Spec.Lifecycle.Install.Template == "" {
			c.logger.Infof("[initController] addon %s/%s does not lifecycle.", item.Namespace, item.Name)
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
			c.logger.Warnf("[initController] addon %s/%s stuck on dependency.", item.Namespace, item.Name)
		}

		item.Finalizers = append(item.Finalizers, addonapiv1.FinalizerName)
		_, err := c.updateAddon(ctx, &item)
		if err != nil {
			c.logger.Errorf("[initController] failed patch addon %s/%s labels.", item.Namespace, item.Name)
		}
		c.logger.Infof("[initController] pre-press addon %s/%s successfully. adding into cache.", item.Namespace, item.Name)
		c.addAddonToCache(&item)
	}
	c.logger.Infof("initController pre-process end.")
	return nil
}
