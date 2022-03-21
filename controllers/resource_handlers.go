package controllers

import (
	"context"
	"fmt"

	addonapiv1 "github.com/keikoproj/addon-manager/api/addon"
	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	batch_v1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
)

// attention: start/re-start, filter existing already processed resources
func (c *Controller) handleNamespaceAdd(ctx context.Context, obj interface{}) error {
	ns, ok := obj.(*v1.Namespace)
	if !ok {
		msg := "expecting v1 namespace"
		return fmt.Errorf(msg)
	}
	addonName, ok := ns.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.logger.Info("owner is not set. checking part of")
		addonName, ok = ns.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			msg := "[handleNamespaceAdd] failed getting addon name, should not happen"
			//c.logger.Error(err, msg)
			return fmt.Errorf(msg)
		}
	}
	key := fmt.Sprintf("%s/%s", c.namespace, addonName)
	nsStatus := addonv1.ObjectStatus{
		Kind:  "Namespace",
		Group: "",
		Name:  ns.GetName(),
		Link:  ns.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, nsStatus)
	if err != nil {
		//c.logger.Error(err, " failed updating namespace resource", " addon ", key)
		return err
	}
	return nil
}

func (c *Controller) handleNamespaceUpdate(ctx context.Context, obj interface{}) error {
	return nil
}

func (c *Controller) handleNamespaceDeletion(ctx context.Context, obj interface{}) error {
	return nil
}

func (c *Controller) handleDeploymentAdd(ctx context.Context, obj interface{}) error {
	deploy, ok := obj.(*appsv1.Deployment)
	if !ok {
		return fmt.Errorf("expecting appsv1 deployment")
	}
	addonName, ok := deploy.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.logger.Info("owner is not set. checking part of")
		addonName, ok = deploy.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			msg := "[handleDeploymentAdd] failed getting addon name, should not happen"
			//c.logger.Error(err, msg)
			return fmt.Errorf(msg)
		}
	}
	nsStatus := addonv1.ObjectStatus{
		Kind:  "Deployment",
		Group: "apps/v1",
		Name:  deploy.GetName(),
		Link:  deploy.GetSelfLink(),
	}
	key := fmt.Sprintf("%s/%s", c.namespace, addonName)
	err := c.updateAddonStatusResources(ctx, key, nsStatus)
	if err != nil {
		//c.logger.Error(err, "failed updating deployment resource", " addon ", key)
		return err
	}
	return nil
}

func (c *Controller) handleDeploymentUpdate(ctx context.Context, obj interface{}) error {
	return nil
}

func (c *Controller) handleDeploymentDeletion(ctx context.Context, obj interface{}) error {
	return nil
}

// attention: start/re-start, filter existing already processed resources
func (c *Controller) handleServiceAccountAdd(ctx context.Context, obj interface{}) error {
	srvacnt, ok := obj.(*v1.ServiceAccount)
	if !ok {
		msg := "expecting v1 ServiceAccount"
		//c.logger.Error(msg)
		return fmt.Errorf(msg)
	}
	addonName, ok := srvacnt.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.logger.Info("owner is not set. checking part of")
		addonName, ok = srvacnt.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			msg := "[handleServiceAccountAdd] failed getting addon name, should not happen"
			//c.logger.Error(err, msg)
			return fmt.Errorf(msg)
		}
	}
	key := fmt.Sprintf("%s/%s", c.namespace, addonName)
	nsStatus := addonv1.ObjectStatus{
		Kind:  "ServiceAccount",
		Group: "v1",
		Name:  srvacnt.GetName(),
		Link:  srvacnt.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, nsStatus)
	if err != nil {
		//c.logger.Error(err, "failed updating service account resource ", " addon ", key)
		return err
	}
	return nil
}

func (c *Controller) handleServiceAccountUpdate(ctx context.Context, obj interface{}) error {
	return nil
}

func (c *Controller) handleServiceAccountDeletion(ctx context.Context, obj interface{}) error {
	return nil
}

// attention: start/re-start, filter existing already processed resources
func (c *Controller) handleConfigMapAdd(ctx context.Context, obj interface{}) error {
	configmap, ok := obj.(*v1.ConfigMap)
	if !ok {
		msg := "expecting v1 ConfigMap"
		//c.logger.Error(msg)
		return fmt.Errorf(msg)
	}
	addonName, ok := configmap.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.logger.Info("owner is not set. checking part of")
		addonName, ok = configmap.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			msg := "[handleConfigMapAdd] failed getting addon name, should not happen"
			//c.logger.Error(msg)
			return fmt.Errorf(msg)
		}
	}
	key := fmt.Sprintf("%s/%s", c.namespace, addonName)
	nsStatus := addonv1.ObjectStatus{
		Kind:  "ConfigMap",
		Group: "v1",
		Name:  configmap.GetName(),
		Link:  configmap.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, nsStatus)
	if err != nil {
		//c.logger.Error(err, "failed updating configmap resource", " addon ", key)
		return err
	}
	return nil
}

func (c *Controller) handleConfigMapUpdate(ctx context.Context, obj interface{}) error {
	return nil
}

func (c *Controller) handleConfigMapDeletion(ctx context.Context, obj interface{}) error {
	return nil
}

// attention: start/re-start, filter existing already processed resources
func (c *Controller) handleClusterRoleAdd(ctx context.Context, obj interface{}) error {
	clsRole, ok := obj.(*rbac_v1.ClusterRole)
	if !ok {
		msg := "expecting v1 ClusterRole"
		//c.logger.Error(err,msg)
		return fmt.Errorf(msg)
	}
	addonName, ok := clsRole.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.logger.Info("owner is not set. checking part of")
		addonName, ok = clsRole.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			msg := "[handleClusterRoleAdd] failed getting addon name, should not happen"
			//c.logger.Error(msg)
			return fmt.Errorf(msg)
		}
	}
	key := fmt.Sprintf("%s/%s", c.namespace, addonName)
	nsStatus := addonv1.ObjectStatus{
		Kind:  "ClusterRole",
		Group: "v1",
		Name:  clsRole.GetName(),
		Link:  clsRole.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, nsStatus)
	if err != nil {
		//c.logger.Error(err, "failed updating cluster role resource", "addon", key)
		return err
	}
	return nil
}

func (c *Controller) handleClusterRoleUpdate(ctx context.Context, obj interface{}) error {
	return nil
}

func (c *Controller) handleClusterRoleDeletion(ctx context.Context, obj interface{}) error {
	return nil
}

func (c *Controller) handleClusterRoleBindingAdd(ctx context.Context, obj interface{}) error {
	clsRoleBnd, ok := obj.(*rbac_v1.ClusterRoleBinding)
	if !ok {
		msg := "expecting v1 ClusterRole"
		//c.logger.Error(msg)
		return fmt.Errorf(msg)
	}
	addonName, ok := clsRoleBnd.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.logger.Info("owner is not set. checking part of")
		addonName, ok = clsRoleBnd.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			msg := "[handleClusterRoleBindingAdd] failed getting addon name, should not happen"
			//c.logger.Error(msg)
			return fmt.Errorf(msg)
		}
	}
	key := fmt.Sprintf("%s/%s", c.namespace, addonName)
	nsStatus := addonv1.ObjectStatus{
		Kind:  "ClusterRoleBinding",
		Group: "rbac/v1",
		Name:  clsRoleBnd.GetName(),
		Link:  clsRoleBnd.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, nsStatus)
	if err != nil {
		//c.logger.Error(err, "failed updating ClusterRoleBinding ", "addon", key)
		return err
	}
	return nil
}

func (c *Controller) handleClusterRoleBindingUpdate(ctx context.Context, obj interface{}) error {
	return nil
}

func (c *Controller) handleClusterRoleBindingDeletion(ctx context.Context, obj interface{}) error {
	return nil
}

func (c *Controller) handleJobAdd(ctx context.Context, obj interface{}) error {
	job, ok := obj.(*batch_v1.Job)
	if !ok {
		msg := "expecting v1 Job"
		//c.logger.Error(msg)
		return fmt.Errorf(msg)
	}
	addonName, ok := job.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.logger.Info("owner is not set. checking part of")
		addonName, ok = job.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			msg := "[handleJobAdd] failed getting addon name, should not happen"
			//c.logger.Error(msg)
			return fmt.Errorf(msg)
		}
	}
	key := fmt.Sprintf("%s/%s", c.namespace, addonName)
	objStatus := addonv1.ObjectStatus{
		Kind:  "Job",
		Group: "batch/v1",
		Name:  job.GetName(),
		Link:  job.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, objStatus)
	if err != nil {
		//c.logger.Error(err, "failed job ", "addon", key)
		return err
	}
	return nil
}

func (c *Controller) handleCronJobAdd(ctx context.Context, obj interface{}) error {
	cjob, ok := obj.(*batch_v1.CronJob)
	if !ok {
		msg := "expecting v1 CronJob"
		//c.logger.Error(msg)
		return fmt.Errorf(msg)
	}
	addonName, ok := cjob.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.logger.Info("owner is not set. checking part of")
		addonName, ok = cjob.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			msg := "[handleCronJobAdd] failed getting addon name, should not happen"
			//.logger.Error(msg)
			return fmt.Errorf(msg)
		}
	}
	key := fmt.Sprintf("%s/%s", c.namespace, addonName)
	objStatus := addonv1.ObjectStatus{
		Kind:  "CronJob",
		Group: "batch/v1",
		Name:  cjob.GetName(),
		Link:  cjob.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, objStatus)
	if err != nil {
		//c.logger.Error(err, "failed CronJob ", " addon ", key)
		return err
	}
	return nil
}

func (c *Controller) handleReplicaSet(ctx context.Context, obj interface{}) error {
	replicaSet, ok := obj.(*appsv1.ReplicaSet)
	if !ok {
		msg := "expecting v1 DaemonSet"
		//c.logger.Error(msg)
		return fmt.Errorf(msg)
	}
	addonName, ok := replicaSet.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.logger.Info("owner is not set. checking part of")
		addonName, ok = replicaSet.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			msg := "[handleReplicaSet] failed getting addon name, should not happen"
			//c.logger.Error(msg)
			return fmt.Errorf(msg)
		}
	}
	key := fmt.Sprintf("%s/%s", c.namespace, addonName)
	objStatus := addonv1.ObjectStatus{
		Kind:  "ReplicaSet",
		Group: "apps/v1",
		Name:  replicaSet.GetName(),
		Link:  replicaSet.GetSelfLink(),
	}
	err := c.updateAddonStatusResources(ctx, key, objStatus)
	if err != nil {
		//c.logger.Error(err, "failed DaemonSet ", " addon ", key)
		return err
	}
	return nil
}

func (c *Controller) handleDaemonSet(ctx context.Context, obj interface{}) error {
	daemonSet, ok := obj.(*appsv1.DaemonSet)
	if !ok {
		msg := "expecting v1 DaemonSet"
		//c.logger.Error(msg)
		return fmt.Errorf(msg)
	}
	addonName, ok := daemonSet.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.logger.Info("owner is not set. checking part of")
		addonName, ok = daemonSet.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			msg := "[handleDaemonSet] failed getting addon name, should not happen"
			//c.logger.Error(msg)
			return fmt.Errorf(msg)
		}
	}
	key := fmt.Sprintf("%s/%s", c.namespace, addonName)
	objStatus := addonv1.ObjectStatus{
		Kind:  "DaemonSet",
		Group: "apps/v1",
		Name:  daemonSet.GetName(),
		Link:  daemonSet.GetSelfLink(),
	}

	err := c.updateAddonStatusResources(ctx, key, objStatus)
	if err != nil {
		//c.logger.Error(err, "failed DaemonSet ", key, " resource status.  err : ", err)
		return err
	}
	return nil
}

func (c *Controller) handleStatefulSet(ctx context.Context, obj interface{}) error {
	statefulSet, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		msg := "expecting v1 StatefulSet"
		//c.logger.Error(msg)
		return fmt.Errorf(msg)
	}
	addonName, ok := statefulSet.GetLabels()[addonapiv1.ResourceDefaultOwnLabel]
	if !ok {
		c.logger.Info("owner is not set. checking part of")
		addonName, ok = statefulSet.GetLabels()[addonapiv1.ResourceDefaultPartLabel]
		if !ok {
			msg := "[handleStatefulSet] failed getting addon name, should not happen"
			//c.logger.Error(msg)
			return fmt.Errorf(msg)
		}
	}
	key := fmt.Sprintf("%s/%s", c.namespace, addonName)
	objStatus := addonv1.ObjectStatus{
		Kind:  "StatefulSet",
		Group: "apps/v1",
		Name:  statefulSet.GetName(),
		Link:  statefulSet.GetSelfLink(),
	}

	err := c.updateAddonStatusResources(ctx, key, objStatus)
	if err != nil {
		//c.logger.Error(err, "failed StatefulSet ", key, " resource status.  err : ", err)
		return err
	}
	return nil
}
