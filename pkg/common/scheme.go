package common

import (
	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		panic(err)
	}

	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		panic(err)
	}

	if err := wfv1.AddToScheme(scheme); err != nil {
		panic(err)
	}

	if err := addonmgrv1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}

}

func GetAddonMgrScheme() *runtime.Scheme {
	return scheme
}
