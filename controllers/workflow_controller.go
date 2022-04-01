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

	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	pkgaddon "github.com/keikoproj/addon-manager/pkg/addon"

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
	addonUpdater *pkgaddon.AddonUpdate
}

func NewWFController(mgr manager.Manager, stopChan <-chan struct{}, addonversioncache pkgaddon.VersionCacheClient) (controller.Controller, error) {
	addonUpdater := pkgaddon.NewAddonUpdate(mgr.GetClient(), ctrl.Log.WithName(wfcontroller), addonversioncache)
	r := &wfreconcile{
		client:       mgr.GetClient(),
		log:          ctrl.Log.WithName(wfcontroller),
		versionCache: addonversioncache,
		addonUpdater: addonUpdater,
	}

	c, err := controller.New(wfcontroller, mgr, controller.Options{Reconciler: r,
		CacheSyncTimeout: controllerCacheSyncTimedOut})
	if err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &wfv1.Workflow{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}

	return c, nil
}

func (r *wfreconcile) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if req.Namespace != workflowDeployedNS {
		return ctrl.Result{}, nil
	}

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
