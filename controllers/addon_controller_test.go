package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"
)

var (
	addonNamespace = "default"
	addonName      = "cluster-autoscaler"
	addonKey       = types.NamespacedName{Name: addonName, Namespace: addonNamespace}
)

const timeout = time.Second * 5

var _ = Describe("AddonController", func() {

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
			if apierrors.IsInvalid(err) {
				Fail(fmt.Sprintf("failed to create object, got an invalid object error. %v", err))
			}
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
					return err
				}

				if len(instance.ObjectMeta.Finalizers) > 0 {
					return nil
				}
				return fmt.Errorf("addon is not valid")
			}, timeout).Should(Succeed())

			By("Verify addon has been reconciled by checking for checksum status")
			Expect(instance.Status.Checksum).ShouldNot(BeEmpty())

			By("Verify addon has finalizers added which means it's valid")
			Expect(instance.ObjectMeta.Finalizers).Should(Equal([]string{"delete.addonmgr.keikoproj.io"}))

			oldCheckSum := instance.Status.Checksum

			//Update instance params for checksum validation
			instance.Spec.Params.Context.ClusterRegion = "us-east-2"
			err = k8sClient.Update(context.TODO(), instance)

			// This sleep is introduced as addon status is updated after multiple requeues - Ideally it should be 2 sec.
			time.Sleep(5 * time.Second)

			if apierrors.IsInvalid(err) {
				log.Error(err, "failed to update object, got an invalid object error")
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
					return err
				}

				if len(instance.ObjectMeta.Finalizers) > 0 {
					return nil
				}
				return fmt.Errorf("addon is not valid")
			}, timeout).Should(Succeed())

			By("Verify changing addon spec generates new checksum")
			Expect(instance.Status.Checksum).ShouldNot(BeIdenticalTo(oldCheckSum))

			By("Verify addon has workflows generated with new checksum name")
			wfName := instance.GetFormattedWorkflowName(v1alpha1.Prereqs)
			var wfv1Key = types.NamespacedName{Name: wfName, Namespace: "default"}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), wfv1Key, wfv1)
			}, timeout).Should(Succeed())
			Expect(wfv1.GetName()).Should(Equal(wfName))

			By("Verify deleting workflows triggers reconcile and doesn't regenerate workflows again")
			Expect(k8sClient.Delete(context.TODO(), wfv1)).To(Succeed())
			Expect(k8sClient.Get(context.TODO(), wfv1Key, wfv1)).ToNot(Succeed())
		})

		It("instance should be deleted w/ deleting state", func() {
			By("Verify deleting instance should set Deleting state")
			Expect(k8sClient.Delete(context.TODO(), instance)).NotTo(HaveOccurred())
			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
					return err
				}

				if instance.ObjectMeta.DeletionTimestamp != nil && instance.Status.Lifecycle.Installed == v1alpha1.Deleting {
					return nil
				}
				return fmt.Errorf("addon is not being deleted")
			}, timeout).Should(Succeed())

			By("Verify delete workflow was generated")
			wfName := instance.GetFormattedWorkflowName(v1alpha1.Delete)
			var wfv1Key = types.NamespacedName{Name: wfName, Namespace: "default"}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), wfv1Key, wfv1)
			}, timeout).Should(Succeed())
			Expect(wfv1.GetName()).Should(Equal(wfName))
		})

		It("instance with dependencies should succeed", func() {
			instance = &v1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: "addon-1", Namespace: addonNamespace},
				Spec: v1alpha1.AddonSpec{
					PackageSpec: v1alpha1.PackageSpec{
						PkgType:    v1alpha1.CompositePkg,
						PkgName:    "test/addon-1",
						PkgVersion: "1.0.1",
					},
					Params: v1alpha1.AddonParams{
						Namespace: "addon-test-ns",
					},
				},
			}
			var instanceKey = types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}
			var instance2 = &v1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: "addon-2", Namespace: addonNamespace},
				Spec: v1alpha1.AddonSpec{
					PackageSpec: v1alpha1.PackageSpec{
						PkgType:    v1alpha1.CompositePkg,
						PkgName:    "test/addon-2",
						PkgVersion: "1.0.0",
						PkgDeps: map[string]string{
							"test/addon-1": "*",
						},
					},
					Params: v1alpha1.AddonParams{
						Namespace: "addon-test-ns",
					},
				},
			}
			var instanceKey2 = types.NamespacedName{Namespace: instance2.Namespace, Name: instance2.Name}

			By("Verify first addon-2 that depends on addon-1 is created and has validation failed state")
			Expect(k8sClient.Create(context.TODO(), instance2)).NotTo(HaveOccurred())
			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), instanceKey2, instance2); err != nil {
					return err
				}

				if instance2.Status.Lifecycle.Installed == v1alpha1.DepPending {
					return nil
				}

				return fmt.Errorf("addon-2 is not in validation failed state")
			}, timeout).Should(Succeed())

			By("Verify addon-1 is submitted and completes successfully")
			Expect(k8sClient.Create(context.TODO(), instance)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), instance)
			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), instanceKey, instance); err != nil {
					return err
				}

				if instance.Status.Lifecycle.Installed == v1alpha1.Succeeded {
					return nil
				}

				return fmt.Errorf("addon-1 is not installed")
			}, timeout).Should(Succeed())

			By("Verify addon-2 succeeds after addon-1 completed")
			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), instanceKey2, instance2); err != nil {
					return err
				}

				if instance2.Status.Lifecycle.Installed == v1alpha1.Succeeded {
					return nil
				}

				return fmt.Errorf("addon-2 is not valid")
			}, timeout*10).Should(Succeed())
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
