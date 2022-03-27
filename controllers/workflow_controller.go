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
	"reflect"
	"strings"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"github.com/go-logr/logr"
	addonapiv1 "github.com/keikoproj/addon-manager/api/addon"
	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	pkgaddon "github.com/keikoproj/addon-manager/pkg/addon"
	"github.com/keikoproj/addon-manager/pkg/common"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	wfcontroller = "workflow-controller"
)

type wfreconcile struct {
	client       client.Client
	log          logr.Logger
	versionCache pkgaddon.VersionCacheClient
}

func NewWFController(mgr manager.Manager, stopChan <-chan struct{}, addonversioncache pkgaddon.VersionCacheClient) (controller.Controller, error) {
	r := &wfreconcile{
		client:       mgr.GetClient(),
		log:          ctrl.Log.WithName(wfcontroller),
		versionCache: addonversioncache,
	}

	c, err := controller.New(wfcontroller, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	err = c.Watch(&source.Kind{Type: &wfv1.Workflow{}}, r.enqueueRequestForOwner())
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *wfreconcile) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.log.Info("reconciling", "request", req)
	wfobj := &wfv1.Workflow{}
	err := r.client.Get(ctx, req.NamespacedName, wfobj)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get workflow %s: %#v", req, err)
	}

	if len(string(wfobj.Status.Phase)) == 0 {
		r.log.Info("workflow ", wfobj.GetNamespace(), wfobj.GetName(), " status", " is empty")
		return ctrl.Result{}, nil
	}

	addonName, lifecycle, err := ExtractAddOnNameAndLifecycleStep(wfobj.GetName())
	if err != nil {
		msg := fmt.Sprintf("could not extract addon/lifecycle from %s/%s workflow.",
			wfobj.GetNamespace(),
			wfobj.GetName())
		r.log.Info(msg)
		return ctrl.Result{}, fmt.Errorf(msg)
	}

	err = r.updateAddonStatusLifecycle(ctx, wfobj.GetNamespace(), addonName, lifecycle, wfobj.Status.Phase)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *wfreconcile) enqueueRequestForOwner() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(a client.Object) []reconcile.Request {
		var namespace = a.GetNamespace()
		if namespace == workflowDeployedNS {
			// Let's lookup addon related to this object.
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name:      a.GetName(),
					Namespace: namespace,
				},
				},
			}
		}
		return []reconcile.Request{}
	})
}

// extract addon-name and lifecyclestep from a workflow name string generated based on
// api types
func ExtractAddOnNameAndLifecycleStep(addonworkflowname string) (string, string, error) {
	if strings.Contains(addonworkflowname, "prereqs") {
		return strings.TrimSpace(addonworkflowname[:strings.Index(addonworkflowname, "prereqs")-1]), "prereqs", nil
	}

	if strings.Contains(addonworkflowname, "install") {
		return strings.TrimSpace(addonworkflowname[:strings.Index(addonworkflowname, "install")-1]), "install", nil

	}
	if strings.Contains(addonworkflowname, "delete") {
		return strings.TrimSpace(addonworkflowname[:strings.Index(addonworkflowname, "delete")-1]), "delete", nil
	}
	if strings.Contains(addonworkflowname, "validate") {
		return strings.TrimSpace(addonworkflowname[:strings.Index(addonworkflowname, "validate")-1]), "validate", nil
	}
	return "", "", fmt.Errorf("no recognized lifecyclestep within ")
}

func (c *wfreconcile) updateAddonStatusLifecycle(ctx context.Context, namespace, name string, lifecycle string, lifecyclestatus wfv1.WorkflowPhase) error {

	key := fmt.Sprintf("%s/%s", namespace, name)
	latest, err := c.getExistingAddon(ctx, key)
	if err != nil || latest == nil {
		return err
	}
	updating := latest.DeepCopy()
	prevStatus := latest.Status
	if c.isAddonCompleted(updating) && prevStatus.Lifecycle.Installed != addonv1.Deleting {
		return nil
	}

	// addon being deletion, skip non-delete wf update
	if lifecycle != "delete" &&
		prevStatus.Lifecycle.Installed == addonv1.Deleting {
		return nil
	}

	newStatus := addonv1.AddonStatus{
		Lifecycle: addonv1.AddonStatusLifecycle{},
		Resources: []addonv1.ObjectStatus{},
	}
	newStatus.Reason = prevStatus.Reason
	newStatus.Resources = append(newStatus.Resources, prevStatus.Resources...)
	newStatus.Checksum = prevStatus.Checksum
	newStatus.StartTime = prevStatus.StartTime
	if lifecycle == "prereqs" {
		newStatus.Lifecycle.Prereqs = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
		newStatus.Lifecycle.Installed = prevStatus.Lifecycle.Installed
		if newStatus.Lifecycle.Prereqs == addonv1.Failed {
			newStatus.Lifecycle.Installed = addonv1.Failed
			newStatus.Reason = "prereqs wf fails."
		}
	} else if lifecycle == "install" || lifecycle == "delete" {
		newStatus.Lifecycle.Installed = addonv1.ApplicationAssemblyPhase(lifecyclestatus)
		newStatus.Lifecycle.Prereqs = prevStatus.Lifecycle.Prereqs
		if addonv1.ApplicationAssemblyPhase(lifecyclestatus) == addonv1.Succeeded {
			newStatus.Reason = ""
		}

		// check whether need patch complete
		if lifecycle == "install" && newStatus.Lifecycle.Installed.Completed() {
			labels := updating.GetLabels()
			if labels == nil {
				labels = map[string]string{}
			}
			labels[addonapiv1.AddonCompleteLabel] = addonapiv1.AddonCompleteTrueKey
			updating.SetLabels(labels)

			updating.Status = newStatus
			if _, err := c.updateAddon(ctx, updating); err != nil {
				return err
			}
			return nil
		}
	}
	updating.Status = newStatus

	if lifecycle == "delete" && addonv1.ApplicationAssemblyPhase(lifecyclestatus).Succeeded() {
		if prevStatus.Lifecycle.Installed.Completed() || prevStatus.Lifecycle.Installed.Deleting() {
			c.removeFinalizer(updating)
			if _, err := c.updateAddon(ctx, updating); err != nil {
				return err
			}
			c.removeFromCache(updating.Name)
			return nil
		}
	}

	var afterupdating *addonv1.Addon
	patchLabel := false
	if lifecycle == "install" && addonv1.ApplicationAssemblyPhase(lifecyclestatus).Succeeded() {
		labels := updating.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[addonapiv1.AddonCompleteLabel] = addonapiv1.AddonCompleteTrueKey
		updating.SetLabels(labels)
		afterupdating, err = c.updateAddon(ctx, updating)
		if err != nil {
			return err
		}
		patchLabel = true
	}

	if patchLabel {
		updating = afterupdating.DeepCopy()
	}

	if reflect.DeepEqual(prevStatus, updating.Status) {
		return nil
	}

	_, err = c.updateAddonStatus(ctx, updating)
	if err != nil {
		return err
	}
	return nil

}

func (c *wfreconcile) updateAddonStatus(ctx context.Context, addon *addonv1.Addon) (*addonv1.Addon, error) {
	latest := &addonv1.Addon{}
	err := c.client.Get(ctx, types.NamespacedName{Namespace: addon.Namespace, Name: addon.Name}, latest)
	if err != nil {
		msg := fmt.Sprintf("updateAddonStatus failed finding addon %s err %v.", addon.Name, err)
		return nil, fmt.Errorf(msg)
	}
	updating := latest.DeepCopy()
	if reflect.DeepEqual(updating.Status, addon.Status) {
		return nil, nil
	}

	updating.Status = addonv1.AddonStatus{
		Checksum: addon.Status.Checksum,
		Lifecycle: addonv1.AddonStatusLifecycle{
			Installed: addon.Status.Lifecycle.Installed,
			Prereqs:   addon.Status.Lifecycle.Prereqs,
		},
		Reason:    addon.Status.Reason,
		StartTime: addon.Status.StartTime,
		Resources: c.mergeResources(addon.Status.Resources, latest.Status.Resources),
	}

	//updated, err := c.addoncli.AddonmgrV1alpha1().Addons(updating.Namespace).UpdateStatus(ctx, updating, metav1.UpdateOptions{})
	err = c.client.Status().Update(ctx, updating, &client.UpdateOptions{})
	if err != nil {
		switch {
		case errors.IsNotFound(err):
			msg := fmt.Sprintf("[updateAddonStatus] addon %s/%s is not found. %v", updating.Namespace, updating.Name, err)
			return nil, fmt.Errorf(msg)
		case strings.Contains(err.Error(), "the object has been modified"):
			if _, err := c.updateAddonStatus(ctx, addon); err != nil {
				c.log.Error(err, fmt.Sprintf("[updateAddonStatus] failed retry updating %s/%s status", updating.Namespace, updating.Name))
			}
		default:
			//c.logger.Error("[updateAddonStatus] failed updating ", updating.Namespace, updating.Name, " status err ", err)
			return nil, err
		}
	}
	c.addAddonToCache(updating)

	return updating, nil
}

// add or remove complete label according to new instance
func (c *wfreconcile) mergeLabels(old, new map[string]string, merged map[string]string) {

	needAddComplete := false
	if _, ok := new[addonapiv1.AddonCompleteLabel]; ok {
		needAddComplete = true
	}

	if needAddComplete {
		if old != nil {
			if _, ok := old[addonapiv1.AddonCompleteLabel]; !ok {
				//c.logger.Infof("mergeLabels add complete label.")
				old[addonapiv1.AddonCompleteLabel] = addonapiv1.AddonCompleteTrueKey
			}

			for k, v := range old {
				merged[k] = v
			}
			return
		}
		merged[addonapiv1.AddonCompleteLabel] = addonapiv1.AddonCompleteTrueKey
		return
	}

	if _, ok := old[addonapiv1.AddonCompleteLabel]; ok {
		//c.logger.Infof("mergeLabels remove complete label.")
		delete(old, addonapiv1.AddonCompleteLabel)
	}
	for k, v := range old {
		merged[k] = v
	}
}

// add or remove addon finalizer according to new instance
func (c *wfreconcile) mergeFinalizer(old, new []string) []string {
	addFinalize := false
	for _, f := range new {
		if f == addonapiv1.FinalizerName {
			// should add finalizer
			addFinalize = true
			break
		}
	}

	if addFinalize {
		needappend := true
		for _, f := range old {
			if f == addonapiv1.FinalizerName {
				needappend = false
			}
		}
		if needappend {
			old = append(old, addonapiv1.FinalizerName)
		}
		//c.logger.Infof("mergeFinalizer after append addon finalizer %#v", old)
		return old
	}

	// otherwise, remove finalizer
	ret := []string{}
	for _, f := range old {
		if f == addonapiv1.FinalizerName {
			continue
		}
		ret = append(ret, f)
	}
	//c.logger.Infof("mergeFinalizer after remove addon finalizer %#v", ret)
	return ret
}

func (c *wfreconcile) mergeResources(res1, res2 []addonv1.ObjectStatus) []addonv1.ObjectStatus {
	merged := []addonv1.ObjectStatus{}
	check := make(map[string]addonv1.ObjectStatus)
	mix := append(res1, res2...)
	for _, obj := range mix {
		id := fmt.Sprintf("%s-%s-%s", strings.TrimSpace(obj.Name), strings.TrimSpace(obj.Kind), strings.TrimSpace(obj.Group))
		check[id] = obj
	}
	for _, obj := range check {
		merged = append(merged, obj)
	}
	return merged
}

func (c *wfreconcile) removeFinalizer(addon *addonv1.Addon) {
	if common.ContainsString(addon.ObjectMeta.Finalizers, addonapiv1.FinalizerName) {
		addon.ObjectMeta.Finalizers = common.RemoveString(addon.ObjectMeta.Finalizers, addonapiv1.FinalizerName)
	}
}

func (c *wfreconcile) removeFromCache(addonName string) {
	// Remove version from cache
	if ok, v := c.versionCache.HasVersionName(addonName); ok {
		c.versionCache.RemoveVersion(v.PkgName, v.PkgVersion)
	}
}

// update addon meta ojbect first, then update status
func (c *wfreconcile) updateAddon(ctx context.Context, updated *addonv1.Addon) (*addonv1.Addon, error) {
	var errs []error

	latest := &addonv1.Addon{}
	err := c.client.Get(ctx, types.NamespacedName{Namespace: updated.Namespace, Name: updated.Name}, latest)
	if err != nil || latest == nil {
		return nil, err
	} else {

		if reflect.DeepEqual(updated, latest) {
			c.log.Info(fmt.Sprintf("[updateAddon] latest and updated %s/%s is the same, skip", updated.Namespace, updated.Name))
			return nil, nil

		}

		// update object metata only
		updating := latest.DeepCopy()
		updating.Finalizers = c.mergeFinalizer(latest.Finalizers, updated.Finalizers)
		updating.ObjectMeta.Labels = map[string]string{}
		c.mergeLabels(latest.GetLabels(), updated.GetLabels(), updating.ObjectMeta.Labels)

		err := c.client.Status().Update(ctx, updating, &client.UpdateOptions{})
		if err != nil {
			switch {
			case errors.IsNotFound(err):
				msg := fmt.Sprintf("[updateAddon] Addon %s/%s is not found. %v", updated.Namespace, updated.Name, err)
				//c.logger.Error(msg)
				return nil, fmt.Errorf(msg)
			case strings.Contains(err.Error(), "the object has been modified"):
				errs = append(errs, err)
				c.log.Info(fmt.Sprintf("[updateAddon] retry updating object metadata %s/%s coz objects has been modified", updated.Namespace, updated.Name))
				if _, err := c.updateAddon(ctx, updated); err != nil {
					//c.logger.Errorf("[updateAddon] retry updating %s/%s, coz err %#v", updated.Namespace, updated.Name, err)
				}
			default:
				//c.logger.Error("[updateAddon] failed  ", updated.Namespace, updated.Name, " err ", err)
				return nil, err
			}
		}
		_, err = c.updateAddonStatus(ctx, updated)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return updated, nil
	}
	c.log.Error(err, fmt.Sprintf("[updateAddon] %s/%s failed.", updated.Namespace, updated.Name))
	return nil, fmt.Errorf("%v", errs)
}

func (c *wfreconcile) getExistingAddon(ctx context.Context, key string) (*addonv1.Addon, error) {
	info := strings.Split(key, "/")
	updating := &addonv1.Addon{}
	err := c.client.Get(ctx, types.NamespacedName{Namespace: info[0], Name: info[1]}, updating)
	if err != nil || updating == nil {
		return nil, fmt.Errorf("[getExistingAddon] failed converting to addon %s from client err %#v", key, err)
	}
	return updating, nil
}

func (c *wfreconcile) isAddonCompleted(addon *addonv1.Addon) bool {
	_, ok := addon.Labels[addonapiv1.AddonCompleteLabel]
	return ok && addon.Labels[addonapiv1.AddonCompleteLabel] == addonapiv1.AddonCompleteTrueKey
}

func (c *wfreconcile) addAddonToCache(addon *addonv1.Addon) {
	var version = pkgaddon.Version{
		Name:        addon.GetName(),
		Namespace:   addon.GetNamespace(),
		PackageSpec: addon.GetPackageSpec(),
		PkgPhase:    addon.GetInstallStatus(),
	}
	c.versionCache.AddVersion(version)
	c.log.WithValues("addAddonToCache", fmt.Sprintf("Adding %s/%s package %s version cache phase %s", addon.GetNamespace(), addon.GetName(), addon.GetPackageSpec(), version.PkgPhase))
}
