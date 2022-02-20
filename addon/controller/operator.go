package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/keikoproj/addon-manager/pkg/addon"
	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/pkg/apis/addon/v1alpha1"

	"github.com/keikoproj/addon-manager/pkg/common"
	"github.com/keikoproj/addon-manager/pkg/workflows"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	addonv1versioned "github.com/keikoproj/addon-manager/pkg/client/clientset/versioned"
	addonv1listers "github.com/keikoproj/addon-manager/pkg/client/listers/addon/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	wflister "github.com/argoproj/argo-workflows/v3/pkg/client/listers/workflow/v1alpha1"
	"k8s.io/client-go/tools/record"
)

type OperatorConfig struct {
	addonclientset addonv1versioned.Interface
	scheme         *runtime.Scheme

	Client    client.Client
	dynClient dynamic.Interface

	versionCache addon.VersionCacheClient
	recorder     record.EventRecorder

	workflowLister wflister.WorkflowLister
}

func NewOperatorConfig(
	dynClient dynamic.Interface,
	addonclientset addonv1versioned.Interface,
	eventRecorder record.EventRecorder,
	workflowLister wflister.WorkflowLister) *OperatorConfig {
	return &OperatorConfig{
		versionCache:   addon.NewAddonVersionCacheClient(),
		dynClient:      dynClient,
		addonclientset: addonclientset,
		recorder:       eventRecorder,
		workflowLister: workflowLister,
	}
}

const (
	managedNamespace   = "addon-manager-system"
	finalizerName      = "delete.addonmgr.keikoproj.io"
	addonCompleteLabel = "addon.keikoproj.io"
	// addon ttl time
	TTL = time.Duration(3) * time.Hour // 1 hour
)

func (op *OperatorConfig) execAddon(ctx context.Context, instance *addonmgrv1alpha1.Addon,
	Client client.Client, scheme *runtime.Scheme) error {

	desired := instance.ObjectMeta.DeletionTimestamp.IsZero()
	if !desired {
		// Resource is being deleted, run finalizers and exit.
		// For a better user experience we want to update the status and requeue
		if instance.Status.Lifecycle.Installed != addonmgrv1alpha1.Deleting {
			// check workflow and its status
			workflow := fmt.Sprintf("%s-%s-%s-wf", instance.Name, "delete", instance.CalculateChecksum())
			wf, _ := op.workflowLister.Workflows(managedNamespace).Get(workflow)
			if wf == nil {
				var wfl = workflows.NewWorkflowLifecycle(Client, op.dynClient, instance, scheme, op.recorder)
				err := op.Finalize(ctx, instance, wfl, finalizerName)
				if err != nil {
					reason := fmt.Sprintf("Addon %s/%s could not be finalized. %v", instance.Namespace, instance.Name, err)
					op.recorder.Event(instance, "Warning", "Failed", reason)
					instance.Status.Lifecycle.Installed = addonmgrv1alpha1.DeleteFailed
					instance.Status.Reason = reason
					return op.updateAddonStatus(ctx, instance, op.addonclientset)
				}

				// Remove finalizer from the list and update it.
				if common.ContainsString(instance.ObjectMeta.Finalizers, finalizerName) {
					instance.ObjectMeta.Finalizers = common.RemoveString(instance.ObjectMeta.Finalizers, finalizerName)
					return op.updateAddonStatus(ctx, instance, op.addonclientset)
				}

				instance.Status.Lifecycle.Installed = addonmgrv1alpha1.Deleting
				return op.updateAddonStatus(ctx, instance, op.addonclientset)
			} else {
				if wf.Status.Phase.Completed() {
					instance.Status.Lifecycle.Installed = addonmgrv1alpha1.ApplicationAssemblyPhase(wf.Status.Phase)
					// Remove version from cache
					op.versionCache.RemoveVersion(instance.Spec.PkgName, instance.Spec.PkgVersion)

					// Remove finalizer from the list and update it.
					if common.ContainsString(instance.ObjectMeta.Finalizers, finalizerName) {
						instance.ObjectMeta.Finalizers = common.RemoveString(instance.ObjectMeta.Finalizers, finalizerName)
						return op.updateAddonStatus(ctx, instance, op.addonclientset)
					}
				}
			}
		}

		return nil
	}

	// Process addon instance
	var wfl = workflows.NewWorkflowLifecycle(Client, op.dynClient, instance, scheme, op.recorder)
	if procErr := op.processAddon(ctx, instance, wfl); procErr != nil {
		return fmt.Errorf("failed processing addon err %v", procErr)
	}

	// Always update cache, status except errors
	if err := op.addAddonToCache(instance); err != nil {
		return fmt.Errorf("failed adding addon into cache err %v", err)
	}

	if err := op.updateAddonStatus(ctx, instance, op.addonclientset); err != nil {
		return fmt.Errorf("failed updating addon status err %v", err)
	}

	return nil
}

func (op *OperatorConfig) addAddonToCache(instance *addonmgrv1alpha1.Addon) error {
	var version = addon.Version{
		Name:        instance.GetName(),
		Namespace:   instance.GetNamespace(),
		PackageSpec: instance.GetPackageSpec(),
		PkgPhase:    instance.GetInstallStatus(),
	}
	op.versionCache.AddVersion(version)
	log.Info("Adding version cache", "phase", version.PkgPhase)
	return nil
}

func (op *OperatorConfig) executePrereq(ctx context.Context, instance *addonmgrv1alpha1.Addon, wfl workflows.AddonLifecycle) error {

	msg := fmt.Sprintf("executing addon %s prereqs workflow.", instance.Name)
	log.Info(msg)

	wfname := instance.GetFormattedWorkflowName(addonmgrv1alpha1.Prereqs)
	log.Info("checking if prereqs wf ", wfname, " exists.")
	wf, err := op.workflowLister.Workflows(managedNamespace).Get(wfname)
	if err != nil || wf == nil {
		msg := fmt.Sprintf("failed getting wf %s/%s, err %v", managedNamespace, wfname, err)
		log.Info(msg)
	}
	if wf != nil {
		log.Info("prereqs wf ", wfname, " -- phase ", wf.Status.Phase)
		instance.Status.Lifecycle.Prereqs = addonmgrv1alpha1.ApplicationAssemblyPhase(wf.Status.Phase)
		if wf.Status.Phase == wfv1.WorkflowFailed {
			instance.Status.Reason = wf.Status.Message
		}
		return nil
	}

	log.Info("install prereqs wf ", wfname)
	instance.Status.Reason = ""
	prereqsPhase, err := op.runWorkflow(addonmgrv1alpha1.Prereqs, instance, wfl)
	if err != nil {
		reason := fmt.Sprintf("Addon %s/%s prereqs failed. %v", instance.Namespace, instance.Name, err)
		log.Error(err, "Addon prereqs workflow failed.")
		instance.Status.Lifecycle.Prereqs = addonmgrv1alpha1.Failed
		instance.Status.Reason = reason
		return nil
	}
	instance.Status.Lifecycle.Prereqs = prereqsPhase

	//handle Prereqs failure
	if instance.Status.Lifecycle.Prereqs == addonmgrv1alpha1.Failed {
		reason := fmt.Sprintf("Addon %s/%s Prereqs status is Failed", instance.Namespace, instance.Name)
		log.Error(err, "Addon prereqs workflow failed.")
		// if prereqs failed, set install status to failed as well so that STATUS is updated
		instance.Status.Lifecycle.Prereqs = addonmgrv1alpha1.Failed
		instance.Status.Reason = reason
	}

	return nil
}

func (op *OperatorConfig) runWorkflow(lifecycleStep addonmgrv1alpha1.LifecycleStep, addon *addonmgrv1alpha1.Addon, wfl workflows.AddonLifecycle) (addonmgrv1alpha1.ApplicationAssemblyPhase, error) {

	wt, err := addon.GetWorkflowType(lifecycleStep)
	if err != nil {
		log.Error(err, "lifecycleStep is not a field in LifecycleWorkflowSpec", "lifecycleStep", lifecycleStep)
		return addonmgrv1alpha1.Failed, err
	}

	if wt.Template == "" {
		// No workflow was provided, so mark as succeeded
		return addonmgrv1alpha1.Succeeded, nil
	}

	wfIdentifierName := addon.GetFormattedWorkflowName(lifecycleStep)
	if wfIdentifierName == "" {
		return addonmgrv1alpha1.Failed, fmt.Errorf("could not generate workflow template name")
	}
	phase, err := wfl.Install(context.TODO(), wt, wfIdentifierName)
	if err != nil {
		return phase, err
	}
	op.recorder.Event(addon, "Normal", "Completed", fmt.Sprintf("Completed %s workflow %s/%s.", strings.Title(string(lifecycleStep)), addon.Namespace, wfIdentifierName))
	return phase, nil
}

func (op *OperatorConfig) executeInstall(ctx context.Context, instance *addonmgrv1alpha1.Addon, wfl workflows.AddonLifecycle) error {
	msg := fmt.Sprintf("execute addon %s install workflow", instance.Name)
	log.Info(msg)
	if instance.Status.Lifecycle.Prereqs == addonmgrv1alpha1.Succeeded {
		if err := op.validateSecrets(ctx, instance); err != nil {
			reason := fmt.Sprintf("Addon %s/%s could not validate secrets. %v", instance.Namespace, instance.Name, err)
			log.Error(err, "Addon could not validate secrets.")
			instance.Status.Lifecycle.Installed = addonmgrv1alpha1.Failed
			instance.Status.Reason = reason
			return nil
		}

		phase, err := op.runWorkflow(addonmgrv1alpha1.Install, instance, wfl)
		instance.Status.Lifecycle.Installed = phase
		if err != nil {
			reason := fmt.Sprintf("Addon %s/%s could not be installed due to error. %v", instance.Namespace, instance.Name, err)
			log.Error(err, "Addon install workflow failed.")
			instance.Status.Reason = reason
			return nil
		}
	}

	return nil
}

func (op *OperatorConfig) validateSecrets(ctx context.Context, addon *addonmgrv1alpha1.Addon) error {
	foundSecrets, err := op.dynClient.Resource(common.SecretGVR()).Namespace(addon.Spec.Params.Namespace).List(ctx, metav1.ListOptions{})
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

func (op *OperatorConfig) processAddon(ctx context.Context, instance *addonmgrv1alpha1.Addon, wfl workflows.AddonLifecycle) error {

	// Calculate Checksum, returns true if checksum is changed
	// possibly configure command
	var changedStatus bool
	changedStatus, instance.Status.Checksum = addon.ValidateChecksum(instance)

	if changedStatus {
		msg := fmt.Sprintf("addon %s/%s spec changes trigger status reset.", instance.GetNamespace(), instance.GetName())
		log.Info(msg)
		// ReSet ttl starttime if checksum has changed
		instance.Status.StartTime = common.GetCurretTimestamp()

		// Clear out status and reason
		instance.Status.Lifecycle.Prereqs = ""
		instance.Status.Lifecycle.Installed = ""
		instance.Status.Reason = ""
	}

	if instance.Status.Lifecycle.Prereqs == "" {
		msg := fmt.Sprintf("addon %s/%s prereqs wf no status yet, trigger prereqs wf installation.", instance.GetNamespace(), instance.GetName())
		log.Info(msg)
		err := op.executePrereq(ctx, instance, wfl)
		if err != nil {
			return err
		}

		err = op.executeInstall(ctx, instance, wfl)
		if err != nil {
			return err
		}
	}

	msg := fmt.Sprintf("addon %s/%s prereqs status <%s> install status <%s>",
		instance.Namespace,
		instance.Name,
		instance.Status.Lifecycle.Prereqs, instance.Status.Lifecycle.Installed)
	log.Info(msg)
	if instance.Status.Lifecycle.Installed == "" && instance.Status.Lifecycle.Prereqs.Succeeded() {
		msg := fmt.Sprintf("addon %s/%s install wf not installed yet, trigger install wf installation.", instance.GetNamespace(), instance.GetName())
		log.Info(msg)
		err := op.executeInstall(ctx, instance, wfl)
		if err != nil {
			return err
		}
	}

	// Resources list
	instance.Status.Resources = make([]addonmgrv1alpha1.ObjectStatus, 0)

	// Check if addon installation expired.
	if instance.Status.Lifecycle.Installed == addonmgrv1alpha1.Pending && common.IsExpired(instance.Status.StartTime, TTL.Milliseconds()) {
		reason := fmt.Sprintf("Addon %s/%s ttl expired, starttime exceeded %s", instance.Namespace, instance.Name, TTL.String())
		err := fmt.Errorf(reason)
		log.Error(err, reason)
		op.recorder.Event(instance, "Warning", "Failed", reason)

		instance.Status.Lifecycle.Installed = addonmgrv1alpha1.Failed
		instance.Status.Reason = reason

		return err
	}

	// Validate Addon
	if ok, err := addon.NewAddonValidator(instance, op.versionCache, op.dynClient).Validate(); !ok {
		// if an addons dependency is in a Pending state then make the parent addon Pending
		if err != nil && strings.HasPrefix(err.Error(), addon.ErrDepPending) {
			reason := fmt.Sprintf("Addon %s/%s is waiting on dependencies to be out of Pending state.", instance.Namespace, instance.Name)
			op.recorder.Event(instance, "Normal", "Pending", reason)
			instance.Status.Lifecycle.Installed = addonmgrv1alpha1.Pending
			instance.Status.Reason = reason

			log.Info(reason)
			return nil
		} else if err != nil && strings.HasPrefix(err.Error(), addon.ErrDepNotInstalled) {
			reason := fmt.Sprintf("Addon %s/%s is waiting on dependencies to be installed. %v", instance.Namespace, instance.Name, err)
			op.recorder.Event(instance, "Normal", "Failed", reason)
			instance.Status.Lifecycle.Installed = addonmgrv1alpha1.ValidationFailed
			instance.Status.Reason = reason

			log.Info(reason)
			return nil
		}

		reason := fmt.Sprintf("Addon %s/%s is not valid. %v", instance.Namespace, instance.Name, err)
		instance.Status.Lifecycle.Installed = addonmgrv1alpha1.ValidationFailed
		instance.Status.Reason = reason

		log.Error(err, "Failed to validate addon.")

		return err
	}

	op.recorder.Event(instance, "Normal", "Completed", fmt.Sprintf("Addon %s/%s is valid.", instance.Namespace, instance.Name))

	// Set finalizer only after addon is valid
	if err := op.SetFinalizer(ctx, instance, finalizerName); err != nil {
		reason := fmt.Sprintf("Addon %s/%s could not add finalizer. %v", instance.Namespace, instance.Name, err)
		log.Error(err, "Failed to add finalizer for addon.")
		instance.Status.Lifecycle.Installed = addonmgrv1alpha1.Failed
		instance.Status.Reason = reason
		return err
	}

	// Execute PreReq and Install workflow, if spec body has changed.
	// In the case when validation failed and continued here we should execute.
	// Also if workflow is in Pending state, execute it to update status to terminal state.
	if instance.Status.Lifecycle.Prereqs == addonmgrv1alpha1.Pending ||
		instance.Status.Lifecycle.Installed == addonmgrv1alpha1.Pending {
		log.Info("Addon Prereqs/Installed is on pending. Wait for workflow update.")
		return nil
	}

	return nil
}

// Finalize runs finalizer for addon
func (op *OperatorConfig) Finalize(ctx context.Context, addon *addonmgrv1alpha1.Addon, wfl workflows.AddonLifecycle, finalizerName string) error {
	// Has Delete workflow defined, let's run it.
	var removeFinalizer = true

	msg := fmt.Sprintf("addon <%s/%s> deletion status <%s>", addon.Namespace, addon.Name, addon.Status.Lifecycle.Installed)
	log.Info(msg)
	if addon.Spec.Lifecycle.Delete.Template != "" {
		removeFinalizer = false

		msg := fmt.Sprintf("deleting addon %s/%s", addon.Namespace, addon.Name)
		log.Info(msg)
		// Run delete workflow
		phase, err := op.runWorkflow(addonmgrv1alpha1.Delete, addon, wfl)
		if err != nil {
			return err
		}

		if phase == addonmgrv1alpha1.Succeeded || phase == addonmgrv1alpha1.Failed {
			// Wait for workflow to succeed or fail.
			removeFinalizer = true
		}

	}

	if removeFinalizer {
		// Remove version from cache
		op.versionCache.RemoveVersion(addon.Spec.PkgName, addon.Spec.PkgVersion)
	}
	return nil
}

func (op *OperatorConfig) updateAddonStatus(ctx context.Context, updating *addonmgrv1alpha1.Addon, addonclientset addonv1versioned.Interface) error {
	updated, err := op.addonclientset.AddonmgrV1alpha1().Addons(updating.Namespace).UpdateStatus(context.Background(), updating,
		metav1.UpdateOptions{})
	if err != nil || updated == nil {
		msg := fmt.Sprintf("Addon %s/%s status could not be updated. %v", updating.Namespace, updating.Name, err)
		log.Error(msg)
		return err
	}

	return nil
}

func (op *OperatorConfig) handleSrvAcntUpdate(obj interface{}, addonlister addonv1listers.AddonLister, addonclientset addonv1versioned.Interface) {
	log.Info("")
	un := obj.(*unstructured.Unstructured)
	addon := un.GetLabels()[LabelKeyAddonName]
	srvAcnt, err := common.FromUnstructured(un)
	if err != nil {
		panic(err)
	}

	objStatus := addonmgrv1alpha1.ObjectStatus{
		Kind:  "ServiceAccount",
		Group: "",
		Name:  srvAcnt.GetName(),
		Link:  srvAcnt.GetSelfLink(),
	}

	namespace := srvAcnt.GetNamespace()
	addonobj, err := addonlister.Addons(namespace).Get(addon)
	if err != nil || addonobj == nil {
		msg := fmt.Sprintf("failed getting addon %s/%s, err %v", namespace, addon, err)
		fmt.Print(msg)
		return

	}
	updating := addonobj.DeepCopy()
	resources := addonobj.Status.Resources
	updating.Status.Resources = []addonmgrv1alpha1.ObjectStatus{}
	updating.Status.Resources = append(updating.Status.Resources, resources...)
	updating.Status.Resources = append(updating.Status.Resources, objStatus)
	op.updateAddonStatus(context.TODO(), updating, addonclientset)
}

func (op *OperatorConfig) SetFinalizer(ctx context.Context, addon *addonmgrv1alpha1.Addon, finalizerName string) error {
	// Resource is not being deleted
	if addon.ObjectMeta.DeletionTimestamp.IsZero() {
		// And does not contain finalizer
		if !common.ContainsString(addon.ObjectMeta.Finalizers, finalizerName) {
			// Set Finalizer
			addon.ObjectMeta.Finalizers = append(addon.ObjectMeta.Finalizers, finalizerName)
			_, err := op.addonclientset.AddonmgrV1alpha1().Addons(addon.Namespace).Update(ctx, addon, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	}

	return nil
}
