package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"time"

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
				log.Error(err, "failed to create object, got an invalid object error")
				return
			}
			Expect(err).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), instance)

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

			var wfv1 = &unstructured.Unstructured{}
			wfv1.SetGroupVersionKind(schema.GroupVersionKind{
				Kind:    "Workflow",
				Group:   "argoproj.io",
				Version: "v1alpha1",
			})
			wfName := instance.GetFormattedWorkflowName(v1alpha1.Prereqs)
			var wfv1Key = types.NamespacedName{Name: wfName, Namespace: "default"}
			k8sClient.Get(context.TODO(), wfv1Key, wfv1)
			By("Verify addon has workflows generated with new checksum name")
			Expect(wfv1.GetName()).Should(Equal(wfName))

			By("Verify deleting workflows triggers reconcile and doesn't regenerate workflows again")
			Expect(k8sClient.Delete(context.TODO(), wfv1)).To(Succeed())
			Expect(k8sClient.Get(context.TODO(), wfv1Key, wfv1)).ToNot(Succeed())
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
