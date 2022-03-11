package controllers

import (
	"context"
	"fmt"
	"strings"

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

	var wfl = workflows.NewWorkflowLifecycle(c.wfcli, c.informer, c.dynCli, addon, c.scheme, c.recorder)

	err := c.createAddon(ctx, addon, wfl)
	if err != nil {
		c.logger.Error("failed creating addon err :", err)
	}

	c.logger.Info("[handleAddonCreation] add ", addon.Namespace, "/", addon.Name, " into memory.")
	c.addAddonToCache(addon)

	return err
}

func (c *Controller) handleAddonUpdate(ctx context.Context, addon *addonv1.Addon) error {
	c.logger.Info("[handleAddonUpdate] ", addon.Namespace, "/", addon.Name)
	var errs []error

	if addon.Status.Lifecycle.Installed.DepPending() {
		c.logger.Info("[handleAddonUpdate]  %s/%s pending on dependencies.", addon.Namespace, addon.Name)
		ready, err := c.isDependenciesReady(ctx, addon)
		if err != nil || !ready {
			c.logger.Errorf("[handleAddonUpdate] addon %s/%s dependency is not ready", addon.Namespace, addon.Name)
			errs = append(errs, err)
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

	if !addon.ObjectMeta.DeletionTimestamp.IsZero() && addon.Status.Lifecycle.Installed != addonv1.Deleting {
		c.logger.Infof("[handleAddonUpdate]  %s/%s is being deleting.", addon.Namespace, addon.Name)
		var wfl = workflows.NewWorkflowLifecycle(c.wfcli, c.informer, c.dynCli, addon, c.scheme, c.recorder)
		err := c.Finalize(ctx, addon, wfl)
		if err != nil {
			reason := fmt.Sprintf("[handleAddonUpdate] Addon %s/%s could not be finalized. err %v", addon.Namespace, addon.Name, err)
			c.recorder.Event(addon, "Warning", "Failed", reason)
			c.logger.Error(reason)
			errs = append(errs, err)

			installPhase := addonv1.DeleteFailed
			if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, nil, &installPhase, reason); err != nil {
				c.logger.Error("[handleAddonUpdate] failed updating ", addon.Namespace, "/", addon.Name, " finalizing status ", err)
				errs = append(errs, err)
			}
		} else {
			installPhase := addonv1.Deleting
			if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, nil, &installPhase, ""); err != nil {
				c.logger.Error("[handleAddonUpdate] failed updating ", addon.Namespace, "/", addon.Name, " deleting status ", err)
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

		installPhase := addonv1.Failed
		if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, nil, &installPhase, reason); err != nil {
			c.logger.Error("[handleAddonUpdate] failed updating ", addon.Namespace, "/", addon.Name, " ttl expire status ", err)
			errs = append(errs, err)
		}
	}

	var changedStatus bool
	changedStatus, addon.Status.Checksum = c.validateChecksum(addon)
	if changedStatus {
		c.logger.Info("[handleAddonUpdate] addon ", addon.Namespace, "/", addon.Name, " spec checksum changes.")
		// Set ttl starttime if checksum has changed
		if err := c.resetAddonStatus(ctx, addon); err != nil {
			c.logger.Error("[handleAddonUpdate] failed reset addon status ", addon.Namespace, "/", addon.Name, " after spec change.", err)
			errs = append(errs, err)
		} else {
			c.logger.Info("[handleAddonUpdate] re install addon after spec changes.")
			err := c.handleAddonCreation(ctx, addon)
			if err != nil {
				c.logger.Error("[handleAddonUpdate] failed re-install addon ", addon.Namespace, "/", addon.Name, " after spec change.", err)
				errs = append(errs, err)
			}
		}
	}

	if len(errs) == 0 {
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
		_, err := c.addoncli.AddonmgrV1alpha1().Addons(addon.Namespace).Update(ctx, addon, metav1.UpdateOptions{})
		if err != nil {
			switch {
			case errors.IsNotFound(err):
				msg := fmt.Sprintf("[handleAddonDeletion] Addon %s/%s is not found. %v", addon.Namespace, addon.Name, err)
				c.logger.Error(msg)
				return fmt.Errorf(msg)
			case strings.Contains(err.Error(), "the object has been modified"):
				c.logger.Info("[handleAddonDeletion] retry updating object for deleted addon.")
				if _, err := c.addoncli.AddonmgrV1alpha1().Addons(addon.Namespace).Update(ctx, addon, metav1.UpdateOptions{}); err != nil {
					c.logger.Error("[handleAddonDeletion] failed retry updating ", addon.Namespace, addon.Name, " lifecycle status err ", err)
					return err
				}
			default:
				c.logger.Error("[handleAddonDeletion] failed updating ", addon.Namespace, addon.Name, " lifecycle status err ", err)
				return err
			}
		}

		err = c.Finalize(ctx, addon, wfl)
		if err != nil {
			reason := fmt.Sprintf("Addon %s/%s could not be finalized. %v", addon.Namespace, addon.Name, err)
			c.recorder.Event(addon, "Warning", "Failed", reason)
			installPhase := addonv1.DeleteFailed
			if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, nil, &installPhase, reason); err != nil {
				c.logger.Error("failed updating ", addon.Namespace, "/", addon.Name, " finalizing failure ", err)
				return err
			}
		}

		// For a better user experience we want to update the status and requeue
		if addon.Status.Lifecycle.Installed != addonv1.Deleting {
			installPhase := addonv1.Deleting
			if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, nil, &installPhase, ""); err != nil {
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
	c.logger.Info("[createAddon]] addon ", addon.Namespace, "/", addon.Name)

	addon.Status = addonv1.AddonStatus{
		StartTime: common.GetCurretTimestamp(),
		Lifecycle: addonv1.AddonStatusLifecycle{
			Prereqs:   "",
			Installed: "",
		},
		Reason:    "",
		Resources: make([]addonv1.ObjectStatus, 0),
	}
	_, addon.Status.Checksum = c.validateChecksum(addon)
	c.logger.Info("[createAddon] init ", addon.Namespace, "/", addon.Name, " status")

	// Set finalizer because it is from managed namespace
	if err := c.SetFinalizer(ctx, addon, addonapiv1.FinalizerName); err != nil {
		reason := fmt.Sprintf("[createAddon] Addon %s/%s could not add finalizer. %v", addon.Namespace, addon.Name, err)
		c.recorder.Event(addon, "Warning", "Failed", reason)
		c.logger.Error(reason)
		c.logger.Errorf(reason)

		installPhase := addonv1.Failed
		if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, nil, &installPhase, reason); err != nil {
			c.logger.Error("[createAddon] Failed updating ", addon.Namespace, "/", addon.Name, " finalizer error status err ", err)
			return err
		}
	}

	// already update status
	isAddonValid, err := c.handleValidation(ctx, addon)
	if err != nil || !isAddonValid {
		c.logger.Info("[createAddon] ", addon.Namespace, "/", addon.Name, " is invalid.")
		return fmt.Errorf("invalid addon, %#v", err)
	}

	return c.createAddonHelper(ctx, addon, wfl)
}

func (c *Controller) createAddonHelper(ctx context.Context, addon *addonv1.Addon, wfl workflows.AddonLifecycle) error {
	errors := []error{}

	if addon.Spec.Lifecycle.Prereqs.Template == "" && addon.Spec.Lifecycle.Install.Template == "" &&
		addon.Spec.Lifecycle.Delete.Template == "" && len(addon.Status.Lifecycle.Installed) == 0 {
		c.logger.Info("addon ", addon.Namespace, "/", addon.Name, " does not have any workflow template.")
		installPhase := addonv1.Succeeded
		if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, nil, &installPhase, ""); err != nil {
			c.logger.Error("[zero wf addon] failed updating ", addon.Namespace, "/", addon.Name, " status err ", err)
			return err
		}
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

	// some addon have delete template only
	if (addon.Spec.Lifecycle.Prereqs.Template == "" && addon.Spec.Lifecycle.Install.Template == "") && addon.Spec.Lifecycle.Delete.Template != "" {
		c.logger.Info("addon ", addon.Namespace, "/", addon.Name, " has delete template.")
		_, err := c.runWorkflow(addonv1.Delete, addon, wfl)
		prereq := addon.Status.Lifecycle.Prereqs
		installed := addon.Status.Lifecycle.Installed
		if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, &prereq, &installed, ""); err != nil {
			c.logger.Error("[createAddonHelper] failed updating ", addon.Namespace, "/", addon.Name, " delete workflow status err ", err)
			return err
		}
		if err != nil {
			c.logger.Error(" failed kick off delete workflow ", err)
			return err
		}
	}

	c.recorder.Event(addon, "Normal", "Completed", fmt.Sprintf("Addon %s/%s workflow is executed.", addon.Namespace, addon.Name))
	c.logger.Info("workflow installation completed. waiting for update.")

	return nil
}

func (c *Controller) executePrereqAndInstall(ctx context.Context, addon *addonv1.Addon, wfl workflows.AddonLifecycle) error {
	prereqsPhase, err := c.runWorkflow(addonv1.Prereqs, addon, wfl)
	if err != nil {
		reason := fmt.Sprintf("Addon %s/%s prereqs failed. %v", addon.Namespace, addon.Name, err)
		c.recorder.Event(addon, "Warning", "Failed", reason)
		c.logger.Error(err, "Addon prereqs workflow failed.")
		// if prereqs failed, set install status to failed as well so that STATUS is updated
		prereqPhase := addonv1.Failed
		if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, &prereqPhase, nil, reason); err != nil {
			c.logger.Error("[executePrereqAndInstall] failed updating ", addon.Namespace, "/", addon.Name, " prereqs workflow status err ", err)
			return err
		}
		return err
	}
	prereqPhase := prereqsPhase
	if err := c.validateSecrets(ctx, addon); err != nil {
		reason := fmt.Sprintf("Addon %s/%s could not validate secrets. %v", addon.Namespace, addon.Name, err)
		c.recorder.Event(addon, "Warning", "Failed", reason)
		c.logger.Error(err, "Addon could not validate secrets.")
		installPhase := addonv1.Failed
		if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, &prereqPhase, &installPhase, reason); err != nil {
			c.logger.Error("[executePrereqAndInstall] failed updating ", addon.Namespace, "/", addon.Name, " validate secrets status err ", err)
			return err
		}
		return err
	}

	phase, err := c.runWorkflow(addonv1.Install, addon, wfl)
	installPhase := phase
	if err != nil {
		reason := fmt.Sprintf("Addon %s/%s could not be installed due to error. %v", addon.Namespace, addon.Name, err)
		c.recorder.Event(addon, "Warning", "Failed", reason)
		c.logger.Error(err, "Addon install workflow failed.")
		addon.Status.Reason = reason
		if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, &prereqPhase, &installPhase, reason); err != nil {
			c.logger.Error("[executePrereqAndInstall] failed updating ", addon.Namespace, "/", addon.Name, " install workflow status err ", err)
			return err
		}
	}

	if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, &prereqPhase, &installPhase, ""); err != nil {
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
			// if err := c.updateAddon(ctx, addon); err != nil {
			// 	c.logger.Error("[SetFinalizer] failed updating addon ", addon.Namespace, addon.Name, " finalizer, err :", err)
			// 	return err
			// }
		}
	}
	return nil
}

// Finalize runs finalizer for addon
func (c *Controller) Finalize(ctx context.Context, addon *addonv1.Addon, wfl workflows.AddonLifecycle) error {
	if addon.Spec.Lifecycle.Delete.Template != "" {
		_, err := c.runWorkflow(addonv1.Delete, addon, wfl)
		if err != nil {
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

func (c *Controller) isDependenciesReady(ctx context.Context, addon *addonv1.Addon) (bool, error) {
	a := pkgaddon.NewAddonValidator(addon, c.versionCache, c.dynCli)
	if err := a.ValidateDependencies(); err != nil {
		reason := fmt.Sprintf("Addon %s/%s is waiting on dependencies to be installed. %v", addon.Namespace, addon.Name, err)
		c.logger.Info("addon ", addon.Namespace, "/", addon.Name, " has pending dependencies.")
		installedPhase := addonv1.ValidationFailed
		if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, nil, &installedPhase, reason); err != nil {
			c.logger.Error("failed verifying dependencies ", addon.Namespace, "/", addon.Name, " waiting on dependencies ", err)
			return false, err
		}
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
			addon.Status.Reason = reason
			c.logger.Error(reason)

			installedPhase := addonv1.DepPending
			if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, nil, &installedPhase, reason); err != nil {
				c.logger.Error("[handleValidation] failed updating ", addon.Namespace, "/", addon.Name, " waiting on dependencies ", err)
			}
			return false, err
		} else if err != nil && strings.HasPrefix(err.Error(), pkgaddon.ErrDepNotInstalled) {
			reason := fmt.Sprintf("[handleValidation] Addon %s/%s is waiting on dependencies to be installed. %v", addon.Namespace, addon.Name, err)
			// Record an event if addon is not valid
			c.recorder.Event(addon, "Normal", "Failed", reason)
			c.logger.Info(reason)
			c.logger.Error(reason)

			installedPhase := addonv1.ValidationFailed
			if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, nil, &installedPhase, reason); err != nil {
				c.logger.Error("[handleValidation] failed verifying dependencies ", addon.Namespace, "/", addon.Name, " waiting on dependencies ", err)
			}
			return false, err
		} else {
			reason := fmt.Sprintf("[handleValidation] Addon %s/%s is not valid. %v", addon.Namespace, addon.Name, err)
			// Record an event if addon is not valid
			c.recorder.Event(addon, "Warning", "Failed", reason)
			c.logger.Error(reason)

			installedPhase := addonv1.ValidationFailed
			if err := c.updateAddonLifeCycle(ctx, addon.Namespace, addon.Name, nil, &installedPhase, reason); err != nil {
				c.logger.Error("[handleValidation] failed updating ", addon.Namespace, "/", addon.Name, " validation ", err)
			}
			return false, err
		}
	}

	return true, nil
}
