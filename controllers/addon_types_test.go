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

package controllers

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	addonv1 "github.com/keikoproj/addon-manager/apis/addon/v1alpha1"
)

var wfSpecTemplate = `
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  generateName: scripts-python-
spec:
  entrypoint: python-script-example
  templates:
    - name: python-script-example
      steps:
        - - name: generate
            template: gen-random-int
        - - name: print
            template: print-message
            arguments:
              parameters:
                - name: message
                  value: "{{steps.generate.outputs.result}}"

    - name: gen-random-int
      script:
        image: python:alpine3.6
        command: [python]
        source: |
          import random
          i = random.randint(1, 100)
          print(i)
    - name: print-message
      inputs:
        parameters:
          - name: message
      container:
        image: alpine:latest
        command: [sh, -c]
        args: ["echo result was: {{inputs.parameters.message}}"]
`

// These tests are written in BDD-style using Ginkgo framework. Refer to
// http://onsi.github.io/ginkgo to learn more.

var _ = Describe("Addon", func() {
	var (
		key              types.NamespacedName
		created, fetched *addonv1.Addon
	)

	BeforeEach(func() {
		// Add any setup steps that needs to be executed before each test
	})

	AfterEach(func() {
		// Add any teardown steps that needs to be executed after each test
	})

	// Add Tests for OpenAPI validation (or additional CRD features) specified in
	// your API definition.
	// Avoid adding tests for vanilla CRUD operations because they would
	// test Kubernetes API server, which isn't the goal here.
	Context("Create API", func() {

		It("should create an object successfully", func() {

			key = types.NamespacedName{
				Name:      "foo",
				Namespace: "default",
			}
			created = &addonv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: addonv1.AddonSpec{
					PackageSpec: addonv1.PackageSpec{
						PkgName:        "my-addon",
						PkgVersion:     "1.0.0",
						PkgType:        addonv1.HelmPkg,
						PkgDescription: "",
						PkgDeps:        map[string]string{"core/A": "*", "core/B": "v1.0.0"},
					},
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "my-app",
						},
					},
					Params: addonv1.AddonParams{
						Namespace: "foo-ns",
						Context: addonv1.ClusterContext{
							ClusterName:   "foo-cluster",
							ClusterRegion: "foo-region",
							AdditionalConfigs: map[string]addonv1.FlexString{
								"additional": "config",
							},
						},
						Data: map[string]addonv1.FlexString{
							"foo-param": "val",
						},
					},
					Lifecycle: addonv1.LifecycleWorkflowSpec{
						Prereqs: addonv1.WorkflowType{
							NamePrefix: "my-prereqs",
							Template:   wfSpecTemplate,
						},
						Install: addonv1.WorkflowType{
							Template: wfSpecTemplate,
						},
						Delete: addonv1.WorkflowType{
							Template: wfSpecTemplate,
						},
					},
				},
			}

			By("creating an API obj")
			Expect(k8sClient.Create(context.TODO(), created)).To(Succeed())

			fetched = &addonv1.Addon{}
			Expect(k8sClient.Get(context.TODO(), key, fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			By("Checking expected fetched values")
			pkgSpec := fetched.GetPackageSpec()
			Expect(pkgSpec).To(Equal(fetched.Spec.PackageSpec))

			addonParams := fetched.GetAllAddonParameters()
			paramsMap := map[string]string{
				"namespace":     "foo-ns",
				"clusterName":   "foo-cluster",
				"clusterRegion": "foo-region",
				"additional":    "config",
				"foo-param":     "val",
			}

			Expect(addonParams).To(HaveLen(len(paramsMap)))
			for name := range paramsMap {
				Expect(addonParams[name]).To(Equal(paramsMap[name]))
			}

			checksum := fetched.CalculateChecksum()
			Expect(checksum).To(Equal("4a77025d"))

			// Update status checksum
			fetched.Status.Checksum = checksum

			wfName := fetched.GetFormattedWorkflowName(addonv1.Install)
			Expect(wfName).To(Equal(fmt.Sprintf("foo-install-%s-wf", checksum)))

			By("updating labels")
			updated := fetched.DeepCopy()
			updated.Labels = map[string]string{"hello": "world"}
			Expect(k8sClient.Update(context.TODO(), updated)).To(Succeed())

			Expect(k8sClient.Get(context.TODO(), key, fetched)).To(Succeed())
			Expect(fetched).To(Equal(updated))

			By("deleting the created object")
			Expect(k8sClient.Delete(context.TODO(), fetched)).To(Succeed())
			Expect(k8sClient.Get(context.TODO(), key, fetched)).ToNot(Succeed())
		})

	})

})
