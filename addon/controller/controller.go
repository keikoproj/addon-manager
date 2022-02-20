package controller

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	"k8s.io/client-go/dynamic"

	kubeinformers "k8s.io/client-go/informers"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	batchinformers "k8s.io/client-go/informers/batch/v1"
	srvinformers "k8s.io/client-go/informers/core/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/util/workqueue"

	wfclientset "github.com/argoproj/argo-workflows/v3/pkg/client/clientset/versioned"
	wflister "github.com/argoproj/argo-workflows/v3/pkg/client/listers/workflow/v1alpha1"
	"github.com/keikoproj/addon-manager/addon/events"
	"github.com/keikoproj/addon-manager/addon/indexes"
	"github.com/keikoproj/addon-manager/addon/metrics"
	"github.com/keikoproj/addon-manager/addon/sync"
	addonv1listers "github.com/keikoproj/addon-manager/pkg/client/listers/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"

	addonapiv1 "github.com/keikoproj/addon-manager/pkg/apis/addon"
	addonv1 "github.com/keikoproj/addon-manager/pkg/apis/addon/v1alpha1"
	addonv1versioned "github.com/keikoproj/addon-manager/pkg/client/clientset/versioned"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	addonwfutility "github.com/keikoproj/addon-manager/pkg/workflows"

	wfinformer "github.com/argoproj/argo-workflows/v3/pkg/client/informers/externalversions"
	addonv1informers "github.com/keikoproj/addon-manager/pkg/client/informers/externalversions"
)

const (
	managedNameSpace = "addon-manager-system"
)

// AddonController is the controller for addon resources
type AddonController struct {

	// namespace of the addon controller
	managedNamespace string

	restConfig *rest.Config

	throttler sync.Throttler

	kubeclientset    kubernetes.Interface
	rateLimiter      *rate.Limiter
	dynamicInterface dynamic.Interface

	wfclientset    wfclientset.Interface
	addonclientset addonv1versioned.Interface

	wflister    wflister.WorkflowLister
	addonlister addonv1listers.AddonLister //addon lister

	addonInformer       cache.SharedIndexInformer
	wfInformer          cache.SharedIndexInformer
	kubeInformerFactory kubeinformers.SharedInformerFactory

	// informers for workflow deployed k8s resources
	serviceInformer    srvinformers.ServiceInformer
	srvacntInformer    srvinformers.ServiceAccountInformer
	deploymentInformer appsinformers.DeploymentInformer
	daemonSetInformer  appsinformers.DaemonSetInformer
	replicaSet         appsinformers.ReplicaSetInformer
	statefulSet        appsinformers.StatefulSetInformer

	jobInformer batchinformers.JobInformer

	addonQueue workqueue.RateLimitingInterface
	wfQueue    workqueue.RateLimitingInterface
	k8sQueue   workqueue.RateLimitingInterface

	eventRecorderManager events.EventRecorderManager

	opConfig *OperatorConfig

	metrics *metrics.Metrics

	// optimize me
	scheme *runtime.Scheme
	client.Client
}

// NewAddonController instantiates a new WorkflowController
func NewAddonController(ctx context.Context, restConfig *rest.Config, kubeclientset kubernetes.Interface, wfclientset wfclientset.Interface, addonclientset addonv1versioned.Interface, managedNamespace string) (*AddonController, error) {
	dynamicInterface, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{})
	if err != nil {
		panic("could not create manager")
	}

	wfc := AddonController{
		restConfig:           restConfig,
		kubeclientset:        kubeclientset,
		dynamicInterface:     dynamicInterface,
		wfclientset:          wfclientset,
		addonclientset:       addonclientset,
		managedNamespace:     managedNamespace,
		eventRecorderManager: events.NewEventRecorderManager(kubeclientset),

		scheme: common.GetAddonMgrScheme(),
		Client: mgr.GetClient(),
	}

	wfc.metrics = metrics.New(wfc.getMetricsServerConfig())
	workqueue.SetProvider(wfc.metrics)
	wfc.addonQueue = wfc.metrics.RateLimiterWithBusyWorkers(&fixedItemIntervalRateLimiter{}, "addon_queue")
	wfc.throttler = wfc.newThrottler()
	wfc.wfQueue = wfc.metrics.RateLimiterWithBusyWorkers(&fixedItemIntervalRateLimiter{}, "workflow_queue")
	wfc.k8sQueue = wfc.metrics.RateLimiterWithBusyWorkers(&fixedItemIntervalRateLimiter{}, "addon_resource_queue")

	return &wfc, nil
}

func (wfc *AddonController) newThrottler() sync.Throttler {
	f := func(key string) { wfc.wfQueue.AddRateLimited(key) }
	return sync.ChainThrottler{
		sync.NewThrottler(1, sync.SingleBucket, f),
		sync.NewThrottler(1, sync.NamespaceBucket, f),
	}
}

const (
	workflowResyncPeriod = 20 * time.Minute
)

var addonindexers = cache.Indexers{
	indexes.AddonIndex: indexes.MetaNamespaceLabelIndexFunc(LabelKeyAppName),
}

var wfindexers = cache.Indexers{}

// Run starts an addon resource controller
func (wfc *AddonController) Run(ctx context.Context, addonWorkers, wfWorkers, addonResourceWorkers int) {
	defer runtimeutil.HandleCrash(runtimeutil.PanicHandlers...)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	defer wfc.addonQueue.ShutDown()
	defer wfc.wfQueue.ShutDown()
	defer wfc.k8sQueue.ShutDown()

	log.WithField("version", "addon-mgr latest version").
		WithField("defaultRequeueTime", GetRequeueTime()).
		Info("Starting addon Controller")

	log.WithField("addon", addonWorkers).
		WithField("workflow", wfWorkers).
		WithField("addonresourcesoworkers", addonResourceWorkers).
		Info("Current Worker Numbers ")

	wfc.addonInformer = NewAddonInformer(wfc.dynamicInterface, addonapiv1.ManagedNameSpace, workflowResyncPeriod, wfc.tweakListOptions, addonindexers)
	wfc.wfInformer = NewWorkflowInformer(wfc.dynamicInterface, addonapiv1.ManagedNameSpace, workflowResyncPeriod, wfc.tweakListOptions, wfindexers)

	wfc.addAddonInformerHandlers(ctx)
	wfc.addWorkflowInformerHandlers(ctx)

	go wfc.wfInformer.Run(ctx.Done())
	go wfc.addonInformer.Run(ctx.Done())

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(make(<-chan struct{}), wfc.wfInformer.HasSynced, wfc.addonInformer.HasSynced) {
		log.Fatal("Timed out waiting for caches to sync")
	}

	stopChan := make(chan struct{})
	wfInformFactory := wfinformer.NewSharedInformerFactory(wfc.wfclientset, time.Second*30)
	wfc.wflister = wfInformFactory.Argoproj().V1alpha1().Workflows().Lister()
	go wfInformFactory.Start(stopChan)

	addonInformFactory := addonv1informers.NewSharedInformerFactory(wfc.addonclientset, time.Second*30)
	wfc.addonlister = addonInformFactory.Addonmgr().V1alpha1().Addons().Lister()
	wfc.opConfig = NewOperatorConfig(wfc.dynamicInterface, wfc.addonclientset, wfc.eventRecorderManager.Get(managedNamespace), wfc.wflister)
	go addonInformFactory.Start(stopChan)

	// handle addon resources for addon status
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(wfc.kubeclientset, time.Second*30)
	wfc.kubeInformerFactory = kubeInformerFactory
	wfc.serviceInformer = kubeInformerFactory.Core().V1().Services()
	wfc.srvacntInformer = kubeInformerFactory.Core().V1().ServiceAccounts()
	wfc.deploymentInformer = kubeInformerFactory.Apps().V1().Deployments()
	wfc.jobInformer = kubeInformerFactory.Batch().V1().Jobs()
	wfc.replicaSet = kubeInformerFactory.Apps().V1().ReplicaSets()
	wfc.statefulSet = kubeInformerFactory.Apps().V1().StatefulSets()
	wfc.daemonSetInformer = kubeInformerFactory.Apps().V1().DaemonSets()
	wfc.addObservedHandler(ctx, stopChan)

	// Start the metrics server
	go wfc.metrics.RunServer(ctx)

	leaderElectionOff := os.Getenv("LEADER_ELECTION_DISABLE")
	if leaderElectionOff == "true" {
		log.Info("Leader election is turned off. Running in single-instance mode")
		logCtx := log.WithField("id", "single-instance")
		go wfc.startLeading(ctx, logCtx, addonWorkers, wfWorkers, addonResourceWorkers)
	} else {
		nodeID, ok := os.LookupEnv("LEADER_ELECTION_IDENTITY")
		if !ok {
			log.Fatal("LEADER_ELECTION_IDENTITY must be set so that the workflow controllers can elect a leader")
		}
		logCtx := log.WithField("id", nodeID)

		leaderName := "addon-manager-controller"

		go leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
			Lock: &resourcelock.LeaseLock{
				LeaseMeta: metav1.ObjectMeta{Name: leaderName, Namespace: wfc.managedNamespace}, Client: wfc.kubeclientset.CoordinationV1(),
				LockConfig: resourcelock.ResourceLockConfig{Identity: nodeID, EventRecorder: wfc.eventRecorderManager.Get(wfc.managedNamespace)},
			},
			ReleaseOnCancel: true,
			LeaseDuration:   LookupEnvDurationOr("LEADER_ELECTION_LEASE_DURATION", 15*time.Second),
			RenewDeadline:   LookupEnvDurationOr("LEADER_ELECTION_RENEW_DEADLINE", 10*time.Second),
			RetryPeriod:     LookupEnvDurationOr("LEADER_ELECTION_RETRY_PERIOD", 5*time.Second),
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(ctx context.Context) {
					wfc.startLeading(ctx, logCtx, addonWorkers, wfWorkers, addonResourceWorkers)
				},
				OnStoppedLeading: func() {
					logCtx.Info("stopped leading")
					cancel()
				},
				OnNewLeader: func(identity string) {
					logCtx.WithField("leader", identity).Info("new leader")
				},
			},
		})
	}
	<-ctx.Done()
}

func (wfc *AddonController) addWorkflowInformerHandlers(ctx context.Context) {
	wfc.wfInformer.AddEventHandler(
		cache.FilteringResourceEventHandler{
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					fmt.Printf("\n cache is empty 111111 ???\n")
					key, err := cache.MetaNamespaceKeyFunc(obj)
					if err == nil {
						// for a new workflow, we do not want to rate limit its execution using AddRateLimited
						log.Info("completed workflow ", key, " Add")
						//wfc.wfQueue.Add(key)
						//wfc.throttler.Add(key, 0, time.Now())
					}
				},
				UpdateFunc: func(old, new interface{}) {
					oldWf, newWf := old.(*unstructured.Unstructured), new.(*unstructured.Unstructured)
					// this check is very important to prevent doing many reconciliations we do not need to do
					if oldWf.GetResourceVersion() == newWf.GetResourceVersion() {
						return
					}
					key, err := cache.MetaNamespaceKeyFunc(new)
					if err == nil {
						log.Info("completed workflow ", key, " Update")
						wfc.wfQueue.Add(key)
					}
				},
				DeleteFunc: func(obj interface{}) {
					// IndexerInformer uses a delta queue, therefore for deletes we have to use this
					// key function.
					key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
					if err == nil {
						log.Info("completed workflow ", key, " Deleted")

					}
				},
			},
		},
	)
	wfc.wfInformer.AddEventHandler(cache.FilteringResourceEventHandler{
		// FilterFunc: func(obj interface{}) bool {
		// 	return !UnstructuredHasCompletedLabel(obj)
		// },
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				key, err := cache.MetaNamespaceKeyFunc(obj)
				if err == nil {
					// for a new workflow, we do not want to rate limit its execution using AddRateLimited
					log.Info("uncompleted wf add ", key)
					wfc.wfQueue.Add(key)
				}

			},
			UpdateFunc: func(_, obj interface{}) {
				key, err := cache.MetaNamespaceKeyFunc(obj)
				if err == nil {
					// for a new workflow, we do not want to rate limit its execution using AddRateLimited
					log.Info("uncompleted wf update ", key)
					wfc.wfQueue.Add(key)
				}
			},
		},
	},
	)
	wfc.wfInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			wf, ok := obj.(*unstructured.Unstructured)
			if ok { // maybe cache.DeletedFinalStateUnknown
				fmt.Printf(" workflow %s/%s deletion", wf.GetNamespace(), wf.GetName())
			}
		},
	})
}

func (wfc *AddonController) addAddonInformerHandlers(ctx context.Context) {
	wfc.addonInformer.AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				yes := !UnstructuredHasCompletedLabel(obj)
				fmt.Printf("\n not complete ? %v \n", yes)
				return yes
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					fmt.Printf("\n cache is empty 3333333 ???\n")
					key, err := cache.MetaNamespaceKeyFunc(obj)
					if err == nil {
						fmt.Printf("\n enque  addon %s add \n", key)
						if wfc.addonQueue == nil {
							fmt.Printf("\n skip addonqueue is not ready net\n")
							return
						}
						wfc.addonQueue.Add(key)
						fmt.Printf("\n after enqueue crash\n")
						//wfc.throttler.Add(key, 0, time.Now())
					}
				},
				UpdateFunc: func(old, new interface{}) {
					oldAddon, newAddon := old.(*unstructured.Unstructured), new.(*unstructured.Unstructured)
					// this check is very important to prevent doing many reconciliations we do not need to do
					if oldAddon.GetResourceVersion() == newAddon.GetResourceVersion() {
						return
					}
					key, err := cache.MetaNamespaceKeyFunc(new)
					if err == nil {
						fmt.Printf("enque  Addon  %s Update", key)
						wfc.addonQueue.Add(key)
					}
				},
				DeleteFunc: func(obj interface{}) {
					// IndexerInformer uses a delta queue, therefore for deletes we have to use this
					// key function.
					key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
					if err == nil {
						log.Info("[Not Completed] Addon Deleted ", key)
					}
				},
			},
		},
	)
	wfc.addonInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			addon, ok := obj.(*unstructured.Unstructured)
			log.Info("completed addon get deleted ", addon.GetNamespace(), "/", addon.GetName())
			if ok {
				wfc.metrics.StopRealtimeMetricsForKey(string(addon.GetUID()))
			}
		},
	})
}

func (wfc *AddonController) addObservedHandler(ctx context.Context, stopCh <-chan struct{}) {
	wfc.srvacntInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			un, ok := obj.(*unstructured.Unstructured)
			// check default labels set during artifact creation
			return ok && un.GetNamespace() == wfc.managedNamespace && un.GetLabels()[LabelKeyManagedBy] == common.AddonGVR().Group && len(un.GetLabels()[LabelKeyAddonName]) > 0
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				fmt.Printf("\n cache is empty 22222 ???\n")
				fmt.Printf("Addon informer 111111111")
			},
			UpdateFunc: func(_, obj interface{}) {
				fmt.Printf("Addon informer DeleteFunc 2222222 ")
			},
		},
	})
	wfc.kubeInformerFactory.Start(stopCh)
}
func (wfc *AddonController) GetManagedNamespace() string {
	if wfc.managedNamespace != "" {
		return wfc.managedNamespace
	}
	return managedNameSpace
}

func (wfc *AddonController) tweakListOptions(options *metav1.ListOptions) {
	// currently, I do not have any label
}

func (wfc *AddonController) startLeading(ctx context.Context, logCtx *log.Entry, addonWorkers, wfWorkers, addonresourcesWorkers int) {
	defer runtimeutil.HandleCrash(runtimeutil.PanicHandlers...)

	logCtx.Info("started leading")
	for i := 0; i < wfWorkers; i++ {
		go wait.Until(wfc.wfWorker, time.Second, ctx.Done())
	}

	for i := 0; i < addonWorkers; i++ {
		fmt.Printf("\n bumped up queue...\n")
		go wait.Until(wfc.addonWorker, time.Second, ctx.Done())
	}

	for i := 0; i < addonresourcesWorkers; i++ {
		go wait.Until(wfc.addonResourcesWorker, time.Second, ctx.Done())
	}
}

func (wfc *AddonController) addonWorker() {
	defer runtimeutil.HandleCrash(runtimeutil.PanicHandlers...)

	ctx := context.Background()
	for wfc.processNextAddonItem(ctx) {
	}
}

func (wfc *AddonController) wfWorker() {
	defer runtimeutil.HandleCrash(runtimeutil.PanicHandlers...)

	ctx := context.Background()
	for wfc.processNextWorkFlowItem(ctx) {
	}
}

func (wfc *AddonController) addonResourcesWorker() {
	defer runtimeutil.HandleCrash(runtimeutil.PanicHandlers...)

	ctx := context.Background()
	for wfc.processAddonResourcesItem(ctx) {
	}
}

// processNextItem is the worker logic for handling workflow updates
func (wfc *AddonController) processNextWorkFlowItem(ctx context.Context) bool {
	fmt.Printf("\n\n processNextWorkFlowItem \n\n")
	key, quit := wfc.addonQueue.Get()
	if quit {
		panic("have to quit")
	}
	defer wfc.wfQueue.Done(key)

	obj, exists, err := wfc.wfInformer.GetIndexer().GetByKey(key.(string))
	if err != nil {
		log.WithFields(log.Fields{"key": key, "error": err}).Error("Failed to get workflow from informer")
		return true
	}
	if !exists {
		return true
	}

	un, ok := obj.(*unstructured.Unstructured)
	if !ok {
		log.WithFields(log.Fields{"key": key}).Warn("index not an unstructured")
		return true
	}
	wfobj := &wfv1.Workflow{}
	err = common.FromUnstructuredObj(un, wfobj)
	if err != nil {
		panic(err)
	}
	fmt.Printf("\n\n start processing wf %s", wfobj.GetName())

	// find the Addon from the namespace and update its status accordingly
	addonName, lifecycle, err := addonwfutility.ExtractAddOnNameAndLifecycleStep(wfobj.GetName())
	if err != nil {
		panic(err)
	}

	wfc.updateAddonStatus(wfobj.GetNamespace(), addonName, lifecycle, wfobj.Status.Phase)
	return true
}

func (wfc *AddonController) updateAddonStatus(namespace, name, lifecycle string, lifecyclestatus wfv1.WorkflowPhase) error {

	msg := fmt.Sprintf("updating addon %s/%s step %s status to %s\n", namespace, name, lifecycle, lifecyclestatus)
	log.Info(msg)

	addonobj, err := wfc.addonlister.Addons(namespace).Get(name)
	if err != nil || addonobj == nil {
		msg := fmt.Sprintf("failed getting addon %s/%s, err %v", namespace, name, err)
		fmt.Print(msg)
		return fmt.Errorf(msg)

	}
	updating := addonobj.DeepCopy()
	cycle := updating.Status.Lifecycle
	msg = fmt.Sprintf("addon %s/%s cycle %v", updating.Namespace, updating.Name, cycle)
	log.Info(msg)
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
		log.Info(msg)
		return nil
	}

	if updating.Status.Lifecycle.Installed.Completed() {
		// add completed label
		labels := updating.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[addonCompleteLabel] = "true"
		un, err := common.ToUnstructured(updating)
		if un == nil || err != nil {
			panic("failed to ToUnstructured.")
		}
		un.SetLabels(labels)
	}

	updated, err := wfc.addonclientset.AddonmgrV1alpha1().Addons(updating.Namespace).UpdateStatus(context.Background(), updating,
		metav1.UpdateOptions{})
	if err != nil {
		msg := fmt.Sprintf("Addon %s/%s status could not be updated. %v", updating.Namespace, updating.Name, err)
		fmt.Print(msg)
	}
	cycle = updated.Status.Lifecycle
	msg = fmt.Sprintf("addon %s/%s updated cycle %v", updating.Namespace, updating.Name, cycle)
	log.Info(msg)
	msg = fmt.Sprintf("successfully update addon %s/%s step %s status to %s", namespace, name, lifecycle, lifecyclestatus)
	log.Info(msg)
	return nil
}

func (wfc *AddonController) processNextAddonItem(ctx context.Context) bool {
	// fix me, tune the sync time
	fmt.Printf("\n\n 111111 processNextAddonItem \n\n")
	key, quit := wfc.addonQueue.Get()
	if quit {
		panic("have to quit")
	}
	defer wfc.addonQueue.Done(key)

	fmt.Printf("\n\n 222222 processNextAddonItem key %s\n\n", key.(string))
	obj, exists, err := wfc.addonInformer.GetIndexer().GetByKey(key.(string))
	if err != nil {
		log.WithFields(log.Fields{"key": key, "error": err}).Error("Failed to get workflow from informer")
		return true
	}
	if !exists {
		return true
	}

	// we need a lock here, lock the key from the queue

	un, ok := obj.(*unstructured.Unstructured)
	if !ok {
		log.WithFields(log.Fields{"key": key}).Warn("not an unstructured")
		return true
	}
	addon, err := common.FromUnstructured(un)
	if err != nil {
		panic(err)
	}
	if addon.Labels[addonCompleteLabel] == "true" {
		log.Info("skip completed addons.")
		return true
	}
	fmt.Printf("\n\n 33333 install addon key %s\n\n", key.(string))
	log.Info("execAddon ", addon.Namespace, "/", addon.Name)
	err = wfc.opConfig.execAddon(ctx, addon, wfc.Client, wfc.scheme)
	if err != nil {
		panic(err)
	}

	return true
}

func (wfc *AddonController) processAddonResourcesItem(ctx context.Context) bool {
	return true
}

func (wfc *AddonController) getMetricsServerConfig() metrics.ServerConfig {
	// Metrics config
	path := metrics.DefaultMetricsServerPath
	port := metrics.DefaultMetricsServerPort

	metricsConfig := metrics.ServerConfig{
		Enabled:      true,
		Path:         path,
		Port:         port,
		TTL:          time.Duration(24 * time.Hour),
		IgnoreErrors: false,

		Secure: false,
	}

	return metricsConfig
}
