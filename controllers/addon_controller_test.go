package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/scheme"
	"github.com/keikoproj/addon-manager/pkg/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	addonNamespace  = "default"
	addonName       = "cluster-autoscaler"
	addonKey        = types.NamespacedName{Name: addonName, Namespace: addonNamespace}
	addonController = &Controller{}
)

const timeout = time.Second * 5

var _ = Describe("AddonController", func() {

	var _ = BeforeEach(func() {
		log = zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
		logf.SetLogger(log)

		ctx, cancel = context.WithCancel(context.TODO())

		By("bootstrapping test environment")
		testEnv = &envtest.Environment{
			CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
			ErrorIfCRDPathMissing: true,
		}

		cfg, err := testEnv.Start()
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg).ToNot(BeNil())

		err = addonmgrv1alpha1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		By("starting reconciler and manager")
		k8sClient, err = client.New(cfg, client.Options{Scheme: common.GetAddonMgrScheme()})
		Expect(err).ToNot(HaveOccurred())
		Expect(k8sClient).ToNot(BeNil())

		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             common.GetAddonMgrScheme(),
			LeaderElection:     false,
			MetricsBindAddress: "0",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(mgr).ToNot(BeNil())

		stopMgr, wg = StartTestManager(mgr)
		kubeClient := kubernetes.NewForConfigOrDie(cfg)
		dynCli, err := dynamic.NewForConfig(cfg)
		if err != nil {
			panic(err)
		}
		wfcli := common.NewWFClient(cfg)
		if wfcli == nil {
			panic("workflow client could not be nil")
		}
		addoncli := common.NewAddonClient(cfg)
		ctx, cancel = context.WithCancel(context.Background())

		ns := "addon-manager-system"
		_, err = kubeClient.CoreV1().Namespaces().Create(ctx, &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		}, metav1.CreateOptions{})
		if err != nil {
			panic(err)
		}
		addonController = newResourceController(kubeClient, dynCli, addoncli, wfcli, "addon", ns)

		go func() {
			addonController.Run(ctx, stopMgr)
		}()
		Expect(addonController).ToNot(BeNil())
	})

	Describe("Addon CR can be reconciled", func() {
		var instance *v1alpha1.Addon

		var wfv1 = &unstructured.Unstructured{}
		wfv1.SetGroupVersionKind(schema.GroupVersionKind{
			Kind:    "Workflow",
			Group:   "argoproj.io",
			Version: "v1alpha1",
		})

		It("instance should be parsable", func() {
			addonYaml, err := ioutil.ReadFile("./tests/clusterautoscaler.yaml")
			Expect(err).ToNot(HaveOccurred())

			instance, err = parseAddonYaml(addonYaml)
			Expect(err).ToNot(HaveOccurred())
			Expect(instance).To(BeAssignableToTypeOf(&v1alpha1.Addon{}))
			Expect(instance.GetName()).To(Equal(addonName))
		})

		It("instance should be reconciled", func() {
			_, err := addonController.addoncli.AddonmgrV1alpha1().Addons(addonNamespace).Create(ctx, instance, metav1.CreateOptions{})
			if apierrors.IsInvalid(err) {
				Fail(fmt.Sprintf("failed to create object, got an invalid object error. %v", err))
			}
			Expect(err).NotTo(HaveOccurred())

			var fetched *addonmgrv1alpha1.Addon
			Eventually(func() error {
				fetched, err = addonController.addoncli.AddonmgrV1alpha1().Addons(addonNamespace).Get(ctx, instance.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if len(fetched.ObjectMeta.Finalizers) > 0 {
					return nil
				}
				return fmt.Errorf("addon is not valid")
			}, timeout).Should(Succeed())

			By("Verify addon has been reconciled by checking for checksum status")
			Expect(fetched.Status.Checksum).ShouldNot(BeEmpty())

			By("Verify addon has finalizers added which means it's valid")
			Expect(fetched.ObjectMeta.Finalizers).Should(Equal([]string{"delete.addonmgr.keikoproj.io"}))
		})
	})
})

func parseAddonYaml(data []byte) (*v1alpha1.Addon, error) {
	var err error
	o := &unstructured.Unstructured{}
	err = yaml.Unmarshal(data, &o.Object)
	if err != nil {
		return nil, err
	}
	a := &v1alpha1.Addon{}
	err = common.GetAddonMgrScheme().Convert(o, a, 0)
	if err != nil {
		return nil, err
	}

	return a, nil
}
