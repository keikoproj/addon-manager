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

	wfclientset "github.com/argoproj/argo-workflows/v3/pkg/client/clientset/versioned"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	addonapiv1 "github.com/keikoproj/addon-manager/api/addon"
	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/addon"
	"github.com/keikoproj/addon-manager/pkg/common"
	"github.com/keikoproj/addon-manager/pkg/workflows"
)

const (
	controllerName = "addon-manager-controller"
)

// AddonReconciler reconciles a Addon object
type AddonReconciler struct {
	client.Client
	Log          logr.Logger
	Scheme       *runtime.Scheme
	versionCache addon.VersionCacheClient
	addonUpdater *addon.AddonUpdater
	dynClient    dynamic.Interface
	recorder     record.EventRecorder

	wfcli      wfclientset.Interface
	wfinformer cache.SharedIndexInformer
}

// NewAddonReconciler returns an instance of AddonReconciler
func NewAddonReconciler(mgr manager.Manager, dynClient dynamic.Interface, wfInf cache.SharedIndexInformer, versionCache addon.VersionCacheClient, addonUpdater *addon.AddonUpdater) *AddonReconciler {
	wfcli := common.NewWFClient(mgr.GetConfig())
	if wfcli == nil {
		panic("workflow client could not be nil")
	}

	return &AddonReconciler{
		Client:       mgr.GetClient(),
		Log:          ctrl.Log.WithName(controllerName),
		Scheme:       mgr.GetScheme(),
		dynClient:    dynClient,
		recorder:     mgr.GetEventRecorderFor("addons"),
		wfcli:        wfcli,
		wfinformer:   wfInf,
		versionCache: versionCache,
		addonUpdater: addonUpdater,
	}
}

// +kubebuilder:rbac:groups=addonmgr.keikoproj.io,resources=addons,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=addonmgr.keikoproj.io,resources=addons/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,namespace=system,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=list
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=get;list;patch;create
// +kubebuilder:rbac:groups="",resources=namespaces;clusterroles;configmaps;events;pods;serviceaccounts;services,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments;daemonsets;replicasets;statefulsets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=extensions,resources=deployments;daemonsets;replicasets;ingresses,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=batch,resources=jobs;cronjobs,verbs=get;list;watch;create;update;patch

// Reconcile method for all addon requests
func (r *AddonReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("addon", req.NamespacedName)

	log.Info("Starting addon-manager reconcile...")
	var instance = &addonmgrv1alpha1.Addon{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		log.Info("Addon not found.")

		// Remove version from cache
		r.addonUpdater.RemoveFromCache(req.Name)

		return reconcile.Result{}, ignoreNotFound(err)
	}

	// Log the addon spec and the checksum
	changedStatus, newChecksum := r.validateChecksum(instance)
	log.Info("Addon spec", "spec", instance.Spec, "checksum", newChecksum, "changedStatus", changedStatus)

	return r.execAddon(ctx, log, instance)
}

func (r *AddonReconciler) execAddon(ctx context.Context, log logr.Logger, instance *addonmgrv1alpha1.Addon) (reconcile.Result, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Info(fmt.Sprintf("Error: Panic occurred during execAdd %s/%s due to %v", instance.Namespace, instance.Name, err))
		}
	}()

	var wfl = workflows.NewWorkflowLifecycle(r.wfcli, r.wfinformer, r.dynClient, instance, r.Scheme, r.recorder, r.Log)

	// Resource is being deleted, run finalizers and exit.
	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// For a better user experience we want to update the status and requeue
		if !instance.GetInstallStatus().Deleting() {
			instance.SetInstallStatus(addonmgrv1alpha1.Deleting)
			log.Info("Requeue to set deleting status")
			err := r.addonUpdater.UpdateStatus(ctx, log, instance)
			return reconcile.Result{}, err
		}

		err := r.Finalize(ctx, instance, wfl, addonapiv1.FinalizerName)
		if err != nil {
			reason := fmt.Sprintf("Addon %s/%s could not be finalized. %v", instance.Namespace, instance.Name, err)
			r.recorder.Event(instance, "Warning", "Failed", reason)
			log.Error(err, "Failed to finalize addon.")

			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	// Process addon instance
	ret, procErr := r.processAddon(ctx, log, instance, wfl)

	err := r.addonUpdater.UpdateStatus(ctx, log, instance)
	if err != nil {
		// Force retry when status fails to update
		return reconcile.Result{RequeueAfter: 1 * time.Second}, err
	}

	return ret, procErr
}

func NewAddonController(mgr manager.Manager, dynClient dynamic.Interface, wfInf cache.SharedIndexInformer, versionCache addon.VersionCacheClient, addonUpdater *addon.AddonUpdater) (controller.Controller, error) {
	r := NewAddonReconciler(mgr, dynClient, wfInf, versionCache, addonUpdater)

	// addon-manager deployed at earliest stage
	// watched namespace workflow deployed much later
	c, err := controller.New(controllerName, mgr,
		controller.Options{Reconciler: r,
			CacheSyncTimeout: addonapiv1.CacheSyncTimeout})
	if err != nil {
		return nil, err
	}

	// Watch for changes to kubernetes Resources matching addon labels.
	if err := c.Watch(source.Kind(mgr.GetCache(), &addonmgrv1alpha1.Addon{}), &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}

	if err := c.Watch(source.Kind(mgr.GetCache(), &appsv1.Deployment{}), r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(source.Kind(mgr.GetCache(), &v1.Service{}), r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(source.Kind(mgr.GetCache(), &appsv1.DaemonSet{}), r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(source.Kind(mgr.GetCache(), &appsv1.ReplicaSet{}), r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(source.Kind(mgr.GetCache(), &appsv1.StatefulSet{}), r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(source.Kind(mgr.GetCache(), &batchv1.Job{}), r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *AddonReconciler) enqueueRequestWithAddonLabel() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(_ context.Context, a client.Object) []reconcile.Request {
		var reqs = make([]reconcile.Request, 0)
		var labels = a.GetLabels()
		if name, ok := labels[addonapiv1.ResourceDefaultOwnLabel]; ok && strings.TrimSpace(name) != "" {
			// Lookup addon related to this object.
			if ok, v := r.versionCache.HasVersionName(name); ok {
				reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      v.Name,
					Namespace: v.Namespace,
				}})
			}
		}
		return reqs
	})
}

func (r *AddonReconciler) processAddon(ctx context.Context, log logr.Logger, instance *addonmgrv1alpha1.Addon, wfl workflows.AddonLifecycle) (reconcile.Result, error) {

	// Calculate Checksum, returns true if checksum is changed
	var changedStatus bool
	changedStatus, instance.Status.Checksum = r.validateChecksum(instance)

	// Resources list
	instance.Status.Resources = make([]addonmgrv1alpha1.ObjectStatus, 0)

	if changedStatus {
		// Delete old workflows
		if err := r.deleteOldWorkflows(ctx, log, instance); err != nil {
			log.Error(err, "Failed to delete old workflows.")
			return reconcile.Result{}, err
		}
		// Set ttl starttime if checksum has changed
		instance.Status.StartTime = common.GetCurrentTimestamp()

		// Clear out status and reason
		instance.ClearStatus()

		log.Info("Checksum changed, addon will be installed...")
		instance.SetPrereqAndInstallStatuses(addonmgrv1alpha1.Pending)
		log.Info("Requeue to set pending status")
		return reconcile.Result{Requeue: true}, nil
	}

	// Check if addon is already completed, if so, skip further reconcile
	if instance.Status.Lifecycle.Installed.Completed() {
		return reconcile.Result{}, nil
	}

	// Validate Addon
	if ok, err := addon.NewAddonValidator(instance, r.versionCache, r.dynClient).Validate(); !ok {
		// if an addons dependency is in a Pending state then make the parent addon Pending
		if err != nil && strings.HasPrefix(err.Error(), addon.ErrDepPending) {
			reason := fmt.Sprintf("Addon %s/%s is waiting on dependencies to be out of Pending state.", instance.Namespace, instance.Name)
			// Record an event if addon is not valid
			r.recorder.Event(instance, "Normal", "Pending", reason)
			log.Info(reason)
			instance.SetInstallStatus(addonmgrv1alpha1.Pending, reason)

			// requeue after 10 seconds
			return reconcile.Result{
				Requeue:      true,
				RequeueAfter: 10 * time.Second,
			}, nil
		} else if err != nil && strings.HasPrefix(err.Error(), addon.ErrDepNotInstalled) {
			reason := fmt.Sprintf("Addon %s/%s is waiting on dependencies to be installed. %v", instance.Namespace, instance.Name, err)
			// Record an event if addon is not valid
			r.recorder.Event(instance, "Normal", "Failed", reason)
			log.Info(reason)
			instance.SetInstallStatus(addonmgrv1alpha1.ValidationFailed, reason)

			// requeue after 30 seconds
			return reconcile.Result{
				Requeue:      true,
				RequeueAfter: 30 * time.Second,
			}, nil
		}

		reason := fmt.Sprintf("Addon %s/%s is not valid. %v", instance.Namespace, instance.Name, err)
		// Record an event if addon is not valid
		r.recorder.Event(instance, "Warning", "Failed", reason)
		log.Error(err, "Failed to validate addon.")
		instance.SetInstallStatus(addonmgrv1alpha1.ValidationFailed, reason)

		return reconcile.Result{}, err
	}

	// Record successful validation
	r.recorder.Event(instance, "Normal", "Completed", fmt.Sprintf("Addon %s/%s is valid.", instance.Namespace, instance.Name))

	// Set finalizer only after addon is valid
	if err := r.SetFinalizer(ctx, instance, addonapiv1.FinalizerName); err != nil {
		reason := fmt.Sprintf("Addon %s/%s could not add finalizer. %v", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance, "Warning", "Failed", reason)
		log.Error(err, "Failed to add finalizer for addon. Requeuing...")

		return reconcile.Result{Requeue: true}, err
	}

	// Execute PreReq and Install workflow, if spec body has changed.
	// In the case when validation failed and continued here we should execute.
	// Also, if workflow is in Pending state, execute it to update status to terminal state.
	if !instance.Status.Lifecycle.Installed.Completed() {
		// Check if addon installation expired.
		if common.IsExpired(instance.Status.StartTime, addonapiv1.TTL.Milliseconds()) {
			reason := fmt.Sprintf("Addon %s/%s ttl expired, starttime exceeded %s", instance.Namespace, instance.Name, addonapiv1.TTL.String())
			r.recorder.Event(instance, "Warning", "Failed", reason)
			err := fmt.Errorf(reason)
			log.Error(err, reason)
			instance.SetInstallStatus(addonmgrv1alpha1.Failed, reason)

			return reconcile.Result{}, err
		}

		log.Info("Addon spec is updated, workflows will be generated")

		err := r.executePrereqAndInstall(ctx, log, instance, wfl)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Observe resources matching selector labels.
	observed, err := r.observeResources(ctx, instance)
	if err != nil {
		reason := fmt.Sprintf("Addon %s/%s failed to find deployed resources. %v", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance, "Warning", "Failed", reason)
		log.Error(err, "Addon failed to find deployed resources.")
		instance.SetInstallStatus(addonmgrv1alpha1.Failed, reason)

		return reconcile.Result{}, err
	}

	if len(observed) > 0 {
		instance.Status.Resources = observed
	}

	// In case workflow controller doesn't update addon status
	if instance.GetInstallStatus().Running() {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func ignoreNotFound(err error) error {
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// runWorkflow attempts creation of wf if corresponding lifecycleStep is not already completed
func (r *AddonReconciler) runWorkflow(ctx context.Context, lifecycleStep addonmgrv1alpha1.LifecycleStep, addon *addonmgrv1alpha1.Addon, wfl workflows.AddonLifecycle) error {
	log := r.Log.WithValues("addon", fmt.Sprintf("%s/%s", addon.Namespace, addon.Name))

	if lifecycleStep == addonmgrv1alpha1.Prereqs && addon.GetPrereqStatus().Completed() {
		log.Info("Lifecycle completed, skipping workflow execution", "lifecycleStep", lifecycleStep)
		return nil
	} else if lifecycleStep == addonmgrv1alpha1.Install && addon.GetInstallStatus().Completed() {
		log.Info("Lifecycle completed, skipping workflow execution", "lifecycleStep", lifecycleStep)
		return nil
	} else if lifecycleStep == addonmgrv1alpha1.Delete && (addon.Status.Lifecycle.Installed == addonmgrv1alpha1.DeleteFailed || addon.Status.Lifecycle.Installed == addonmgrv1alpha1.DeleteSucceeded) {
		log.Info("Lifecycle completed, skipping workflow execution", "lifecycleStep", lifecycleStep)
		return nil
	}

	wt, err := addon.GetWorkflowType(lifecycleStep)
	if err != nil {
		log.Error(err, "lifecycleStep is not a field in LifecycleWorkflowSpec", "lifecycleStep", lifecycleStep)
		addon.SetStatusByLifecyleStep(lifecycleStep, addonmgrv1alpha1.Failed)
		return err
	}

	if wt.Template == "" {
		log.Info("Workflow template is empty, skipping workflow execution", "lifecycleStep", lifecycleStep)
		// No workflow was provided, so mark as succeeded
		addon.SetStatusByLifecyleStep(lifecycleStep, addonmgrv1alpha1.Succeeded)
		return nil
	}

	wfIdentifierName := addon.GetFormattedWorkflowName(lifecycleStep)
	if wfIdentifierName == "" {
		addon.SetStatusByLifecyleStep(lifecycleStep, addonmgrv1alpha1.Failed)
		return fmt.Errorf("could not generate workflow template name")
	}
	err = wfl.Install(ctx, workflows.NewWorkflowProxy(wfIdentifierName, wt, lifecycleStep))
	if err != nil {
		addon.SetStatusByLifecyleStep(lifecycleStep, addonmgrv1alpha1.Failed)
		return err
	}
	r.recorder.Event(addon, "Normal", "Submitted", fmt.Sprintf("Submitted %s workflow %s/%s.", strings.Title(string(lifecycleStep)), addon.Namespace, wfIdentifierName))
	return nil
}

func (r *AddonReconciler) validateSecrets(ctx context.Context, addon *addonmgrv1alpha1.Addon) error {
	foundSecrets, err := r.dynClient.Resource(common.SecretGVR()).Namespace(addon.Spec.Params.Namespace).List(ctx, metav1.ListOptions{})
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

// executePrereqAndInstall kicks off Prereq/Install workflows and sets ONLY Failed/Succeeded as statuses
func (r *AddonReconciler) executePrereqAndInstall(ctx context.Context, log logr.Logger, instance *addonmgrv1alpha1.Addon, wfl workflows.AddonLifecycle) error {
	// Execute PreReq workflow
	err := r.runWorkflow(ctx, addonmgrv1alpha1.Prereqs, instance, wfl)
	if err != nil {
		reason := fmt.Sprintf("Addon %s/%s prereqs failed. %v", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance, "Warning", "Failed", reason)
		log.Error(err, "Addon prereqs workflow failed.")

		return err
	}

	// Validate and Install workflow
	if instance.Status.Lifecycle.Prereqs.Succeeded() {
		if err := r.validateSecrets(ctx, instance); err != nil {
			reason := fmt.Sprintf("Addon %s/%s could not validate secrets. %v", instance.Namespace, instance.Name, err)
			r.recorder.Event(instance, "Warning", "Failed", reason)
			log.Error(err, "Addon could not validate secrets.")
			instance.SetInstallStatus(addonmgrv1alpha1.Failed, reason)

			return err
		}

		err := r.runWorkflow(ctx, addonmgrv1alpha1.Install, instance, wfl)
		if err != nil {
			reason := fmt.Sprintf("Addon %s/%s could not be installed due to error. %v", instance.Namespace, instance.Name, err)
			r.recorder.Event(instance, "Warning", "Failed", reason)
			log.Error(err, "Addon install workflow failed.")

			return err
		}
	} else if instance.Status.Lifecycle.Prereqs.Failed() {
		instance.SetInstallStatus(addonmgrv1alpha1.Failed) // reason already set in SetPrereqAndInstallStatuses(Failed)
	}

	return nil
}

func (r *AddonReconciler) observeResources(ctx context.Context, a *addonmgrv1alpha1.Addon) ([]addonmgrv1alpha1.ObjectStatus, error) {
	var observed []addonmgrv1alpha1.ObjectStatus
	var labelSelector = a.Spec.Selector

	if len(labelSelector.MatchLabels) == 0 {
		labelSelector.MatchLabels = make(map[string]string)
	}
	// Always add app.kubernetes.io/managed-by and app.kubernetes.io/name to label selector
	labelSelector.MatchLabels["app.kubernetes.io/managed-by"] = common.AddonGVR().Group
	labelSelector.MatchLabels["app.kubernetes.io/name"] = fmt.Sprintf("%s", a.GetName())

	selector, err := metav1.LabelSelectorAsSelector(&labelSelector)
	if err != nil {
		return observed, fmt.Errorf("label selector is invalid. %v", err)
	}

	var errs []error
	cli := r.Client

	res, err := ObserveService(cli, a.GetNamespace(), selector)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to observe resource %s for addon %s/%s: %w", "service", a.GetNamespace(), a.GetName(), err))
	}
	observed = append(observed, res...)
	res, err = ObserveJob(cli, a.GetNamespace(), selector)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to observe resource %s for addon %s/%s: %w", "job", a.GetNamespace(), a.GetName(), err))
	}
	observed = append(observed, res...)
	res, err = ObserveCronJob(cli, a.GetNamespace(), selector)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to observe resource %s for addon %s/%s: %w", "cronjob", a.GetNamespace(), a.GetName(), err))
	}
	observed = append(observed, res...)

	res, err = ObserveStatefulSet(cli, a.GetNamespace(), selector)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to observe resource %s for addon %s/%s: %w", "StatefulSet", a.GetNamespace(), a.GetName(), err))
	}
	observed = append(observed, res...)

	res, err = ObserveDeployment(cli, a.GetNamespace(), selector)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to observe resource %s for addon %s/%s: %w", "Deployment", a.GetNamespace(), a.GetName(), err))
	}
	observed = append(observed, res...)

	res, err = ObserveDaemonSet(cli, a.GetNamespace(), selector)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to observe resource %s for addon %s/%s: %w", "DaemonSet", a.GetNamespace(), a.GetName(), err))
	}
	observed = append(observed, res...)

	res, err = ObserveReplicaSet(cli, a.GetNamespace(), selector)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to observe resource %s for addon %s/%s: %w", "ReplicaSet", a.GetNamespace(), a.GetName(), err))
	}
	observed = append(observed, res...)

	if len(errs) > 0 {
		return observed, fmt.Errorf("observed err %v", errs)
	}
	return observed, nil
}

// Calculates new checksum and validates if there is a diff
func (r *AddonReconciler) validateChecksum(instance *addonmgrv1alpha1.Addon) (bool, string) {
	newCheckSum := instance.CalculateChecksum()

	if instance.Status.Checksum == newCheckSum {
		return false, newCheckSum
	}

	return true, newCheckSum
}

func (r *AddonReconciler) deleteOldWorkflows(ctx context.Context, log logr.Logger, addon *addonmgrv1alpha1.Addon) error {
	// Define the selector to get the workflows related to the addon
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			addonapiv1.ResourceDefaultOwnLabel: addon.Name,
		},
	}

	selectorString, err := metav1.LabelSelectorAsSelector(&labelSelector)
	if err != nil {
		log.Info(fmt.Sprintf("unable to generate selector string for %+v", labelSelector))
		return err
	}
	log.Info(fmt.Sprintf("deleting old workflows with selector %+v", selectorString))

	// List the workflows
	workflows, err := r.wfcli.ArgoprojV1alpha1().Workflows(addon.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selectorString.String(),
	})
	if err != nil {
		log.Info(fmt.Sprintf("unable to list old workflows with selectorString %+v", selectorString.String()))
		return err
	}
	log.Info(fmt.Sprintf("%d workflows found", len(workflows.Items)))
	for _, workflow := range workflows.Items {
		log.Info("found old workflow", "name", workflow.Name, "status", workflow.Status.Phase)
	}

	// Delete each workflow
	for _, workflow := range workflows.Items {
		if err := r.wfcli.ArgoprojV1alpha1().Workflows(addon.Namespace).Delete(ctx, workflow.Name, metav1.DeleteOptions{}); err != nil {
			log.Info(fmt.Sprintf("unable to delete old workflow: %+v", workflow.Name))
		}
		log.Info(fmt.Sprintf("deleted old workflow: %+v", workflow.Name))
	}

	return nil
}

// Finalize runs finalizer for addon
func (r *AddonReconciler) Finalize(ctx context.Context, addon *addonmgrv1alpha1.Addon, wfl workflows.AddonLifecycle, finalizerName string) error {
	// Has Delete workflow defined, let's run it.
	var removeFinalizer = true

	if addon.Spec.Lifecycle.Delete.Template != "" {

		removeFinalizer = false

		// Run delete workflow
		err := r.runWorkflow(ctx, addonmgrv1alpha1.Delete, addon, wfl)
		if err != nil {
			return err
		}

		if addon.GetInstallStatus().Succeeded() {
			// Wait for workflow to succeed.
			removeFinalizer = true
		}
	}

	// Remove version from cache
	r.versionCache.RemoveVersion(addon.Spec.PkgName, addon.Spec.PkgVersion)

	// Remove finalizer from the list and update it.
	if removeFinalizer {
		controllerutil.RemoveFinalizer(addon, finalizerName)
		if err := r.Update(ctx, addon); err != nil {
			reason := fmt.Sprintf("Addon %s/%s could not be deleted, %v", addon.Namespace, addon.Name, err)
			addon.SetInstallStatus(addonmgrv1alpha1.DeleteFailed, reason)
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
		if !controllerutil.ContainsFinalizer(addon, finalizerName) {
			// Set Finalizer
			controllerutil.AddFinalizer(addon, finalizerName)
			if err := r.Update(ctx, addon); err != nil {
				return err
			}
		}
	}

	return nil
}
