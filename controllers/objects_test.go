package controllers

import (
	"reflect"
	"testing"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func init() {
	_ = addonmgrv1alpha1.AddToScheme(clientgoscheme.Scheme)
	_ = wfv1.AddToScheme(clientgoscheme.Scheme)
	_ = clientgoscheme.AddToScheme(clientgoscheme.Scheme)

}

// Declare test objects to be used in tests
var (
	objects = []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "TestService",
				Namespace:       "default",
				OwnerReferences: nil,
			},
		},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "TestJob",
				Namespace:       "default",
				OwnerReferences: nil,
			},
		},
		&batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "TestCronJob",
				Namespace:       "default",
				OwnerReferences: nil,
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "TestDeployment",
				Namespace:       "default",
				OwnerReferences: nil,
			},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "TestDaemonSet",
				Namespace:       "default",
				OwnerReferences: nil,
			},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "TestReplicaSet",
				Namespace:       "default",
				OwnerReferences: nil,
			},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "TestStatefulSet",
				Namespace:       "default",
				OwnerReferences: nil,
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "TestNamespace",
				Namespace:       "TestNamespace",
				OwnerReferences: nil,
			},
		},
	}
)

func TestObserveService(t *testing.T) {

	fakeCli := fake.NewClientBuilder().WithScheme(testScheme).WithRuntimeObjects(objects...).Build()

	type args struct {
		cli       client.Client
		namespace string
		selector  labels.Selector
	}
	tests := []struct {
		name    string
		args    args
		want    []addonmgrv1alpha1.ObjectStatus
		wantErr bool
	}{
		{
			name:    "Found-Service",
			args:    args{cli: fakeCli, namespace: "default", selector: labels.Everything()},
			want:    []addonmgrv1alpha1.ObjectStatus{{Link: "", Name: "TestService", Kind: "Service", Group: "", Status: ""}},
			wantErr: false,
		},
		{"No-Service", args{fakeCli, "invalid-namespace", labels.Everything()}, []addonmgrv1alpha1.ObjectStatus{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ObserveService(tt.args.cli, tt.args.namespace, tt.args.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("ObserveService() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ObserveService() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObserveJob(t *testing.T) {

	fakeCli := fake.NewClientBuilder().WithScheme(testScheme).WithRuntimeObjects(objects...).Build()

	type args struct {
		cli       client.Client
		namespace string
		selector  labels.Selector
	}
	tests := []struct {
		name string
		args struct {
			cli       client.Client
			namespace string
			selector  labels.Selector
		}
		want    []addonmgrv1alpha1.ObjectStatus
		wantErr bool
	}{
		{
			name:    "Found-Job",
			args:    args{cli: fakeCli, namespace: "default", selector: labels.Everything()},
			want:    []addonmgrv1alpha1.ObjectStatus{{Link: "", Name: "TestJob", Kind: "Job", Group: "batch/v1", Status: ""}},
			wantErr: false,
		},
		{"No-Job", args{fakeCli, "invalid-namespace", labels.Everything()}, []addonmgrv1alpha1.ObjectStatus{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ObserveJob(tt.args.cli, tt.args.namespace, tt.args.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("ObserveJob() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ObserveJob() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObserveCronJob(t *testing.T) {

	fakeCli := fake.NewClientBuilder().WithScheme(testScheme).WithRuntimeObjects(objects...).Build()

	type args struct {
		cli       client.Client
		namespace string
		selector  labels.Selector
	}
	tests := []struct {
		name    string
		args    args
		want    []addonmgrv1alpha1.ObjectStatus
		wantErr bool
	}{
		{
			name:    "Found-CronJob",
			args:    args{cli: fakeCli, namespace: "default", selector: labels.Everything()},
			want:    []addonmgrv1alpha1.ObjectStatus{{Link: "", Name: "TestCronJob", Kind: "CronJob", Group: "batch/v1", Status: ""}},
			wantErr: false,
		},
		{"No-CronJob", args{fakeCli, "invalid-namespace", labels.Everything()}, []addonmgrv1alpha1.ObjectStatus{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ObserveCronJob(tt.args.cli, tt.args.namespace, tt.args.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("ObserveCronJob() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ObserveCronJob() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObserveDeployment(t *testing.T) {

	fakeCli := fake.NewClientBuilder().WithScheme(testScheme).WithRuntimeObjects(objects...).Build()

	type args struct {
		cli       client.Client
		namespace string
		selector  labels.Selector
	}
	tests := []struct {
		name    string
		args    args
		want    []addonmgrv1alpha1.ObjectStatus
		wantErr bool
	}{
		{
			name:    "Found-Deployment",
			args:    args{cli: fakeCli, namespace: "default", selector: labels.Everything()},
			want:    []addonmgrv1alpha1.ObjectStatus{{Link: "", Name: "TestDeployment", Kind: "Deployment", Group: "apps/v1", Status: ""}},
			wantErr: false,
		},
		// {"No-Deployment", args{fakeCli, "invalid-namespace", labels.Everything()}, []addonmgrv1alpha1.ObjectStatus{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ObserveDeployment(tt.args.cli, tt.args.namespace, tt.args.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("ObserveDeployment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ObserveDeployment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObserveDaemonSet(t *testing.T) {

	fakeCli := fake.NewClientBuilder().WithScheme(testScheme).WithRuntimeObjects(objects...).Build()

	type args struct {
		cli       client.Client
		namespace string
		selector  labels.Selector
	}
	tests := []struct {
		name    string
		args    args
		want    []addonmgrv1alpha1.ObjectStatus
		wantErr bool
	}{
		{
			name:    "Found-DaemonSet",
			args:    args{cli: fakeCli, namespace: "default", selector: labels.Everything()},
			want:    []addonmgrv1alpha1.ObjectStatus{{Link: "", Name: "TestDaemonSet", Kind: "DaemonSet", Group: "apps/v1", Status: ""}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ObserveDaemonSet(tt.args.cli, tt.args.namespace, tt.args.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("ObserveDaemonSet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ObserveDaemonSet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObserveReplicaSet(t *testing.T) {

	fakeCli := fake.NewClientBuilder().WithScheme(testScheme).WithRuntimeObjects(objects...).Build()

	type args struct {
		cli       client.Client
		namespace string
		selector  labels.Selector
	}
	tests := []struct {
		name    string
		args    args
		want    []addonmgrv1alpha1.ObjectStatus
		wantErr bool
	}{
		{
			name:    "Found-ReplicaSet",
			args:    args{cli: fakeCli, namespace: "default", selector: labels.Everything()},
			want:    []addonmgrv1alpha1.ObjectStatus{{Link: "", Name: "TestReplicaSet", Kind: "ReplicaSe", Group: "apps/v1", Status: ""}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ObserveReplicaSet(tt.args.cli, tt.args.namespace, tt.args.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("ObserveReplicaSet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ObserveReplicaSet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObserveStatefulSet(t *testing.T) {

	fakeCli := fake.NewClientBuilder().WithScheme(testScheme).WithRuntimeObjects(objects...).Build()

	type args struct {
		cli      client.Client
		name     string
		selector labels.Selector
	}
	tests := []struct {
		name    string
		args    args
		want    []addonmgrv1alpha1.ObjectStatus
		wantErr bool
	}{
		{
			name:    "Found-StatefulSet",
			args:    args{cli: fakeCli, name: "TestStatefulSet", selector: labels.Everything()},
			want:    []addonmgrv1alpha1.ObjectStatus{{Link: "", Name: "TestStatefulSet", Kind: "StatefulSet", Group: "apps/v1", Status: ""}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ObserveStatefulSet(tt.args.cli, tt.args.name, tt.args.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("ObserveStatefulSet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ObserveStatefulSet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObserveNamespace(t *testing.T) {

	fakeCli := fake.NewClientBuilder().WithScheme(testScheme).WithRuntimeObjects(objects...).Build()

	type args struct {
		cli      client.Client
		name     string
		selector labels.Selector
	}
	tests := []struct {
		name    string
		args    args
		want    []addonmgrv1alpha1.ObjectStatus
		wantErr bool
	}{
		{
			name:    "Found-Namepspace",
			args:    args{cli: fakeCli, name: "TestNamespace", selector: labels.Everything()},
			want:    []addonmgrv1alpha1.ObjectStatus{{Link: "", Name: "TestNamespace", Kind: "Namespace", Group: "", Status: ""}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ObserveNamespace(tt.args.cli, tt.args.name, tt.args.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("ObserveNamespace() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ObserveNamespace() = %v, want %v", got, tt.want)
			}
		})
	}
}
