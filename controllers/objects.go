package controllers

import (
	"context"
	"fmt"

	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/apis/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func ObserveService(cli client.Client, namespace string, selector labels.Selector) ([]addonmgrv1alpha1.ObjectStatus, error) {
	services := &v1.ServiceList{}
	err := cli.List(context.TODO(), services, &client.ListOptions{
		LabelSelector: selector,
		Namespace:     namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list services %v", err)
	}
	if services == nil {
		return nil, fmt.Errorf("services is empty")
	}
	res := []addonmgrv1alpha1.ObjectStatus{}
	for _, service := range services.Items {
		if service.ObjectMeta.Namespace == namespace {
			res = append(res, addonmgrv1alpha1.ObjectStatus{
				Kind:  "Service",
				Group: "",
				Name:  service.GetName(),
				Link:  service.GetSelfLink(),
			})
		}
	}
	return res, nil
}

func ObserveJob(cli client.Client, namespace string, selector labels.Selector) ([]addonmgrv1alpha1.ObjectStatus, error) {
	jobs := &batchv1.JobList{}
	err := cli.List(context.TODO(), jobs, &client.ListOptions{
		LabelSelector: selector,
		Namespace:     namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list cronjobs %v", err)
	}
	if jobs == nil {
		return nil, fmt.Errorf("batchv1.JobList is empty")
	}
	res := []addonmgrv1alpha1.ObjectStatus{}
	for _, job := range jobs.Items {
		res = append(res, addonmgrv1alpha1.ObjectStatus{
			Kind:  "Job",
			Group: "batch/v1",
			Name:  job.GetName(),
			Link:  job.GetSelfLink(),
		})
	}
	return res, nil
}

func ObserveCronJob(cli client.Client, namespace string, selector labels.Selector) ([]addonmgrv1alpha1.ObjectStatus, error) {
	cronJobs := &batchv1beta1.CronJobList{}
	err := cli.List(context.TODO(), cronJobs, &client.ListOptions{
		LabelSelector: selector,
		Namespace:     namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list cronjobs %v", err)
	}
	if cronJobs == nil {
		return nil, fmt.Errorf("batchv1beta1.CronJobList is empty")
	}
	res := []addonmgrv1alpha1.ObjectStatus{}
	for _, cronJob := range cronJobs.Items {
		res = append(res, addonmgrv1alpha1.ObjectStatus{
			Kind:  "CronJob",
			Group: "batch/v1beta1",
			Name:  cronJob.GetName(),
			Link:  cronJob.GetSelfLink(),
		})
	}
	return res, nil
}

func ObserveDeployment(cli client.Client, namespace string, selector labels.Selector) ([]addonmgrv1alpha1.ObjectStatus, error) {
	deployments := &appsv1.DeploymentList{}
	err := cli.List(context.TODO(), deployments, &client.ListOptions{
		LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	if deployments == nil {
		return nil, fmt.Errorf("failed to list deployments")
	}

	res := []addonmgrv1alpha1.ObjectStatus{}
	for _, deployment := range deployments.Items {
		res = append(res, addonmgrv1alpha1.ObjectStatus{
			Kind:  "Deployment",
			Group: "apps/v1",
			Name:  deployment.GetName(),
			Link:  deployment.GetSelfLink(),
		})
	}
	return res, nil
}

func ObserveDaemonSet(cli client.Client, namespace string, selector labels.Selector) ([]addonmgrv1alpha1.ObjectStatus, error) {
	daemonSets := &appsv1.DaemonSetList{}
	err := cli.List(context.TODO(), daemonSets, &client.ListOptions{
		LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	if daemonSets == nil {
		return nil, fmt.Errorf("failed to list daemonSets")
	}

	res := []addonmgrv1alpha1.ObjectStatus{}
	for _, deployment := range daemonSets.Items {
		res = append(res, addonmgrv1alpha1.ObjectStatus{
			Kind:  "DaemonSet",
			Group: "apps/v1",
			Name:  deployment.GetName(),
			Link:  deployment.GetSelfLink(),
		})
	}
	return res, nil
}

func ObserveReplicaSet(cli client.Client, namespace string, selector labels.Selector) ([]addonmgrv1alpha1.ObjectStatus, error) {
	replicaSets := &appsv1.ReplicaSetList{}
	err := cli.List(context.TODO(), replicaSets, &client.ListOptions{
		LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	if replicaSets == nil {
		return nil, fmt.Errorf("failed to list replicaSets")
	}

	res := []addonmgrv1alpha1.ObjectStatus{}
	for _, replicaSet := range replicaSets.Items {
		res = append(res, addonmgrv1alpha1.ObjectStatus{
			Kind:  "ReplicaSe",
			Group: "apps/v1",
			Name:  replicaSet.GetName(),
			Link:  replicaSet.GetSelfLink(),
		})
	}
	return res, nil
}

func ObserveStatefulSet(cli client.Client, name string, selector labels.Selector) ([]addonmgrv1alpha1.ObjectStatus, error) {
	statefulSets := &appsv1.StatefulSetList{}
	err := cli.List(context.TODO(), statefulSets, &client.ListOptions{
		LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	if statefulSets == nil {
		return nil, fmt.Errorf("failed to list replicaSets")
	}

	res := []addonmgrv1alpha1.ObjectStatus{}
	for _, statefulSet := range statefulSets.Items {
		res = append(res, addonmgrv1alpha1.ObjectStatus{
			Kind:  "StatefulSet",
			Group: "apps/v1",
			Name:  statefulSet.GetName(),
			Link:  statefulSet.GetSelfLink(),
		})
	}
	return res, nil
}

func ObserveNamespace(cli client.Client, name string, selector labels.Selector) ([]addonmgrv1alpha1.ObjectStatus, error) {
	namespaces := &corev1.NamespaceList{}
	err := cli.List(context.TODO(), namespaces, &client.ListOptions{
		LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	if namespaces == nil {
		return nil, fmt.Errorf("failed to list namespace")
	}

	res := []addonmgrv1alpha1.ObjectStatus{}
	for _, namespace := range namespaces.Items {
		if namespace.Name == name {
			res = append(res, addonmgrv1alpha1.ObjectStatus{
				Kind:  "Namespace",
				Group: "",
				Name:  namespace.GetName(),
				Link:  namespace.GetSelfLink(),
			})
		}
	}
	return res, nil
}
