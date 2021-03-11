package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
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
		})

		//modify instance
		wt := &instance.Spec.Lifecycle.Prereqs

		wp, err := parseAddonWorkflow(wt)
		if err != nil {
			log.Error(err,"Error while parsing addon worflow")
		}
		log.Info("workflow", wp)

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

func parseAddonWorkflow(wt *v1alpha1.WorkflowType) (*unstructured.Unstructured,error){

	wf := &unstructured.Unstructured{}

	var data map[string]interface{}

	// Load workflow spec into data obj
	if err := yaml.Unmarshal([]byte(wt.Template), &data); err != nil {
		return nil,fmt.Errorf("invalid workflow yaml spec passed. %v", err)
	}

	// We need to marshal and unmarshal due to conversion issues.
	raw, err := json.Marshal(data)
	if err != nil {
		return nil,errors.New("invalid workflow, unable to marshal data")
	}
	err = wf.UnmarshalJSON(raw)
	if err != nil {
		return nil, errors.New("invalid workflow, unable to unmarshal to workflow")
	}

	return wf,nil

}

func updateTTLObject(wf *unstructured.Unstructured) error {
	var ttl, _ = time.ParseDuration("1m")
	err := unstructured.SetNestedField(wf.Object, int64(ttl.Seconds()), "spec", "ttlSecondsAfterFinished")
	if err != nil {
		return err
	}

	return nil
}
