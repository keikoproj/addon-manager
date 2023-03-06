package controllers

import (
	"testing"
	"time"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	logrtesting "github.com/go-logr/logr/testing"
	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	pkgaddon "github.com/keikoproj/addon-manager/pkg/addon"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	dynfake "k8s.io/client-go/dynamic/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	testScheme = runtime.NewScheme()
	rcdr       = record.NewBroadcasterForTests(1*time.Second).NewRecorder(testScheme, v1.EventSource{Component: "addons"})
)

func init() {
	_ = addonmgrv1alpha1.AddToScheme(testScheme)
	_ = wfv1.AddToScheme(testScheme)
	_ = clientgoscheme.AddToScheme(testScheme)
}

func TestWorkflowReconciler_Reconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	fakeCli := fake.NewClientBuilder().WithScheme(testScheme).WithObjects().Build()
	dynFakeCli := dynfake.NewSimpleDynamicClient(testScheme)
	log := logrtesting.TestLogger{T: t}
	addonUpdater := pkgaddon.NewAddonUpdater(rcdr, fakeCli, pkgaddon.NewAddonVersionCacheClient(), log)

	r := &WorkflowReconciler{
		client:       fakeCli,
		dynClient:    dynFakeCli,
		log:          log,
		addonUpdater: addonUpdater,
	}

	res, err := r.Reconcile(ctx, controllerruntime.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "test"}})
	g.Expect(err).To(gomega.BeNil())
	g.Expect(res).To(gomega.Equal(controllerruntime.Result{}))
}
