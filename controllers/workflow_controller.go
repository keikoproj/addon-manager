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

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"github.com/go-logr/logr"
	addonapiv1 "github.com/keikoproj/addon-manager/api/addon"
	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	pkgaddon "github.com/keikoproj/addon-manager/pkg/addon"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	wfcontroller = "addon-manager-wf-controller"
)

type WorkflowReconciler struct {
	client       client.Client
	dynClient    dynamic.Interface
	log          logr.Logger
	versionCache pkgaddon.VersionCacheClient
	addonUpdater *pkgaddon.AddonUpdate
}

func NewWFController(mgr manager.Manager, dynClient dynamic.Interface, wfInf cache.SharedIndexInformer, addonversioncache pkgaddon.VersionCacheClient) error {
	addonUpdater := pkgaddon.NewAddonUpdate(mgr.GetClient(), ctrl.Log.WithName(wfcontroller), addonversioncache)
	r := &WorkflowReconciler{
		client:       mgr.GetClient(),
		dynClient:    dynClient,
		log:          ctrl.Log.WithName(wfcontroller),
		versionCache: addonversioncache,
		addonUpdater: addonUpdater,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&wfv1.Workflow{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(w client.Object) bool {
			// Only watch workflows in addon-manager-system namespace
			return w.GetNamespace() == addonapiv1.ManagedNameSpace
		}))).
		WithOptions(controller.Options{CacheSyncTimeout: addonapiv1.CacheSyncTimeout}).
		//Watches(&source.Informer{Informer: wfInf}, &handler.EnqueueRequestForOwner{
		//	IsController: true,
		//	OwnerType:    &addonv1.Addon{},
		//}).
		Complete(r)
}

func (r *WorkflowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	wfobj := &wfv1.Workflow{}
	err := r.client.Get(ctx, req.NamespacedName, wfobj)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get workflow %s: %#v", req, err)
	}
	r.log.Info("reconciling", "request", req, " workflow ", wfobj.Name)
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

	err = r.addonUpdater.UpdateAddonStatusLifecycle(ctx, wfobj.GetNamespace(), addonName, lifecycle, wfobj.Status.Phase)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// ExtractAddOnNameAndLifecycleStep extract addon-name and lifecyclestep from a workflow name string generated based on
// api types
func ExtractAddOnNameAndLifecycleStep(addonworkflowname string) (string, string, error) {
	if strings.Contains(addonworkflowname, string(addonv1.Prereqs)) {
		return strings.TrimSpace(addonworkflowname[:strings.Index(addonworkflowname, string(addonv1.Prereqs))-1]), string(addonv1.Prereqs), nil
	}

	if strings.Contains(addonworkflowname, string(addonv1.Install)) {
		return strings.TrimSpace(addonworkflowname[:strings.Index(addonworkflowname, string(addonv1.Install))-1]), string(addonv1.Install), nil

	}
	if strings.Contains(addonworkflowname, string(addonv1.Delete)) {
		return strings.TrimSpace(addonworkflowname[:strings.Index(addonworkflowname, string(addonv1.Delete))-1]), string(addonv1.Delete), nil
	}

	return "", "", fmt.Errorf("no recognized lifecyclestep within ")
}
