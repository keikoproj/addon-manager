/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"testing"

	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"
	"github.com/keikoproj/addon-manager/test-bdd/testutil"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	apiextcs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var ctx = context.TODO()

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Addon Type Suite", []Reporter{junitReporter})
}

var _ = Describe("Addon Mgr should install CRD and Addon correctly", func() {
	cfg := config.GetConfigOrDie()

	extClient := apiextcs.NewForConfigOrDie(cfg)
	dynClient := dynamic.NewForConfigOrDie(cfg)
	kubeClient := kubernetes.NewForConfigOrDie(cfg)

	var addonName string
	var addonNamespace string
	var relativeAddonPath = "../docs/examples/eventrouter.yaml"
	var addonGroupSchema = common.AddonGVR()
	var workflowGroupSchema = common.WorkflowGVR()

	var commonLabels = func(ad *unstructured.Unstructured) map[string]string {
		spec, _, _ := unstructured.NestedMap(ad.UnstructuredContent(), "spec")
		metadata, _, _ := unstructured.NestedMap(ad.UnstructuredContent(), "metadata")
		return map[string]string{
			"app.kubernetes.io/name":       spec["pkgName"].(string),
			"app.kubernetes.io/version":    spec["pkgVersion"].(string),
			"app.kubernetes.io/part-of":    metadata["name"].(string),
			"app.kubernetes.io/managed-by": "addonmgr.keikoproj.io",
		}
	}

	// Setup CRD
	It("should create CRD", func() {
		crdsRoot := "../config/crd/bases"
		files, err := ioutil.ReadDir(crdsRoot)
		if err != nil {
			Fail(fmt.Sprintf("failed to read crd path. %v", err))
		}

		for _, file := range files {
			err = testutil.CreateCRD(extClient, fmt.Sprintf("%s/%s", crdsRoot, file.Name()))
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("should create an Addon object", func() {
		addon, err := testutil.CreateAddon(dynClient, relativeAddonPath, "")
		Expect(err).NotTo(HaveOccurred())

		addonName = addon.GetName()
		addonNamespace = addon.GetNamespace()
		Eventually(func() map[string]interface{} {
			a, _ := dynClient.Resource(addonGroupSchema).Namespace(addonNamespace).Get(ctx, addonName, metav1.GetOptions{})
			return a.UnstructuredContent()
		}, 20).Should(HaveKey("status"))
	})

	It("addon workflows should succeed and addon lifecycle should be updated", func() {
		Eventually(func() addonmgrv1alpha1.ApplicationAssemblyPhase {
			addonObject, err := dynClient.Resource(addonGroupSchema).Namespace(addonNamespace).Get(ctx, addonName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			statusMap := addonObject.UnstructuredContent()["status"].(map[string]interface{})
			lifecycleMap := statusMap["lifecycle"].(map[string]interface{})

			return addonmgrv1alpha1.ApplicationAssemblyPhase(lifecycleMap["prereqs"].(string))
		}, 30).Should(Equal(addonmgrv1alpha1.Succeeded))

		Eventually(func() addonmgrv1alpha1.ApplicationAssemblyPhase {
			addonObject, err := dynClient.Resource(addonGroupSchema).Namespace(addonNamespace).Get(ctx, addonName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			statusMap := addonObject.UnstructuredContent()["status"].(map[string]interface{})
			lifecycleMap := statusMap["lifecycle"].(map[string]interface{})

			return addonmgrv1alpha1.ApplicationAssemblyPhase(lifecycleMap["installed"].(string))
		}, 30).Should(Equal(addonmgrv1alpha1.Succeeded))
	})

	It("should copy addon.spec.params to workflow.spec.arguments.parameters", func() {
		for _, workflowLifecycleStep := range []string{"prereqs", "install"} {
			addonObject, err := dynClient.Resource(addonGroupSchema).Namespace(addonNamespace).Get(ctx, addonName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			addonNamespaceParam, found, _ := unstructured.NestedString(addonObject.UnstructuredContent(), "spec", "params", "namespace")
			Expect(found).To(BeTrue())
			addonContextParams, found, _ := unstructured.NestedMap(addonObject.UnstructuredContent(), "spec", "params", "context")
			Expect(found).To(BeTrue())
			dataParams, found, _ := unstructured.NestedMap(addonObject.UnstructuredContent(), "spec", "params", "data")
			Expect(found).To(BeFalse())
			addonChecksum, found, _ := unstructured.NestedString(addonObject.UnstructuredContent(), "status", "checksum")
			Expect(found).To(BeTrue())

			workflowTemplatePrefixName, found, _ := unstructured.NestedString(addonObject.UnstructuredContent(), "spec", "lifecycle", workflowLifecycleStep, "namePrefix")
			prefix := addonName
			if found {
				prefix = fmt.Sprintf("%s-%s", prefix, workflowTemplatePrefixName)
			}
			workflowIdentifierName := fmt.Sprintf("%s-%s-%s-wf", prefix, workflowLifecycleStep, addonChecksum)

			workflow, err := dynClient.Resource(workflowGroupSchema).Namespace(addonNamespace).Get(ctx, workflowIdentifierName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			workflowParameters, found, _ := unstructured.NestedSlice(workflow.UnstructuredContent(), "spec", "arguments", "parameters")
			Expect(found).To(BeTrue())

			wfParamsMap := make(map[string]interface{})
			for _, param := range workflowParameters {
				param := param.(map[string]interface{})
				paramName := param["name"]
				paramValue := param["value"]
				wfParamsMap[paramName.(string)] = paramValue
			}

			addonNS := "addon-event-router-ns"

			Expect(wfParamsMap["namespace"]).To(Equal(addonNS))

			for name, val := range addonContextParams {
				Expect(wfParamsMap[name]).To(Equal(val))
			}

			for name, val := range dataParams {
				Expect(wfParamsMap[name]).To(Equal(val))
			}

			Expect(addonNamespaceParam).To(Equal(addonNS))
		}
	})

	It("should label the deployment with the default labels", func() {
		addonObject, err := dynClient.Resource(addonGroupSchema).Namespace(addonNamespace).Get(ctx, addonName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		deploymentNamespace, found, _ := unstructured.NestedString(addonObject.UnstructuredContent(), "spec", "params", "namespace")
		Expect(found).To(BeTrue())
		deployment, err := kubeClient.AppsV1().Deployments(deploymentNamespace).Get(ctx, addonName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		labels := deployment.ObjectMeta.Labels
		for name, val := range commonLabels(addonObject) {
			Expect(labels[name]).To(Equal(val))
		}
	})

	It("should list argo as an addon with watched resources", func() {
		argoAddonName := "addon-manager-argo-addon"
		addonObject, err := dynClient.Resource(addonGroupSchema).Namespace(addonNamespace).Get(ctx, argoAddonName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		objs, found, err := unstructured.NestedSlice(addonObject.UnstructuredContent(), "status", "resources")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).Should(BeTrue())
		Expect(len(objs)).ShouldNot(BeZero())
	})

	It("duplicate addons are not allowed", func() {
		addon, err := testutil.CreateAddon(dynClient, relativeAddonPath, "-2")
		Expect(err).NotTo(HaveOccurred())

		addonName = addon.GetName()
		addonNamespace = addon.GetNamespace()
		Eventually(func() map[string]interface{} {
			a, _ := dynClient.Resource(addonGroupSchema).Namespace(addonNamespace).Get(ctx, addonName, metav1.GetOptions{})
			return a.UnstructuredContent()
		}, 20).Should(HaveKey("status"))

		addonObject, err := dynClient.Resource(addonGroupSchema).Namespace(addonNamespace).Get(ctx, addonName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		status, found, err := unstructured.NestedString(addonObject.UnstructuredContent(), "status", "lifecycle", "installed")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(status).To(Equal("Failed"))
	})
})
