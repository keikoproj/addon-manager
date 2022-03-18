package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/scheme"
	"github.com/keikoproj/addon-manager/pkg/common"
)

var (
	addonNamespace = "default"
	addonName      = "cluster-autoscaler"
	addonKey       = types.NamespacedName{Name: addonName, Namespace: addonNamespace}
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
			MetricsBindAddress: "0",
			LeaderElection:     false,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(mgr).ToNot(BeNil())

		stopMgr = make(chan struct{})
		wg := &sync.WaitGroup{}
		//wg.Add(1)
		StartTestManager(mgr, wg)
		//wg.Wait()
		StartController(mgr, stopMgr, wg)

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
			instance.SetNamespace(addonNamespace)
			err := k8sClient.Create(context.TODO(), instance)
			// created, err := addonController.addoncli.AddonmgrV1alpha1().Addons(addonNamespace).Create(ctx, instance, metav1.CreateOptions{})
			// if apierrors.IsInvalid(err) {
			// 	Fail(fmt.Sprintf("failed to create object, got an invalid object error. %v", err))
			// }
			Expect(err).NotTo(HaveOccurred())
			// By("Verify addon has been reconciled by checking for checksum status")
			// Expect(instance.Status.Checksum).ShouldNot(BeEmpty())

			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
					return err
				}

				if len(instance.ObjectMeta.Finalizers) > 0 {
					return nil
				}
				return fmt.Errorf("addon is not valid")
			}, timeout).Should(Succeed())

			// By("Verify addon has been reconciled by checking for checksum status")
			// Expect(instance.Status.Checksum).ShouldNot(BeEmpty())

			// By("Verify addon has finalizers added which means it's valid")
			// Expect(instance.ObjectMeta.Finalizers).Should(Equal([]string{"delete.addonmgr.keikoproj.io"}))

			// oldCheckSum := instance.Status.Checksum

			// //Update instance params for checksum validation
			// instance.Spec.Params.Context.ClusterRegion = "us-east-2"
			// err = k8sClient.Update(context.TODO(), instance)

			// // This sleep is introduced as addon status is updated after multiple requeues - Ideally it should be 2 sec.
			// time.Sleep(5 * time.Second)

			// if apierrors.IsInvalid(err) {
			// 	log.Error(err, "failed to update object, got an invalid object error")
			// 	return
			// }
			// Expect(err).NotTo(HaveOccurred())
			// Eventually(func() error {
			// 	if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
			// 		return err
			// 	}

			// 	if len(instance.ObjectMeta.Finalizers) > 0 {
			// 		return nil
			// 	}
			// 	return fmt.Errorf("addon is not valid")
			// }, timeout).Should(Succeed())

			// By("Verify changing addon spec generates new checksum")
			// Expect(instance.Status.Checksum).ShouldNot(BeIdenticalTo(oldCheckSum))

			// By("Verify addon has workflows generated with new checksum name")
			// wfName := instance.GetFormattedWorkflowName(v1alpha1.Prereqs)
			// var wfv1Key = types.NamespacedName{Name: wfName, Namespace: "default"}
			// Eventually(func() error {
			// 	return k8sClient.Get(context.TODO(), wfv1Key, wfv1)
			// }, timeout).Should(Succeed())
			// Expect(wfv1.GetName()).Should(Equal(wfName))

			// By("Verify deleting workflows triggers reconcile and doesn't regenerate workflows again")
			// Expect(k8sClient.Delete(context.TODO(), wfv1)).To(Succeed())
			// Expect(k8sClient.Get(context.TODO(), wfv1Key, wfv1)).ToNot(Succeed())
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
