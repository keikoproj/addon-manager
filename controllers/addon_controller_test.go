package controllers

import (
	"fmt"
	"io/ioutil"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	addonNamespace  = "default"
	addonName       = "cluster-autoscaler"
	addonKey        = types.NamespacedName{Name: addonName, Namespace: addonNamespace}
	addonController = &Controller{}
	stopMgr         chan struct{}
)

const timeout = time.Second * 5

var _ = Describe("AddonController", func() {

	Describe("Addon CR can be reconciled", func() {
		var instance *v1alpha1.Addon

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
