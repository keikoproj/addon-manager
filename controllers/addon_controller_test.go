package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/keikoproj/addon-manager/api/v1alpha1"
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
			addonYaml, err := ioutil.ReadFile("../docs/examples/clusterautoscaler.yaml")
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
			//defer k8sClient.Delete(context.TODO(), instance)

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

		It("instance with dependencies should succeed", func() {
			var instance2 = &v1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: addonNamespace},
				Spec: v1alpha1.AddonSpec{
					PackageSpec: v1alpha1.PackageSpec{
						PkgType:    v1alpha1.CompositePkg,
						PkgName:    "test/addon-1",
						PkgVersion: "1.0.0",
						PkgDeps: map[string]string{
							"cluster-autoscaler": "*",
						},
					},
					Params: v1alpha1.AddonParams{
						Namespace: "addon-test-ns",
					},
				},
			}

			By("Verify first addon-1 exists and is valid")
			patch := client.MergeFrom(instance.DeepCopy())
			instance.Status.Lifecycle.Installed = v1alpha1.Succeeded
			instance.Status.Lifecycle.Prereqs = v1alpha1.Succeeded
			Expect(k8sClient.Status().Patch(context.TODO(), instance, patch)).ToNot(HaveOccurred())
			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
					return err
				}

				if instance.Status.Lifecycle.Installed == v1alpha1.Succeeded {
					return nil
				}

				return fmt.Errorf("addon-1 is not installed")
			}, timeout).Should(Succeed())

			By("Verify addon-2 that depends on addon-1 works as expected")
			err := k8sClient.Create(context.TODO(), instance2)
			if apierrors.IsInvalid(err) {
				Fail(fmt.Sprintf("failed to create object, got an invalid object error. %v", err))
			}
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), types.NamespacedName{Name: instance2.Name, Namespace: instance2.Namespace}, instance2); err != nil {
					return err
				}

				if instance2.Status.Lifecycle.Installed == v1alpha1.Succeeded {
					return nil
				}

				//fmt.Printf("Addon phase: %s", instance2.Status.Lifecycle.Installed)

				return fmt.Errorf("addon-2 is not valid")
			}, timeout*2).Should(Succeed())

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
	err = scheme.Scheme.Convert(o, a, 0)
	if err != nil {
		return nil, err
	}

	return a, nil
}
