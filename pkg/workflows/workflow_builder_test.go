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

package workflows

import (
	"testing"
	"time"

	"github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var c client.Client

const timeout = time.Second * 5

// Verify the default workflow after calling Build()
func TestWorkflowBuilder(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	builder := New()
	wf := builder.Build()

	g.Expect(wf.GetAPIVersion()).To(gomega.Equal("argoproj.io/v1alpha1"))
	g.Expect(wf.GetKind()).To(gomega.Equal("Workflow"))
	g.Expect(wf.GetGenerateName()).To(gomega.Equal("-"))

	spec := wf.UnstructuredContent()["spec"].(map[string]interface{})
	g.Expect(spec).NotTo(gomega.BeNil())
	g.Expect(spec["entrypoint"]).To(gomega.Equal("entry"))
	g.Expect(spec["serviceAccountName"]).To(gomega.Equal("addon-manager-workflow-installer-sa"))

	templates := spec["templates"].([]map[string]interface{})
	g.Expect(templates).To(gomega.HaveLen(2))

	g.Expect(templates[0]).To(gomega.HaveKeyWithValue("name", "entry"))
	g.Expect(templates[1]).To(gomega.HaveKeyWithValue("name", "submit"))

	submitTemplateContainer := templates[1]["container"].(map[string]interface{})
	g.Expect(submitTemplateContainer["args"]).To(gomega.Equal([]string{"kubectl apply -f /tmp/doc"}))
	g.Expect(submitTemplateContainer["command"]).To(gomega.Equal([]string{"sh", "-c"}))
	g.Expect(submitTemplateContainer["image"]).To(gomega.Equal(DefaultSubmitContainerImage))

	submitTemplateInputs := templates[1]["inputs"].(map[string]interface{})
	g.Expect(submitTemplateInputs["parameters"]).To(gomega.Equal(make([]map[string]interface{}, 0)))
	g.Expect(submitTemplateInputs["artifacts"]).To(gomega.Equal(make([]map[string]interface{}, 0)))
}

func TestDeleteWorkflowBuilder(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	builder := New()
	wf := builder.Delete().Build()

	g.Expect(wf.GetAPIVersion()).To(gomega.Equal("argoproj.io/v1alpha1"))
	g.Expect(wf.GetKind()).To(gomega.Equal("Workflow"))
	g.Expect(wf.GetGenerateName()).To(gomega.Equal("-"))

	spec := wf.UnstructuredContent()["spec"].(map[string]interface{})
	g.Expect(spec).NotTo(gomega.BeNil())
	g.Expect(spec["entrypoint"]).To(gomega.Equal("entry"))
	g.Expect(spec["serviceAccountName"]).To(gomega.Equal("addon-manager-workflow-installer-sa"))

	templates := spec["templates"].([]map[string]interface{})
	g.Expect(templates).To(gomega.HaveLen(2))
	g.Expect(templates[0]).To(gomega.HaveKeyWithValue("name", "delete-wf"))
	g.Expect(templates[1]).To(gomega.HaveKeyWithValue("name", "delete-ns"))

	deleteWfTemplateSteps := templates[0]["steps"].([][]map[string]interface{})
	g.Expect(deleteWfTemplateSteps[0][0]["name"]).To(gomega.Equal("delete-ns"))
	g.Expect(deleteWfTemplateSteps[0][0]["template"]).To(gomega.Equal("delete-ns"))

	deleteNSContainer := templates[1]["container"].(map[string]interface{})
	g.Expect(deleteNSContainer["args"]).To(gomega.Equal([]string{"kubectl delete all -n {{workflow.parameters.namespace}} --all"}))
	g.Expect(deleteNSContainer["command"]).To(gomega.Equal([]string{"sh", "-c"}))
	g.Expect(deleteNSContainer["image"]).To(gomega.Equal(DefaultSubmitContainerImage))
}
