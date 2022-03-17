package controllers

// controllers functional tests
import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	"io/ioutil"
	"testing"

	"github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	addonv1 "github.com/keikoproj/addon-manager/api/addon"

	addonapiv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
)

var (
	testNamespace = "default"
	testAddon     = "cluster-autoscaler"
)

func configureCRD(dyCli dynamic.Interface) error {
	var addonCRD = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1beta1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"name": "addons.addonmgr.keikoproj.io",
				"spec": map[string]interface{}{
					"group": addonv1.Group,
					"names": map[string]interface{}{
						"kind":   addonv1.AddonKind,
						"plural": addonv1.AddonPlural,
					},
					"scope":   "Namespaced",
					"version": "v1alpha1",
				},
			},
		}}

	CRDSchema := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1beta1", Resource: "customresourcedefinitions"}
	_, err := dyCli.Resource(CRDSchema).Create(context.Background(), addonCRD, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("failed creating Addon CRDs.")
		return err
	}
	return nil
}

func TestAddonInstall(t *testing.T) {
	t.Skip()
	RegisterFailHandler(Fail)
	logger := logrus.WithField("test-controllers", "addon")
	ctx := context.TODO()

	addonYaml, err := ioutil.ReadFile("./tests/clusterautoscaler.yaml")
	Expect(err).To(BeNil())

	instance, err := parseAddonYaml(addonYaml)
	Expect(err).ToNot(HaveOccurred())
	instance.SetName(testAddon)
	instance.SetNamespace(testNamespace)
	Expect(instance).To(BeAssignableToTypeOf(&addonapiv1.Addon{}))

	// install addon
	logger.Info("installing addon")
	addonController := newController(instance)

	logger.Info("prcoessing addon")
	processed := addonController.processNextItem(ctx)
	Expect(processed).To(BeTrue())

	// verify addon checksum
	logger.Info("fetching Addon")
	fetchedAddon, err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, testAddon, metav1.GetOptions{})
	Expect(err).To(BeNil())
	Expect(fetchedAddon).NotTo(BeNil())
	logger.Info("verifying Addon finalizer and checksum")
	// verify finalizer is set
	Expect(fetchedAddon.ObjectMeta.Finalizers).NotTo(BeZero())
	// verify checksum
	Expect(fetchedAddon.Status.Checksum).ShouldNot(BeEmpty())

	logger.Info("verifying Addon checksum changes.")
	oldCheckSum := fetchedAddon.Status.Checksum
	//Update instance params for checksum validation
	fetchedAddon.Spec.Params.Context.ClusterRegion = "us-east-2"
	err = addonController.handleAddonUpdate(ctx, fetchedAddon)
	fmt.Printf("\n err %#v\n", err)
	Expect(err).To(BeNil())

	updated, err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, testAddon, metav1.GetOptions{})
	Expect(err).To(BeNil())
	Expect(updated).NotTo(BeNil())
	Expect(updated.Status.Checksum).ShouldNot(BeIdenticalTo(oldCheckSum))

	logger.Info("verifying workflow generated.")
	wfName := updated.GetFormattedWorkflowName(addonapiv1.Prereqs)
	fetchedwf, err := addonController.wfcli.ArgoprojV1alpha1().Workflows(testNamespace).Get(ctx, wfName, metav1.GetOptions{})
	Expect(err).To(BeNil())
	Expect(fetchedwf.GetName()).Should(Equal(wfName))

	logger.Info("verifying workflow deleted.")
	err = addonController.wfcli.ArgoprojV1alpha1().Workflows(testNamespace).Delete(ctx, wfName, metav1.DeleteOptions{})
	Expect(err).To(BeNil())
	wf, err := addonController.wfcli.ArgoprojV1alpha1().Workflows(testNamespace).Get(ctx, wfName, metav1.GetOptions{})
	Expect(err).NotTo(BeNil())
	Expect(wf).To(BeNil())
}

func parseAddonYaml(data []byte) (*addonapiv1.Addon, error) {
	var err error
	o := &unstructured.Unstructured{}
	err = yaml.Unmarshal(data, &o.Object)
	if err != nil {
		return nil, err
	}
	a := &addonapiv1.Addon{}
	err = scheme.Scheme.Convert(o, a, 0)
	if err != nil {
		return nil, err
	}

	return a, nil
}
