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
	"strings"

	"context"
	"fmt"

	"github.com/go-logr/logr"
	pkgaddon "github.com/keikoproj/addon-manager/pkg/addon"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	addonapiv1 "github.com/keikoproj/addon-manager/api/addon"
	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	batch_v1 "k8s.io/api/batch/v1"
	batch_v1beta1 "k8s.io/api/batch/v1beta1"
	rbac_v1 "k8s.io/api/rbac/v1"
)

const (
	resourceControllerName = "addon-manager-resource-controller"
)

type resourceReconcile struct {
	client       client.Client
	log          logr.Logger
	versionCache pkgaddon.VersionCacheClient
}

func NewResourceController(mgr manager.Manager, stopChan <-chan struct{}, addonversioncache pkgaddon.VersionCacheClient) (controller.Controller, error) {
	r := &resourceReconcile{
		client:       mgr.GetClient(),
		log:          ctrl.Log.WithName(resourceControllerName),
		versionCache: addonversioncache,
	}

	c, err := controller.New(resourceControllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &v1.Service{}}, r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &v1.ServiceAccount{}}, r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &v1.Namespace{}}, r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &v1.ConfigMap{}}, r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &rbac_v1.ClusterRole{}}, r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &rbac_v1.ClusterRoleBinding{}}, r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &appsv1.ReplicaSet{}}, r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &batchv1.Job{}}, r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &batch_v1beta1.CronJob{}}, r.enqueueRequestWithAddonLabel()); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *resourceReconcile) enqueueRequestWithAddonLabel() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(a client.Object) []reconcile.Request {
		var reqs = make([]reconcile.Request, 0)
		var labels = a.GetLabels()
		if name, ok := labels["app.kubernetes.io/name"]; ok && strings.TrimSpace(name) != "" {
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

// Reconcile addon dependent resources
func (r *resourceReconcile) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.log.Info("reconciling", "request", req)

	namespace := &v1.Namespace{}
	if err := r.client.Get(ctx, req.NamespacedName, namespace); err == nil {
		r.log.Info(fmt.Sprintf("processing namespace %s", namespace.Name))
		if err = r.handleNamespace(ctx, namespace); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed handling namespace %#v", err)
		}
		return reconcile.Result{}, nil
	}

	deploy := &appsv1.Deployment{}
	if err := r.client.Get(ctx, req.NamespacedName, deploy); err == nil {
		r.log.Info(fmt.Sprintf("processing Deployment %s", deploy.Name))
		err = r.handleDeployment(ctx, deploy)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed handling Deployment %#v", err)
		}
		return reconcile.Result{}, nil
	}

	var srvacnt *v1.ServiceAccount
	if err := r.client.Get(ctx, req.NamespacedName, srvacnt); err == nil {
		r.log.Info(fmt.Sprintf("processing ServiceAccount %s", srvacnt.Name))
		err = r.handleServiceAccount(ctx, srvacnt)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed handling ServiceAccount %#v", err)
		}
		return reconcile.Result{}, nil
	}

	var configmap *v1.ConfigMap
	if err := r.client.Get(ctx, req.NamespacedName, configmap); err == nil {
		r.log.Info(fmt.Sprintf("processing ConfigMap %s", namespace.Name))
		if err = r.handleConfigMap(ctx, configmap); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed handling ConfigMap %#v", err)
		}
		return reconcile.Result{}, nil
	}

	var clsRole *rbac_v1.ClusterRole
	if err := r.client.Get(ctx, req.NamespacedName, clsRole); err == nil {
		r.log.Info(fmt.Sprintf("processing ClusterRole %s", clsRole.Name))
		if err = r.handleClusterRole(ctx, clsRole); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed handling ClusterRole %#v", err)
		}
		return reconcile.Result{}, nil
	}

	var clsRoleBnd *rbac_v1.ClusterRoleBinding
	if err := r.client.Get(ctx, req.NamespacedName, clsRoleBnd); err == nil {
		r.log.Info(fmt.Sprintf("processing ClusterRoleBinding %s", clsRoleBnd.Name))
		if err = r.handleClusterRoleBinding(ctx, clsRoleBnd); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed handling ClusterRoleBinding %#v", err)
		}
		return reconcile.Result{}, nil
	}

	var job *batch_v1.Job
	if err := r.client.Get(ctx, req.NamespacedName, job); err == nil {
		r.log.Info(fmt.Sprintf("processing Job %s", job.Name))
		if err = r.handleJobAdd(ctx, job); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed handling Job %#v", err)
		}
		return reconcile.Result{}, nil
	}

	var cjob *batch_v1.CronJob
	if err := r.client.Get(ctx, req.NamespacedName, cjob); err == nil {
		r.log.Info(fmt.Sprintf("processing CronJob %s", cjob.Name))
		if err = r.handleCronJob(ctx, cjob); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed handling CronJob %#v", err)
		}
		return reconcile.Result{}, nil
	}

	var replicaSet *appsv1.ReplicaSet
	if err := r.client.Get(ctx, req.NamespacedName, replicaSet); err == nil {
		r.log.Info(fmt.Sprintf("processing ReplicaSet %s", replicaSet.Name))
		if err = r.handleReplicaSet(ctx, replicaSet); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed handling ReplicaSet %#v", err)
		}
		return reconcile.Result{}, nil
	}

	var daemonSet *appsv1.DaemonSet
	if err := r.client.Get(ctx, req.NamespacedName, daemonSet); err == nil {
		r.log.Info(fmt.Sprintf("processing daemonSet %s", daemonSet.Name))
		if err := r.handleDaemonSet(ctx, daemonSet); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed handling daemonSet %#v", err)
		}
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

func (c *resourceReconcile) handleNamespace(ctx context.Context, ns *v1.Namespace) error {
	addonName, ok := ns.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.log.Info("owner is not set. checking part of")
		addonName, ok = ns.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			msg := "[handleNamespaceAdd] failed getting addon name, should not happen"
			return fmt.Errorf(msg)
		}
	}
	key := fmt.Sprintf("%s/%s", workflowDeployedNS, addonName)
	nsStatus := addonv1.ObjectStatus{
		Kind:  "Namespace",
		Group: "",
		Name:  ns.GetName(),
		Link:  ns.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, nsStatus)
	if err != nil {
		return err
	}
	return nil
}

func (c *resourceReconcile) handleDeployment(ctx context.Context, deploy *appsv1.Deployment) error {
	addonName, ok := deploy.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.log.Info("owner is not set. checking part of")
		addonName, ok = deploy.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			return fmt.Errorf("[handleDeploymentAdd] failed getting addon name, should not happen")
		}
	}
	nsStatus := addonv1.ObjectStatus{
		Kind:  "Deployment",
		Group: "apps/v1",
		Name:  deploy.GetName(),
		Link:  deploy.GetSelfLink(),
	}
	key := fmt.Sprintf("%s/%s", workflowDeployedNS, addonName)
	err := c.updateAddonStatusResources(ctx, key, nsStatus)
	if err != nil {
		return err
	}
	return nil
}

func (c *resourceReconcile) handleServiceAccount(ctx context.Context, srvacnt *v1.ServiceAccount) error {
	addonName, ok := srvacnt.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.log.Info("owner is not set. checking part of")
		addonName, ok = srvacnt.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			return fmt.Errorf("[handleServiceAccountAdd] failed getting addon name, should not happen")
		}
	}
	key := fmt.Sprintf("%s/%s", workflowDeployedNS, addonName)
	nsStatus := addonv1.ObjectStatus{
		Kind:  "ServiceAccount",
		Group: "v1",
		Name:  srvacnt.GetName(),
		Link:  srvacnt.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, nsStatus)
	if err != nil {
		return err
	}
	return nil
}

func (c *resourceReconcile) handleConfigMap(ctx context.Context, configmap *v1.ConfigMap) error {
	addonName, ok := configmap.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.log.Info("owner is not set. checking part of")
		addonName, ok = configmap.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			return fmt.Errorf("[handleConfigMapAdd] failed getting addon name, should not happen")
		}
	}
	key := fmt.Sprintf("%s/%s", workflowDeployedNS, addonName)
	nsStatus := addonv1.ObjectStatus{
		Kind:  "ConfigMap",
		Group: "v1",
		Name:  configmap.GetName(),
		Link:  configmap.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, nsStatus)
	if err != nil {
		return err
	}
	return nil
}

func (c *resourceReconcile) handleClusterRole(ctx context.Context, clsRole *rbac_v1.ClusterRole) error {
	addonName, ok := clsRole.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.log.Info("owner is not set. checking part of")
		addonName, ok = clsRole.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			return fmt.Errorf("[handleClusterRoleAdd] failed getting addon name, should not happen")
		}
	}
	key := fmt.Sprintf("%s/%s", workflowDeployedNS, addonName)
	nsStatus := addonv1.ObjectStatus{
		Kind:  "ClusterRole",
		Group: "v1",
		Name:  clsRole.GetName(),
		Link:  clsRole.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, nsStatus)
	if err != nil {
		return err
	}
	return nil
}

func (c *resourceReconcile) handleClusterRoleBinding(ctx context.Context, clsRoleBnd *rbac_v1.ClusterRoleBinding) error {
	addonName, ok := clsRoleBnd.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.log.Info("owner is not set. checking part of")
		addonName, ok = clsRoleBnd.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			return fmt.Errorf("[handleClusterRoleBindingAdd] failed getting addon name, should not happen")
		}
	}
	key := fmt.Sprintf("%s/%s", workflowDeployedNS, addonName)
	nsStatus := addonv1.ObjectStatus{
		Kind:  "ClusterRoleBinding",
		Group: "rbac/v1",
		Name:  clsRoleBnd.GetName(),
		Link:  clsRoleBnd.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, nsStatus)
	if err != nil {
		return err
	}
	return nil
}

func (c *resourceReconcile) handleJobAdd(ctx context.Context, job *batch_v1.Job) error {
	addonName, ok := job.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.log.Info("owner is not set. checking part of")
		addonName, ok = job.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			return fmt.Errorf("[handleJobAdd] failed getting addon name, should not happen")
		}
	}
	key := fmt.Sprintf("%s/%s", workflowDeployedNS, addonName)
	objStatus := addonv1.ObjectStatus{
		Kind:  "Job",
		Group: "batch/v1",
		Name:  job.GetName(),
		Link:  job.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, objStatus)
	if err != nil {
		return err
	}
	return nil
}

func (c *resourceReconcile) handleCronJob(ctx context.Context, cjob *batch_v1.CronJob) error {
	addonName, ok := cjob.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.log.Info("owner is not set. checking part of")
		addonName, ok = cjob.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			return fmt.Errorf("[handleCronJobAdd] failed getting addon name, should not happen")
		}
	}
	key := fmt.Sprintf("%s/%s", workflowDeployedNS, addonName)
	objStatus := addonv1.ObjectStatus{
		Kind:  "CronJob",
		Group: "batch/v1",
		Name:  cjob.GetName(),
		Link:  cjob.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, objStatus)
	if err != nil {
		return err
	}
	return nil
}

func (c *resourceReconcile) handleReplicaSet(ctx context.Context, replicaSet *appsv1.ReplicaSet) error {
	addonName, ok := replicaSet.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.log.Info("owner is not set. checking part of")
		addonName, ok = replicaSet.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			return fmt.Errorf("[handleReplicaSet] failed getting addon name, should not happen")
		}
	}
	key := fmt.Sprintf("%s/%s", workflowDeployedNS, addonName)
	objStatus := addonv1.ObjectStatus{
		Kind:  "ReplicaSet",
		Group: "apps/v1",
		Name:  replicaSet.GetName(),
		Link:  replicaSet.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, objStatus)
	if err != nil {
		return err
	}
	return nil
}

func (c *resourceReconcile) handleDaemonSet(ctx context.Context, daemonSet *appsv1.DaemonSet) error {
	addonName, ok := daemonSet.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.log.Info("owner is not set. checking part of")
		addonName, ok = daemonSet.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			return fmt.Errorf("[handleDaemonSet] failed getting addon name, should not happen")
		}
	}
	key := fmt.Sprintf("%s/%s", workflowDeployedNS, addonName)
	objStatus := addonv1.ObjectStatus{
		Kind:  "DaemonSet",
		Group: "apps/v1",
		Name:  daemonSet.GetName(),
		Link:  daemonSet.GetSelfLink(),
	}

	err := c.updateAddonStatusResources(ctx, key, objStatus)
	if err != nil {
		return err
	}
	return nil
}

func (c *resourceReconcile) updateAddonStatusResources(ctx context.Context, key string, resource addonv1.ObjectStatus) error {
	c.log.Info(fmt.Sprintf("updateAddonStatusResources %s resource %s", key, resource))

	updating, err := c.getExistingAddon(ctx, key)
	if err != nil || updating == nil {
		return err
	}

	c.log.Info("updateAddonStatusResources  ", "--", updating.Namespace, "--", updating.Name, " new resources -- ", resource, " existing resources -- ", updating.Status.Resources)
	newResources := []addonv1.ObjectStatus{resource}
	updating.Status.Resources = c.mergeResources(newResources, updating.Status.Resources)

	var errs []error
	err = c.client.Status().Update(ctx, updating, &client.UpdateOptions{})
	if err != nil {
		switch {
		case errors.IsNotFound(err):
			return err
		case strings.Contains(err.Error(), "the object has been modified"):
			c.log.Error(err, fmt.Sprintf("[updateAddonStatusResources] retry %s coz the object has been modified", resource))
			if err := c.updateAddonStatusResources(ctx, key, resource); err != nil {
				errs = append(errs, fmt.Errorf("[updateAddonStatusResources] failed to update addon %s/%s resources: %w", updating.Namespace,
					updating.Name, err))
			}
		default:
			errs = append(errs, fmt.Errorf("[updateAddonStatusResources] default failed to update addon %s/%s resources: %w", updating.Namespace,
				updating.Name, err))
		}
	}

	if len(errs) == 0 {
		c.log.Info("updateAddonStatusResources %s resource %s successfully", key, resource)
		return nil
	}

	c.log.Error(err, "updateAddonStatusResources failed processing %s resources %#v", key, errs)
	return fmt.Errorf("updateAddonStatusResources failed processing %s resources %#v", key, errs)
}

func (c *resourceReconcile) mergeResources(res1, res2 []addonv1.ObjectStatus) []addonv1.ObjectStatus {
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

func (c *resourceReconcile) getExistingAddon(ctx context.Context, key string) (*addonv1.Addon, error) {
	info := strings.Split(key, "/")
	updating := &addonv1.Addon{}
	err := c.client.Get(ctx, types.NamespacedName{Namespace: info[0], Name: info[1]}, updating)
	if err != nil || updating == nil {
		return nil, fmt.Errorf("[getExistingAddon] failed converting to addon %s from client err %#v", key, err)
	}
	return updating, nil
}
