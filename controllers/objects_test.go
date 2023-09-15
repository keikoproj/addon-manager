package controllers

import (
	"reflect"
	"testing"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"k8s.io/apimachinery/pkg/labels"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func init() {
	_ = addonmgrv1alpha1.AddToScheme(clientgoscheme.Scheme)
	_ = wfv1.AddToScheme(clientgoscheme.Scheme)
	_ = clientgoscheme.AddToScheme(clientgoscheme.Scheme)
}

func TestObserveService(t *testing.T) {

	fakeCli := fake.NewClientBuilder().WithScheme(testScheme).Build()

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
		{"test1", args{fakeCli, "default", labels.Everything()}, []addonmgrv1alpha1.ObjectStatus{}, false},
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
