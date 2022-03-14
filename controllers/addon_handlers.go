package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"
	"github.com/keikoproj/addon-manager/pkg/workflows"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonapiv1 "github.com/keikoproj/addon-manager/api/addon"
	pkgaddon "github.com/keikoproj/addon-manager/pkg/addon"
)

func (c *Controller) handleAddonCreation(ctx context.Context, addon *addonv1.Addon) error {
	c.logger.Info("[handleAddonCreation]  ", addon.Namespace, "/", addon.Name)

	// check if addon being deletion
	deleting, err := c.isAddonBeingDeleting(ctx, addon)
	if deleting {
		c.logger.Info("[handleAddonCreation]  ", addon.Namespace, "/", addon.Name, " being deletion. skip")
		return err
	}

	// check if labelled with complete
	completed := c.isAddonCompleted(addon)
	if completed {
		c.logger.Info("[handleAddonCreation]  ", addon.Namespace, "/", addon.Name, " completed. skip")
		return nil
	}

	var wfl = workflows.NewWorkflowLifecycle(c.wfcli, c.informer, c.dynCli, addon, c.scheme, c.recorder)
	err = c.createAddon(ctx, addon, wfl)
	if err != nil {
		c.logger.Error("failed creating addon err :", err)
		return err
	}
	return nil
}

// check and process if addon is being deleted
func (c *Controller) isAddonBeingDeleting(ctx context.Context, addon *addonv1.Addon) (bool, error) {
	c.logger.Infof("isAddonBeingDeleting %s/%s.", addon.Namespace, addon.Name)

	if !addon.ObjectMeta.DeletionTimestamp.IsZero() {
		if addon.Status.Lifecycle.Installed == addonv1.Deleting || addon.Status.Lifecycle.Installed == addonv1.DeleteFailed {
			c.logger.Infof("isAddonBeingDeleting addon %s/%s deletion status %s.", addon.Namespace, addon.Name, addon.Status.Lifecycle.Installed)
			return true, nil
		}

		c.logger.Infof("[isAddonBeingDeleting]  %s/%s is being deleting.", addon.Namespace, addon.Name)
		var wfl = workflows.NewWorkflowLifecycle(c.wfcli, c.informer, c.dynCli, addon, c.scheme, c.recorder)
		err := c.Finalize(ctx, addon, wfl)
		if err != nil {
			reason := fmt.Sprintf("[isAddonBeingDeleting] Addon %s/%s could not be finalized. err %v", addon.Namespace, addon.Name, err)
			c.recorder.Event(addon, "Warning", "Failed", reason)
			c.logger.Error(reason)

			addon.Status.Lifecycle.Installed = addonv1.DeleteFailed
			addon.Status.Reason = reason
		} else {
			addon.Status.Lifecycle.Installed = addonv1.Deleting
		}

		if _, err := c.updateAddonStatus(ctx, addon); err != nil {
			c.logger.Error("[isAddonBeingDeleting] failed updating ", addon.Namespace, "/", addon.Name, " deletion status ", err)
		} else {
			c.logger.Infof("isAddonBeingDeleting %s/%s processed successfully.", addon.Namespace, addon.Name)
		}
		return true, err
	}

	c.logger.Infof("isAddonBeingDeleting %s/%s not being deletion.", addon.Namespace, addon.Name)
	return false, nil
}

func (c *Controller) handleAddonUpdate(ctx context.Context, addon *addonv1.Addon) error {
	c.logger.Info("[handleAddonUpdate] ", addon.Namespace, "/", addon.Name)
	var errs []error

	var changedStatus bool
	changedStatus, addon.Status.Checksum = c.validateChecksum(addon)
	if changedStatus {
		c.logger.Info("[handleAddonUpdate] addon ", addon.Namespace, "/", addon.Name, " spec checksum changes.")
		resetedAddon, err := c.resetAddonStatus(ctx, addon)
		// requeue should also be an option
		if err != nil || resetedAddon == nil {
			c.logger.Error("[handleAddonUpdate] failed reset addon status ", addon.Namespace, "/", addon.Name, " after spec change.", err)
			errs = append(errs, err)
		} else {
			c.logger.Info("[handleAddonUpdate] re install addon after spec changes.")
			err := c.handleAddonCreation(ctx, resetedAddon)
			if err != nil {
				c.logger.Error("[handleAddonUpdate] failed re-install addon ", addon.Namespace, "/", addon.Name, " after spec change.", err)
				errs = append(errs, err)
			}
		}
	}

	beingDeleting, err := c.isAddonBeingDeleting(ctx, addon)
	if beingDeleting {
		return err
	}

	if addon.Status.Lifecycle.Installed.DepPending() {
		c.logger.Infof("[handleAddonUpdate]  %s/%s pending on package dependencies.", addon.Namespace, addon.Name)
		ready, err := c.isDependenciesReady(ctx, addon)
		if err != nil || !ready {
			c.logger.Errorf("[handleAddonUpdate] addon %s/%s dependency is not ready", addon.Namespace, addon.Name)
			errs = append(errs, err)
			newEvent := Event{
				key:       fmt.Sprintf("%s/%s", addon.Namespace, addon.Name),
				eventType: "update",
			}
			c.queue.AddAfter(newEvent, 2*time.Second)
		} else {
			c.logger.Info("[handleAddonUpdate] ", addon.Namespace, "/", addon.Name, " resolves dependencies. ready to install")
			wfl := workflows.NewWorkflowLifecycle(c.wfcli, c.informer, c.dynCli, addon, c.scheme, c.recorder)
			err = c.createAddonHelper(ctx, addon, wfl)
			if err != nil {
				c.logger.Errorf("[handleAddonUpdate] failed kick off addon %s/%s wf after resolving dependencies. err %#v", addon.Namespace, addon.Name, err)
				errs = append(errs, err)
			}
		}
	}

	// Check if addon installation expired.
	if !addon.Status.Lifecycle.Installed.Completed() && common.IsExpired(addon.Status.StartTime, addonapiv1.TTL.Milliseconds()) {
		reason := fmt.Sprintf("[handleAddonUpdate] Addon %s/%s ttl expired, starttime exceeded %s", addon.Namespace, addon.Name, addonapiv1.TTL.String())
		c.recorder.Event(addon, "Warning", "Failed", reason)
		err := fmt.Errorf(reason)
		c.logger.Error(err, reason)

		addon.Status.Lifecycle.Installed = addonv1.Failed
		addon.Status.Reason = reason
		if _, err := c.updateAddonStatus(ctx, addon); err != nil {
			c.logger.Error("[handleAddonUpdate] failed updating ", addon.Namespace, "/", addon.Name, " ttl expire status ", err)
			errs = append(errs, err)
		}
	}

	// check if prereq completed while install wf not being kicked off yet
	if addon.Status.Lifecycle.Prereqs.Succeeded() && len(addon.Status.Lifecycle.Installed) == 0 {
		c.logger.Infof("[handleAddonUpdate] %s/%s prereq %s while install %s, run install wf", addon.Namespace, addon.Name, addon.Status.Lifecycle.Prereqs, addon.Status.Lifecycle.Installed)

		if err := c.validateSecrets(ctx, addon); err != nil {
			reason := fmt.Sprintf("handleAddonUpdate Addon %s/%s could not validate secrets. %v", addon.Namespace, addon.Name, err)
			c.recorder.Event(addon, "Warning", "Failed", reason)
			c.logger.Error(err, "handleAddonUpdate Addon could not validate secrets.")
			addon.Status.Lifecycle.Installed = addonv1.Failed
			addon.Status.Reason = reason
			if _, err := c.updateAddonStatus(ctx, addon); err != nil {
				c.logger.Error("[handleAddonUpdate] failed updating ", addon.Namespace, "/", addon.Name, " validate secrets status err ", err)
				return err
			}
			return err
		}

		var wfl = workflows.NewWorkflowLifecycle(c.wfcli, c.informer, c.dynCli, addon, c.scheme, c.recorder)
		phase, err := c.runWorkflow(addonv1.Install, addon, wfl)
		if err != nil {
			reason := fmt.Sprintf("handleAddonUpdate Addon %s/%s wf could not be installed due to error. %v", addon.Namespace, addon.Name, err)
			c.recorder.Event(addon, "Warning", "Failed", reason)
			c.logger.Error(err, "handleAddonUpdate Addon install workflow failed.")

			addon.Status.Reason = reason
			addon.Status.Lifecycle.Installed = phase
			if _, err := c.updateAddonStatus(ctx, addon); err != nil {
				c.logger.Error("[handleAddonUpdate] failed updating ", addon.Namespace, "/", addon.Name, " install workflow status err ", err)
				return err
			}
		}
		addon.Status.Lifecycle.Installed = phase
		if _, err := c.updateAddonStatus(ctx, addon); err != nil {
			c.logger.Error("[handleAddonUpdate] failed updating ", addon.Namespace, "/", addon.Name, "  execute install status err ", err)
			return err
		}
		return nil
	}

	noErr := true
	if len(errs) > 0 {
		for _, x := range errs {
			if x != nil {
				noErr = false
			}
		}
	}
	if noErr || len(errs) == 0 {
		c.logger.Infof("[handleAddonUpdate] %s/%s succeed.", addon.Namespace, addon.Name)
		return nil
	}

	c.logger.Infof("[handleAddonUpdate] %s/%s failed.", addon.Namespace, addon.Name)
	return fmt.Errorf("%v", errs)
}

func (c *Controller) handleAddonDeletion(ctx context.Context, addon *addonv1.Addon) error {
	c.logger.Info("[handleAddonDeletion] ", addon.Namespace, "/", addon.Name)
	if !addon.ObjectMeta.DeletionTimestamp.IsZero() {
		var wfl = workflows.NewWorkflowLifecycle(c.wfcli, c.informer, c.dynCli, addon, c.scheme, c.recorder)

		c.logger.Info("[handleAddonDeletion] addon ", addon.GetNamespace(), "/", addon.GetName(), " does not have install template. remove finalizer directly.")
		c.removeFinalizer(addon)
		c.unLabelComplete(addon)

		retry := 1
		for retry < 10 {
			_, err := c.addoncli.AddonmgrV1alpha1().Addons(addon.Namespace).Update(ctx, addon, metav1.UpdateOptions{})
			switch {
			case errors.IsNotFound(err):
				msg := fmt.Sprintf("[handleAddonDeletion] Addon %s/%s is not found. %v", addon.Namespace, addon.Name, err)
				c.logger.Error(msg)
				return fmt.Errorf(msg)
			case strings.Contains(err.Error(), "the object has been modified"):
				c.logger.Info("[handleAddonDeletion] retry updating object for deleted addon.")
				retry++
			default:
				c.logger.Error("[handleAddonDeletion] failed updating ", addon.Namespace, addon.Name, " lifecycle status err ", err)
				return err
			}
		}

		err := c.Finalize(ctx, addon, wfl)
		if err != nil {
			reason := fmt.Sprintf("Addon %s/%s could not be finalized. %v", addon.Namespace, addon.Name, err)
			c.recorder.Event(addon, "Warning", "Failed", reason)

			addon.Status.Lifecycle.Installed = addonv1.DeleteFailed
			addon.Status.Reason = reason
			if _, err := c.updateAddonStatus(ctx, addon); err != nil {
				c.logger.Error("failed updating ", addon.Namespace, "/", addon.Name, " finalizing failure ", err)
				return err
			}
		}

		// For a better user experience we want to update the status and requeue
		if addon.Status.Lifecycle.Installed != addonv1.Deleting {
			addon.Status.Lifecycle.Installed = addonv1.Deleting
			if _, err := c.updateAddonStatus(ctx, addon); err != nil {
				c.logger.Error("failed updating ", addon.Namespace, "/", addon.Name, " deleting status ", err)
				return err
			}
		}
	}
	return nil
}

func (c *Controller) removeFromCache(addonName string) {
	// Remove version from cache
	if ok, v := c.versionCache.HasVersionName(addonName); ok {
		c.versionCache.RemoveVersion(v.PkgName, v.PkgVersion)
	}
}

func (c *Controller) namespacenameFromKey(key string) (string, string) {
	info := strings.Split(key, "/")
	ns, name := info[0], info[1]
	return ns, name
}

func (c *Controller) createAddon(ctx context.Context, addon *addonv1.Addon, wfl workflows.AddonLifecycle) error {
	c.logger.Info("[createAddon] addon ", addon.Namespace, "/", addon.Name)

	changed, checksum := c.validateChecksum(addon)
	if changed {
		addon.Status = addonv1.AddonStatus{
			StartTime: common.GetCurretTimestamp(),
			Lifecycle: addonv1.AddonStatusLifecycle{
				Prereqs:   "",
				Installed: "",
			},
			Reason:    "",
			Resources: make([]addonv1.ObjectStatus, 0),
			Checksum:  checksum,
		}
	}

	if addon.Status.Lifecycle.Installed == addonv1.Init {
		c.logger.Info("[createAddon] ", addon.Namespace, "/", addon.Name, " to set init status")
		addon.Status.Lifecycle.Installed = addonv1.Init
	}

	latest, err := c.updateAddonStatus(ctx, addon)
	if err != nil {
		c.logger.Error("[createAddon] failed updating ", addon.Namespace, "/", addon.Name, " status ", err)
		return err
	}

	// status does not change, addon no update, keep previous addon object
	if latest == nil {
		latest = addon
	}

	// Check if addon installation expired.
	if !latest.Status.Lifecycle.Installed.Completed() && common.IsExpired(latest.Status.StartTime, addonapiv1.TTL.Milliseconds()) {
		reason := fmt.Sprintf("[createAddon] Addon %s/%s ttl expired, starttime exceeded %s", addon.Namespace, addon.Name, addonapiv1.TTL.String())
		c.recorder.Event(addon, "Warning", "Failed", reason)
		err := fmt.Errorf(reason)
		c.logger.Error(err, reason)
		addon.Status.Lifecycle.Installed = addonv1.Failed
		addon.Status.Reason = reason

		if _, err := c.updateAddonStatus(ctx, addon); err != nil {
			c.logger.Error("[handleAddonUpdate] failed updating ", addon.Namespace, "/", addon.Name, " ttl expire status ", err)
		}
		return err
	}

	// already update status
	isAddonValid, err := c.handleValidation(ctx, latest)
	if err != nil || !isAddonValid {
		c.logger.Info("[createAddon] ", addon.Namespace, "/", addon.Name, " is ", addon.Status.Lifecycle.Installed)
		return fmt.Errorf("invalid addon, %#v", err)
	}
	c.recorder.Event(addon, "Normal", "Completed", fmt.Sprintf("addon %s/%s is valid.", addon.Namespace, addon.Name))

	// Set finalizer only if it is valid
	if err := c.SetFinalizer(ctx, latest, addonapiv1.FinalizerName); err != nil {
		reason := fmt.Sprintf("[createAddon] Addon %s/%s could not add finalizer. %v", addon.Namespace, addon.Name, err)
		c.recorder.Event(addon, "Warning", "Failed", reason)
		c.logger.Errorf(reason)

		addon.Status.Lifecycle.Installed = addonv1.Failed
		addon.Status.Reason = reason

		if _, err := c.updateAddonStatus(ctx, addon); err != nil {
			c.logger.Error("[createAddon] Failed updating ", addon.Namespace, "/", addon.Name, " finalizer error status err ", err)
		}
		return err
	}

	err = c.createAddonHelper(ctx, latest, wfl)
	if err != nil {
		c.logger.Errorf("[createAddon] failed %s/%s err %#v", addon.Namespace, addon.Name, err)
	} else {
		c.logger.Infof("[createAddon] addon %s/%s successfully", addon.Namespace, addon.Name)
	}
	return err
}

func (c *Controller) createAddonHelper(ctx context.Context, addon *addonv1.Addon, wfl workflows.AddonLifecycle) error {
	errors := []error{}

	if addon.Spec.Lifecycle.Prereqs.Template == "" && addon.Spec.Lifecycle.Install.Template == "" {
		c.logger.Info("[createAddonHelper] addon ", addon.Namespace, "/", addon.Name, " does not have any workflow template.")
		addon.Status.Lifecycle.Installed = addonv1.Succeeded
		labels := addon.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[addonapiv1.AddonCompleteLabel] = addonapiv1.AddonCompleteTrueKey
		addon.SetLabels(labels)
		_, err := c.updateAddon(ctx, addon)
		if err != nil {
			c.logger.Errorf("[createAddonHelper] update addon %s/%s succeed and complete status.", addon.Namespace, addon.Name)
		}
		return nil
	}

	if addon.Spec.Lifecycle.Prereqs.Template != "" || addon.Spec.Lifecycle.Install.Template != "" {
		err := c.executePrereqAndInstall(ctx, addon, wfl)
		if err != nil {
			msg := fmt.Sprintf("failed installing addon %s/%s prereqs and instll err %v", addon.Namespace, addon.Name, err)
			c.logger.Error(msg)
			errors = append(errors, err)
			return fmt.Errorf("%v", errors)
		}
	} else {
		c.logger.Info("addon ", addon.Namespace, "/", addon.Name, " does not need prereqs or install.")
	}

	c.recorder.Event(addon, "Normal", "Completed", fmt.Sprintf("Addon %s/%s workflow is executed.", addon.Namespace, addon.Name))
	c.logger.Info("workflow installation completed. waiting for update.")
	return nil
}

func (c *Controller) executePrereqAndInstall(ctx context.Context, addon *addonv1.Addon, wfl workflows.AddonLifecycle) error {
	prereqsPhase, err := c.runWorkflow(addonv1.Prereqs, addon, wfl)
	if err != nil {
		reason := fmt.Sprintf("Addon %s/%s prereqs wf execution failed. %v", addon.Namespace, addon.Name, err)
		c.recorder.Event(addon, "Warning", "Failed", reason)
		c.logger.Error(err, "Addon prereqs workflow execution failed.")

		addon.Status.Lifecycle.Prereqs = addonv1.Failed
		addon.Status.Reason = reason
		if _, err := c.updateAddonStatus(ctx, addon); err != nil {
			c.logger.Error("[executePrereqAndInstall] failed updating ", addon.Namespace, "/", addon.Name, " prereqs workflow execution status err ", err)
			return err
		}
		return err
	}

	if prereqsPhase == addonv1.Failed {
		reason := fmt.Sprintf("Addon %s/%s prereqs wf failed. %v", addon.Namespace, addon.Name, err)
		c.recorder.Event(addon, "Warning", "Failed", reason)
		c.logger.Error(err, "Addon prereqs workflow failed.")

		addon.Status.Lifecycle.Prereqs = addonv1.Failed
		addon.Status.Reason = reason
		if _, err := c.updateAddonStatus(ctx, addon); err != nil {
			c.logger.Error("[executePrereqAndInstall] failed updating ", addon.Namespace, "/", addon.Name, " prereqs workflow status err ", err)
			return err
		}
		return fmt.Errorf(reason)
	}

	addon.Status.Lifecycle.Prereqs = prereqsPhase
	c.logger.Infof("executePrereqAndInstall %s/%s prereqs status %s", addon.Namespace, addon.Name, prereqsPhase)
	if prereqsPhase == addonv1.Succeeded {
		if err := c.validateSecrets(ctx, addon); err != nil {
			reason := fmt.Sprintf("Addon %s/%s could not validate secrets. %v", addon.Namespace, addon.Name, err)
			c.recorder.Event(addon, "Warning", "Failed", reason)
			c.logger.Error(err, "Addon could not validate secrets.")
			addon.Status.Lifecycle.Installed = addonv1.Failed
			addon.Status.Reason = reason
			if _, err := c.updateAddonStatus(ctx, addon); err != nil {
				c.logger.Error("[executePrereqAndInstall] failed updating ", addon.Namespace, "/", addon.Name, " validate secrets status err ", err)
				return err
			}
			return err
		}

		phase, err := c.runWorkflow(addonv1.Install, addon, wfl)
		if err != nil {
			reason := fmt.Sprintf("Addon %s/%s wf could not be installed due to error. %v", addon.Namespace, addon.Name, err)
			c.recorder.Event(addon, "Warning", "Failed", reason)
			c.logger.Error(err, " Addon install workflow failed.")

			addon.Status.Reason = reason
			addon.Status.Lifecycle.Installed = phase
			if _, err := c.updateAddonStatus(ctx, addon); err != nil {
				c.logger.Error("[executePrereqAndInstall] failed updating ", addon.Namespace, "/", addon.Name, " install workflow status err ", err)
				return err
			}
		}
		addon.Status.Lifecycle.Installed = phase
	}

	if _, err := c.updateAddonStatus(ctx, addon); err != nil {
		c.logger.Error("[executePrereqAndInstall] failed updating ", addon.Namespace, "/", addon.Name, "  execute prereq and install status err ", err)
		return err
	}

	return nil
}

func (c *Controller) validateSecrets(ctx context.Context, addon *addonv1.Addon) error {
	foundSecrets, err := c.dynCli.Resource(common.SecretGVR()).Namespace(addon.Spec.Params.Namespace).List(ctx, metav1.ListOptions{})
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

func (c *Controller) addAddonToCache(addon *addonv1.Addon) {
	var version = pkgaddon.Version{
		Name:        addon.GetName(),
		Namespace:   addon.GetNamespace(),
		PackageSpec: addon.GetPackageSpec(),
		PkgPhase:    addon.GetInstallStatus(),
	}
	c.versionCache.AddVersion(version)
	c.logger.Info("Adding ", addon.GetNamespace(), "/", addon.GetName(), " package ", addon.GetPackageSpec(), " version cache phase ", version.PkgPhase)
}

func (c *Controller) runWorkflow(lifecycleStep addonv1.LifecycleStep, addon *addonv1.Addon, wfl workflows.AddonLifecycle) (addonv1.ApplicationAssemblyPhase, error) {

	wt, err := addon.GetWorkflowType(lifecycleStep)
	if err != nil {
		c.logger.Error(err, "lifecycleStep is not a field in LifecycleWorkflowSpec", "lifecycleStep", lifecycleStep)
		return addonv1.Failed, err
	}

	if wt.Template == "" {
		// No workflow was provided, so mark as succeeded
		return addonv1.Succeeded, nil
	}

	wfIdentifierName := addon.GetFormattedWorkflowName(lifecycleStep)
	if wfIdentifierName == "" {
		return addonv1.Failed, fmt.Errorf("could not generate workflow template name")
	}
	phase, err := wfl.Install(context.TODO(), wt, wfIdentifierName)
	if err != nil {
		return phase, err
	}
	c.recorder.Event(addon, "Normal", "Completed", fmt.Sprintf("Completed %s workflow %s/%s.", strings.Title(string(lifecycleStep)), addon.Namespace, wfIdentifierName))
	return phase, nil
}

// Calculates new checksum and validates if there is a diff
func (c *Controller) validateChecksum(addon *addonv1.Addon) (bool, string) {
	newCheckSum := addon.CalculateChecksum()

	if addon.Status.Checksum == newCheckSum {
		return false, newCheckSum
	}

	return true, newCheckSum
}

// SetFinalizer adds finalizer to addon instances
func (c *Controller) SetFinalizer(ctx context.Context, addon *addonv1.Addon, finalizerName string) error {
	// Resource is not being deleted
	if addon.ObjectMeta.DeletionTimestamp.IsZero() {
		// And does not contain finalizer
		if !common.ContainsString(addon.ObjectMeta.Finalizers, finalizerName) {
			// Set Finalizer
			addon.ObjectMeta.Finalizers = append(addon.ObjectMeta.Finalizers, finalizerName)
			if _, err := c.updateAddon(ctx, addon); err != nil {
				return err
			}
		}
	}
	return nil
}

// Finalize runs finalizer for addon
func (c *Controller) Finalize(ctx context.Context, addon *addonv1.Addon, wfl workflows.AddonLifecycle) error {
	if addon.Spec.Lifecycle.Delete.Template != "" {
		_, err := c.runWorkflow(addonv1.Delete, addon, wfl)
		if err != nil {
			c.logger.Errorf("Finalize execute delete %s/%s delete wf %#v", addon.Namespace, addon.Name, err)
			return err
		}
	}

	// Remove version from cache
	c.versionCache.RemoveVersion(addon.Spec.PkgName, addon.Spec.PkgVersion)
	return nil
}

func (c *Controller) removeFinalizer(addon *addonv1.Addon) {
	if common.ContainsString(addon.ObjectMeta.Finalizers, addonapiv1.FinalizerName) {
		addon.ObjectMeta.Finalizers = common.RemoveString(addon.ObjectMeta.Finalizers, addonapiv1.FinalizerName)
	}
}

func (c *Controller) unLabelComplete(addon *addonv1.Addon) {
	if _, ok := addon.GetLabels()[addonapiv1.AddonCompleteLabel]; ok {
		labels := addon.GetLabels()
		delete(labels, addonapiv1.AddonCompleteLabel)
		addon.SetLabels(labels)
	}
}

func (c *Controller) isDependenciesReady(ctx context.Context, addon *addonv1.Addon) (bool, error) {
	a := pkgaddon.NewAddonValidator(addon, c.versionCache, c.dynCli)
	if err := a.ValidateDependencies(); err != nil {
		c.logger.Infof("Addon %s/%s is waiting on dependencies to be installed. %v", addon.Namespace, addon.Name, err)
		return false, err
	}
	addon.Status.Lifecycle.Installed = addonv1.Init
	addon.Status.Reason = ""
	if _, err := c.updateAddonStatus(ctx, addon); err != nil {
		c.logger.Errorf("isDependenciesReady failed reset %s/%s status back to init %#v", addon.Namespace, addon.Name, err)
	}
	return true, nil
}

// handleValidation validate duplicates, dependencies etc.
// nil, true --> valid addon
func (c *Controller) handleValidation(ctx context.Context, addon *addonv1.Addon) (bool, error) {
	// Validate Addon
	if ok, err := pkgaddon.NewAddonValidator(addon, c.versionCache, c.dynCli).Validate(); !ok {
		// if an addons dependency is in a Pending state then make the parent addon Pending
		if err != nil && strings.HasPrefix(err.Error(), pkgaddon.ErrDepPending) {
			reason := fmt.Sprintf("[handleValidation] Addon %s/%s is waiting on dependencies to be out of Pending state.", addon.Namespace, addon.Name)
			// Record an event if addon is not valid
			c.recorder.Event(addon, "Normal", "Pending", reason)
			c.logger.Error(reason)

			addon.Status.Lifecycle.Installed = addonv1.DepPending
			addon.Status.Reason = reason
			if _, err := c.updateAddonStatus(ctx, addon); err != nil {
				c.logger.Error("[handleValidation] failed updating ", addon.Namespace, "/", addon.Name, " waiting on dependencies ", err)
			}
			return false, err
		} else if err != nil && strings.HasPrefix(err.Error(), pkgaddon.ErrDepNotInstalled) {
			reason := fmt.Sprintf("[handleValidation] Addon %s/%s is waiting on dependencies to be installed. %v", addon.Namespace, addon.Name, err)
			// Record an event if addon is not valid
			c.recorder.Event(addon, "Normal", "Failed", reason)
			c.logger.Error(reason)

			addon.Status.Lifecycle.Installed = addonv1.DepNotInstalled
			addon.Status.Reason = reason
			if _, err := c.updateAddonStatus(ctx, addon); err != nil {
				c.logger.Error("[handleValidation] failed verifying dependencies ", addon.Namespace, "/", addon.Name, " waiting on dependencies ", err)
			}
			return false, err
		} else {
			reason := fmt.Sprintf("[handleValidation] Addon %s/%s is not valid. %v", addon.Namespace, addon.Name, err)
			// Record an event if addon is not valid
			c.recorder.Event(addon, "Warning", "Failed", reason)
			c.logger.Error(reason)

			addon.Status.Lifecycle.Installed = addonv1.ValidationFailed
			addon.Status.Reason = reason
			if _, err := c.updateAddonStatus(ctx, addon); err != nil {
				c.logger.Error("[handleValidation] failed updating ", addon.Namespace, "/", addon.Name, " validation ", err)
			}
			return false, err
		}
	}

	return true, nil
}
