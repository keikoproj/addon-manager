/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/jinzhu/inflection"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/addon"
	"github.com/keikoproj/addon-manager/pkg/common"
	"github.com/keikoproj/addon-manager/pkg/workflows"
)

// addon ttl time
const TTL int64 = 180000

// Watched resources
var (
	resources = [...]runtime.Object{
		&v1.Service{TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "v1"}},
		&batchv1.Job{TypeMeta: metav1.TypeMeta{Kind: "Job", APIVersion: "batch/v1"}}, &batchv1beta1.CronJob{TypeMeta: metav1.TypeMeta{Kind: "CronJob", APIVersion: "batch/v1beta1"}},
		&appsv1.Deployment{TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"}},
		&appsv1.DaemonSet{TypeMeta: metav1.TypeMeta{Kind: "DaemonSet", APIVersion: "apps/v1"}},
		&appsv1.ReplicaSet{TypeMeta: metav1.TypeMeta{Kind: "ReplicaSet", APIVersion: "apps/v1"}},
		&appsv1.StatefulSet{TypeMeta: metav1.TypeMeta{Kind: "StatefulSet", APIVersion: "apps/v1"}},
	}
	finalizerName = "delete.addonmgr.keikoproj.io"
)

// AddonReconciler reconciles a Addon object
type AddonReconciler struct {
	client.Client
	Log             logr.Logger
	Scheme          *runtime.Scheme
	versionCache    addon.VersionCacheClient
	dynClient       dynamic.Interface
	generatedClient *kubernetes.Clientset
	recorder        record.EventRecorder
}

// NewAddonReconciler returns an instance of AddonReconciler
func NewAddonReconciler(mgr manager.Manager, log logr.Logger) *AddonReconciler {
	return &AddonReconciler{
		Client:          mgr.GetClient(),
		Log:             log,
		Scheme:          mgr.GetScheme(),
		versionCache:    addon.NewAddonVersionCacheClient(),
		dynClient:       dynamic.NewForConfigOrDie(mgr.GetConfig()),
		generatedClient: kubernetes.NewForConfigOrDie(mgr.GetConfig()),
		recorder:        mgr.GetEventRecorderFor("addons"),
	}
}


// +kubebuilder:rbac:groups=addonmgr.keikoproj.io,resources=addons,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=addonmgr.keikoproj.io,resources=addons/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=argoproj.io,resources=workflows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=list
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=get;list;patch;create
// +kubebuilder:rbac:groups="",resources=namespaces;clusterroles;configmaps;events;pods;serviceaccounts;services,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments;daemonsets;replicasets;statefulsets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=extensions,resources=deployments;daemonsets;replicasets;ingresses,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=batch,resources=jobs;cronjobs,verbs=get;list;watch;create;update;patch

// Reconcile method for all addon requests
func (r *AddonReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("addon", req.NamespacedName)

	log.Info("Starting addon-manager reconcile ...")

	var instance = &addonmgrv1alpha1.Addon{}
	if err := r.Get(context.TODO(), req.NamespacedName, instance); err != nil {
		log.Info("Addon not found.")

		// Remove version from cache
		if ok, v := r.versionCache.HasVersionName(req.Name); ok {
			r.versionCache.RemoveVersion(v.PkgName, v.PkgVersion)
		}

		return reconcile.Result{}, ignoreNotFound(err)
	}

	// Process addon instance
	ret, err := r.processAddon(ctx, req, log, instance)

	// Always update cache, status
	r.updateAddonStatus(ctx, instance)
	r.addAddonToCache(instance)

	return ret, err
}

// SetupWithManager is called to setup manager and watchers
func (r *AddonReconciler) SetupWithManager(mgr ctrl.Manager) error {
	log := r.Log
	//var addongrp = common.AddonGVR().Group
	bldr := ctrl.NewControllerManagedBy(mgr).
		For(&addonmgrv1alpha1.Addon{}).
		// Watch workflows created by addon
		Watches(&source.Kind{Type: common.WorkflowType()}, &handler.EnqueueRequestForOwner{
			IsController: false,
			OwnerType:    &addonmgrv1alpha1.Addon{},
		})

	generatedInformers := informers.NewSharedInformerFactory(r.generatedClient, time.Minute*30)

	err := mgr.Add(manager.RunnableFunc(func(s <-chan struct{}) error {
		generatedInformers.Start(s)
		generatedInformers.WaitForCacheSync(s)
		<-s
		return nil
	}))
	if err != nil {
		log.Error(err, "Error adding InformerFactory to the Manager")
		return err
	}

	// Watch for changes to kubernetes Resources matching addon labels.
	for _, resc := range resources {
		gvk := resc.GetObjectKind().GroupVersionKind()
		_, kind := gvk.ToAPIVersionAndKind()

		gvr := schema.GroupVersionResource{
			Group:    gvk.Group,
			Version:  gvk.Version,
			Resource: inflection.Plural(strings.ToLower(kind)),
		}

		inf, err := generatedInformers.ForResource(gvr)
		if err != nil {
			return err
		}

		bldr = bldr.Watches(&source.Informer{Informer: inf.Informer()}, &handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
				var reqs = make([]reconcile.Request, 0)
				var labels = a.Meta.GetLabels()
				if name, ok := labels["app.kubernetes.io/name"]; ok && strings.TrimSpace(name) != "" {
					// Let's lookup addon related to this object.
					if ok, v := r.versionCache.HasVersionName(name); ok {
						reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
							Name:      v.Name,
							Namespace: v.Namespace,
						}})
					}
				}
				return reqs
			}),
		})
	}

	return bldr.Complete(r)
}

func (r *AddonReconciler) processAddon(ctx context.Context, req reconcile.Request, log logr.Logger, instance *addonmgrv1alpha1.Addon) (reconcile.Result, error) {

	// Calculate Checksum
	instance.Status.Checksum = instance.CalculateChecksum()

	// Resources list
	instance.Status.Resources = make([]addonmgrv1alpha1.ObjectStatus, 0)

	//set ttl starttime if it is 0
	if instance.Status.StartTime == 0 {
		instance.Status.StartTime = common.GetCurretTimestamp()
	}

	// Clear out the reason to start with
	instance.Status.Reason = ""

	// Update status that we have started reconciling this addon.
	if instance.Status.Lifecycle.Installed == "" {
		instance.Status.Lifecycle.Installed = addonmgrv1alpha1.Pending
		log.Info("Requeue to set pending status")
		return reconcile.Result{Requeue: true}, nil
	}

	//if addon installation expired
	if common.IsExpired(instance.Status.StartTime, TTL) && (instance.Status.Lifecycle.Installed == addonmgrv1alpha1.Pending ||
		instance.Status.Lifecycle.Installed == addonmgrv1alpha1.Deleting)  {
		reason := fmt.Sprintf("Addon %s/%s ttl expired", instance.Namespace, instance.Name)
		r.recorder.Event(instance, "Warning", "Failed", reason)
		err := fmt.Errorf(reason)
		log.Error(err, "Addon %s/%s expired.", instance.Namespace, instance.Name)
		if instance.Status.Lifecycle.Installed == addonmgrv1alpha1.Deleting {
			instance.Status.Lifecycle.Installed = addonmgrv1alpha1.DeleteFailed
		} else {
			instance.Status.Lifecycle.Installed = addonmgrv1alpha1.Failed
		}
		instance.Status.Reason = reason
		instance.Status.StartTime = 0

		return reconcile.Result{}, err
	}

	var wfl = workflows.NewWorkflowLifecycle(r.Client, r.dynClient, instance, r.recorder, r.Scheme)

	// Resource is being deleted, run finalizers and exit.
	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// For a better user experience we want to update the status and requeue
		if instance.Status.Lifecycle.Installed != addonmgrv1alpha1.Deleting {
			instance.Status.Lifecycle.Installed = addonmgrv1alpha1.Deleting
			log.Info("Requeue to set deleting status")
			return reconcile.Result{Requeue: true}, nil
		}

		err := r.Finalize(ctx, instance, wfl, finalizerName)
		if err != nil {
			reason := fmt.Sprintf("Addon %s/%s could not be finalized. %v", instance.Namespace, instance.Name, err)
			r.recorder.Event(instance, "Warning", "Failed", reason)
			instance.Status.Lifecycle.Installed = addonmgrv1alpha1.DeleteFailed
			instance.Status.StartTime = 0
			instance.Status.Reason = reason
			log.Error(err, "Failed to finalize addon.")
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	// Validate Addon
	if ok, err := addon.NewAddonValidator(instance, r.versionCache, r.dynClient).Validate(); !ok {
		reason := fmt.Sprintf("Addon %s/%s is not valid. %v", instance.Namespace, instance.Name, err)
		// Record an event if addon is not valid
		r.recorder.Event(instance, "Warning", "Failed", reason)
		instance.Status.Lifecycle.Installed = addonmgrv1alpha1.Failed
		instance.Status.StartTime = 0
		instance.Status.Reason = reason
		log.Error(err, "Failed to validate addon.")

		return reconcile.Result{}, err
	}

	// Record successful validation
	r.recorder.Event(instance, "Normal", "Completed", fmt.Sprintf("Addon %s/%s is valid.", instance.Namespace, instance.Name))

	// Set finalizer only after addon is valid
	if err := r.SetFinalizer(ctx, instance, finalizerName); err != nil {
		reason := fmt.Sprintf("Addon %s/%s could not add finalizer. %v", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance, "Warning", "Failed", reason)
		log.Error(err, "Failed to add finalizer for addon.")
		instance.Status.Lifecycle.Installed = addonmgrv1alpha1.Failed
		instance.Status.StartTime = 0
		instance.Status.Reason = reason
		return reconcile.Result{}, err
	}

	// Add addon to cache
	//r.addAddonToCache(req, addon, addonmgrv1alpha1.Pending)

	// Prereqs workflow
	prereqsPhase, err := r.runWorkflow(addonmgrv1alpha1.Prereqs, instance, wfl)
	instance.Status.Lifecycle.Prereqs = prereqsPhase
	if err != nil {
		reason := fmt.Sprintf("Addon %s/%s prereqs failed. %v", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance, "Warning", "Failed", reason)
		log.Error(err, "Addon prereqs workflow failed.")
		// if prereqs failed, set install status to failed as well so that STATUS is updated
		instance.Status.Lifecycle.Installed = addonmgrv1alpha1.Failed
		instance.Status.StartTime = 0
		instance.Status.Reason = reason

		return reconcile.Result{}, err
	}
	
	//handle Prereqs failure
	if instance.Status.Lifecycle.Prereqs == addonmgrv1alpha1.Failed {
		reason := fmt.Sprintf("Addon %s/%s Prereqs status is Failed", instance.Namespace, instance.Name)
		r.recorder.Event(instance, "Warning", "Failed", reason)
		log.Error(err, "Addon prereqs workflow failed.")
		// if prereqs failed, set install status to failed as well so that STATUS is updated
		instance.Status.Lifecycle.Installed = addonmgrv1alpha1.Failed
		instance.Status.StartTime = 0
		instance.Status.Reason = reason

		return reconcile.Result{}, fmt.Errorf(reason)
	}

	// Validate secrets are in the addon deployment namespace, this is here and not in validator b/c namespace must be used to validate.
	if instance.Status.Lifecycle.Prereqs == addonmgrv1alpha1.Succeeded {
		if err := r.validateSecrets(instance); err != nil {
			reason := fmt.Sprintf("Addon %s/%s could not validate secrets. %v", instance.Namespace, instance.Name, err)
			r.recorder.Event(instance, "Warning", "Failed", reason)
			log.Error(err, "Addon could not validate secrets.")
			instance.Status.Lifecycle.Installed = addonmgrv1alpha1.Failed
			instance.Status.StartTime = 0
			instance.Status.Reason = reason

			return reconcile.Result{}, err
		}

		phase, err := r.runWorkflow(addonmgrv1alpha1.Install, instance, wfl)
		instance.Status.Lifecycle.Installed = phase
		if err != nil {
			reason := fmt.Sprintf("Addon %s/%s could not be installed due to error. %v", instance.Namespace, instance.Name, err)
			r.recorder.Event(instance, "Warning", "Failed", reason)
			log.Error(err, "Addon install workflow failed.")
			instance.Status.Lifecycle.Installed = addonmgrv1alpha1.Failed
			instance.Status.StartTime = 0
			instance.Status.Reason = reason

			return reconcile.Result{}, err
		}

		//r.addAddonToCache(req, instance, phase)
	}

	// Observe resources matching selector labels.
	observed, err := r.observeResources(ctx, instance)
	if err != nil {
		reason := fmt.Sprintf("Addon %s/%s failed to find deployed resources. %v", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance, "Warning", "Failed", reason)
		log.Error(err, "Addon failed to find deployed resources.")
		instance.Status.Lifecycle.Installed = addonmgrv1alpha1.Failed
		instance.Status.StartTime = 0
		instance.Status.Reason = reason

		return reconcile.Result{}, err
	}

	if len(observed) > 0 {
		instance.Status.Resources = observed
	}

	return ctrl.Result{}, nil
}

func ignoreNotFound(err error) error {
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *AddonReconciler) runWorkflow(lifecycleStep addonmgrv1alpha1.LifecycleStep, addon *addonmgrv1alpha1.Addon, wfl workflows.AddonLifecycle) (addonmgrv1alpha1.ApplicationAssemblyPhase, error) {
	log := r.Log.WithValues("addon", fmt.Sprintf("%s/%s", addon.Namespace, addon.Name))
	var wt *addonmgrv1alpha1.WorkflowType
	switch lifecycleStep {
	case addonmgrv1alpha1.Prereqs:
		wt = &addon.Spec.Lifecycle.Prereqs
	case addonmgrv1alpha1.Install:
		wt = &addon.Spec.Lifecycle.Install
	case addonmgrv1alpha1.Delete:
		wt = &addon.Spec.Lifecycle.Delete
	case addonmgrv1alpha1.Validate:
		wt = &addon.Spec.Lifecycle.Validate
	default:
		err := fmt.Errorf("no workflowType of this type %s exists", lifecycleStep)
		log.Error(err, "lifecycleStep is not a field in LifecycleWorkflowSpec", "lifecycleStep", lifecycleStep)
		return addonmgrv1alpha1.Failed, err
	}

	if wt.Template == "" {
		// No workflow was provided, so mark as succeeded
		return addonmgrv1alpha1.Succeeded, nil
	}

	wfIdentifierName := addon.GetFormattedWorkflowName(wt, string(lifecycleStep))
	phase, err := wfl.Install(context.TODO(), wt, wfIdentifierName)
	if err != nil {
		return phase, err
	}
	r.recorder.Event(addon, "Normal", "Completed", fmt.Sprintf("Completed %s workflow %s/%s.", strings.Title(string(lifecycleStep)), addon.Namespace, wfIdentifierName))
	return phase, nil
}

func (r *AddonReconciler) validateSecrets(addon *addonmgrv1alpha1.Addon) error {
	foundSecrets, err := r.dynClient.Resource(common.SecretGVR()).Namespace(addon.Spec.Params.Namespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	secretsList := make(map[string]struct{}, len(foundSecrets.Items))
	for _, foundSecret := range foundSecrets.Items {
		secretsList[foundSecret.UnstructuredContent()["metadata"].(map[string]interface{})["name"].(string)] = struct{}{}
	}

	for _, secret := range addon.Spec.Secrets {
		if _, ok := secretsList[secret.Name]; !ok {
			return fmt.Errorf("addon %s needs secret \"%s\" that was not found in namespace %s", addon.Name, secret.Name, addon.Spec.Params.Namespace)
		}
	}

	return nil
}

func (r *AddonReconciler) updateAddonStatus(ctx context.Context, addon *addonmgrv1alpha1.Addon) {
	log := r.Log.WithValues("addon", fmt.Sprintf("%s/%s", addon.Namespace, addon.Name))
	if err := r.Status().Update(ctx, addon); err != nil {
		log.Error(err, "Addon status could not be updated.")
		r.recorder.Event(addon, "Warning", "Failed", fmt.Sprintf("Addon %s/%s status could not be updated. %v", addon.Namespace, addon.Name, err))
	}
}

func (r *AddonReconciler) addAddonToCache(instance *addonmgrv1alpha1.Addon) {
	var version = addon.Version{
		Name:        instance.GetName(),
		Namespace:   instance.GetNamespace(),
		PackageSpec: instance.GetPackageSpec(),
		PkgPhase:    instance.GetInstallStatus(),
	}
	r.versionCache.AddVersion(version)
}

func (r *AddonReconciler) observeResources(ctx context.Context, a *addonmgrv1alpha1.Addon) ([]addonmgrv1alpha1.ObjectStatus, error) {
	var observed []addonmgrv1alpha1.ObjectStatus
	var labelSelector = a.Spec.Selector

	if len(labelSelector.MatchLabels) == 0 {
		labelSelector.MatchLabels = make(map[string]string)
	}
	// Always add app.kubernetes.io/managed-by and app.kubernetes.io/name to label to selector
	labelSelector.MatchLabels["app.kubernetes.io/managed-by"] = common.AddonGVR().Group
	labelSelector.MatchLabels["app.kubernetes.io/name"] = fmt.Sprintf("%s", a.GetName())

	selector, err := metav1.LabelSelectorAsSelector(&labelSelector)
	if err != nil {
		return observed, fmt.Errorf("label selector is invalid. %v", err)
	}

	generatedInformers := informers.NewSharedInformerFactory(r.generatedClient, time.Minute*30)

	for _, resc := range resources {

		gvk := resc.GetObjectKind().GroupVersionKind()
		_, kind := gvk.ToAPIVersionAndKind()

		gvr := schema.GroupVersionResource{
			Group:    gvk.Group,
			Version:  gvk.Version,
			Resource: inflection.Plural(strings.ToLower(kind)),
		}

		inf, err := generatedInformers.ForResource(gvr)
		if err != nil {
			return observed, err
		}

		objs, err := inf.Lister().ByNamespace(a.Spec.Params.Namespace).List(selector)
		if err != nil {
			return observed, err
		}

		for _, item := range objs {
			observed = append(observed, addonmgrv1alpha1.ObjectStatus{
				Kind:  gvk.Kind,
				Group: gvk.Group,
				Name:  item.(metav1.Object).GetName(),
				Link:  item.(metav1.Object).GetSelfLink(),
			})
		}
	}

	return observed, nil
}

// Finalize runs finalizer for addon
func (r *AddonReconciler) Finalize(ctx context.Context, addon *addonmgrv1alpha1.Addon, wfl workflows.AddonLifecycle, finalizerName string) error {
	// Has Delete workflow defined, let's run it.
	var removeFinalizer = true

	if addon.Spec.Lifecycle.Delete.Template != "" {

		removeFinalizer = false

		// Run delete workflow
		phase, err := r.runWorkflow(addonmgrv1alpha1.Delete, addon, wfl)
		if err != nil {
			return err
		}

		if phase == addonmgrv1alpha1.Succeeded || phase == addonmgrv1alpha1.Failed {
			// Wait for workflow to succeed or fail.
			removeFinalizer = true
		}
	}

	// Remove version from cache
	r.versionCache.RemoveVersion(addon.Spec.PkgName, addon.Spec.PkgVersion)

	// Remove finalizer from the list and update it.
	if removeFinalizer && common.ContainsString(addon.ObjectMeta.Finalizers, finalizerName) {
		addon.ObjectMeta.Finalizers = common.RemoveString(addon.ObjectMeta.Finalizers, finalizerName)
		if err := r.Update(ctx, addon); err != nil {
			return err
		}
	}

	return nil
}

// SetFinalizer adds finalizer to addon instances
func (r *AddonReconciler) SetFinalizer(ctx context.Context, addon *addonmgrv1alpha1.Addon, finalizerName string) error {
	// Resource is not being deleted
	if addon.ObjectMeta.DeletionTimestamp.IsZero() {
		// And does not contain finalizer
		if !common.ContainsString(addon.ObjectMeta.Finalizers, finalizerName) {
			// Set Finalizer
			addon.ObjectMeta.Finalizers = append(addon.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(ctx, addon); err != nil {
				return err
			}
		}
	}

	return nil
}
