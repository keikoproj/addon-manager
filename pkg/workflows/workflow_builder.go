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
	"fmt"
	"log"
	"strings"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const defaultPython3ScriptImage = "python:3"
const defaultSubmitContainerImage = "expert360/kubectl-awscli:v1.11.2"

var doDelete = false

// WorkflowBuilder interface for building an unstructured workflow
type WorkflowBuilder interface {
	Scripts(map[string]string) WorkflowBuilder
	Resources([]string) WorkflowBuilder
	Delete() WorkflowBuilder
	Build() unstructured.Unstructured
}

type workflowBuilder struct {
	defaultContent        map[string]interface{}
	defaultEntryTemplate  map[string]interface{}
	defaultSubmitStep     map[string]interface{}
	defaultSubmitTemplate map[string]interface{}
	deleteTemplates       []map[string]interface{}
	workflowName          string
	workflowNamespace     string
}

// New creates a reference to the workflowBuilder type
func New() WorkflowBuilder {
	// content := wf.UnstructuredContent()
	content := make(map[string]interface{})
	content["spec"] = make(map[string]interface{})
	content["spec"].(map[string]interface{})["entrypoint"] = "entry"
	content["spec"].(map[string]interface{})["serviceAccountName"] = "addon-manager-workflow-installer-sa"
	content["spec"].(map[string]interface{})["templates"] = make([]map[string]interface{}, 0)

	// default submit container
	dsc := make(map[string]interface{})
	dsc["image"] = defaultSubmitContainerImage
	dsc["command"] = []string{"sh", "-c"}
	dsc["args"] = []string{"kubectl apply -f /tmp/doc"}

	// default submit template
	submitTemplate := make(map[string]interface{})
	submitTemplate["name"] = "submit"
	submitTemplate["inputs"] = make(map[string]interface{})
	submitTemplate["inputs"].(map[string]interface{})["parameters"] = make([]map[string]interface{}, 0)
	submitTemplate["inputs"].(map[string]interface{})["artifacts"] = make([]map[string]interface{}, 0)
	submitTemplate["container"] = dsc

	// default entry template
	entryTemplate := make(map[string]interface{})
	entryTemplate["name"] = "entry"
	entryTemplate["steps"] = make([][]map[string]interface{}, 1)

	// default submit step
	submitStep := make(map[string]interface{})
	submitStep["name"] = "install"
	submitStep["arguments"] = make(map[string]interface{})
	submitStep["arguments"].(map[string]interface{})["parameters"] = make([]map[string]interface{}, 0)
	submitStep["arguments"].(map[string]interface{})["artifacts"] = make([]map[string]interface{}, 0)
	submitStep["template"] = "submit"

	// default delete templates
	deleteEntryTemplate := make(map[string]interface{})
	deleteEntryTemplate["name"] = "delete-wf"
	deleteEntryTemplate["steps"] = make([][]map[string]interface{}, 1)

	deleteStepsTemp := make(map[string]interface{})
	deleteStepsTemp["name"] = "delete-ns"
	deleteStepsTemp["template"] = "delete-ns"

	deleteEntryTemplate["steps"].([][]map[string]interface{})[0] = append(deleteEntryTemplate["steps"].([][]map[string]interface{})[0], deleteStepsTemp)

	deleteNSTemplate := make(map[string]interface{})
	deleteNSTemplate["name"] = "delete-ns"
	deleteNSTemplate["container"] = make(map[string]interface{})
	deleteNSTemplate["container"].(map[string]interface{})["image"] = defaultSubmitContainerImage
	deleteNSTemplate["container"].(map[string]interface{})["command"] = []string{"sh", "-c"}
	deleteNSTemplate["container"].(map[string]interface{})["args"] = []string{"kubectl delete all -n {{workflow.parameters.namespace}} --all"}

	deleteTemplates := []map[string]interface{}{deleteEntryTemplate, deleteNSTemplate}

	return &workflowBuilder{
		defaultContent:        content,
		defaultEntryTemplate:  entryTemplate,
		defaultSubmitStep:     submitStep,
		defaultSubmitTemplate: submitTemplate,
		deleteTemplates:       deleteTemplates,
	}
}

// Build returns the unstructured workflow object
func (wb *workflowBuilder) Build() unstructured.Unstructured {
	wf := unstructured.Unstructured{}

	// append the entry and submit templates to the existing templates list
	if !doDelete {
		wb.defaultContent["spec"].(map[string]interface{})["templates"] = append(wb.defaultContent["spec"].(map[string]interface{})["templates"].([]map[string]interface{}), wb.defaultEntryTemplate, wb.defaultSubmitTemplate)
	} else {
		wb.defaultContent["spec"].(map[string]interface{})["templates"] = append(wb.defaultContent["spec"].(map[string]interface{})["templates"].([]map[string]interface{}), wb.deleteTemplates[0], wb.deleteTemplates[1])
	}

	// set the contents in the unstructured workflow object
	wf.SetUnstructuredContent(wb.defaultContent)
	wf.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "Workflow",
		Group:   "argoproj.io",
		Version: "v1alpha1",
	})
	wf.SetGenerateName("-") // for consistancy with current workflows, although i believe these will not be necessary

	return wf
}

// ConvertUnstructuredWorkflowToString converts the unstructured content into a string
func ConvertUnstructuredWorkflowToString(u unstructured.Unstructured) string {
	wfYAMLBytes, err := yaml.Marshal(u.UnstructuredContent())
	if err != nil {
		log.Fatal(err)
	}
	return string(wfYAMLBytes)
}

func (wb *workflowBuilder) Delete() WorkflowBuilder {
	doDelete = true
	return wb
}

// func (wb *workflowBuilder) Resources(resources []map[string]interface{}) WorkflowBuilder {
func (wb *workflowBuilder) Resources(resources []string) WorkflowBuilder {
	resourceArtifactData := ""
	for _, resource := range resources {
		var separator = "---\n"
		if !strings.HasSuffix(resource, "\n") {
			separator = "\n" + separator
		}
		resourceArtifactData += resource + separator
	}

	resourceArtifact := make(map[string]interface{})
	resourceArtifact["name"] = "doc"
	resourceArtifact["path"] = "/tmp/doc"
	resourceArtifact["raw"] = make(map[string]interface{})
	resourceArtifact["raw"].(map[string]interface{})["data"] = resourceArtifactData

	// append the accumulated resource artifact to the submit template inputs
	wb.defaultSubmitTemplate["inputs"].(map[string]interface{})["artifacts"] = append(wb.defaultSubmitTemplate["inputs"].(map[string]interface{})["artifacts"].([]map[string]interface{}), resourceArtifact)
	wb.defaultEntryTemplate["steps"].([][]map[string]interface{})[0] = append(wb.defaultEntryTemplate["steps"].([][]map[string]interface{})[0], wb.defaultSubmitStep)

	return wb
}

func (wb *workflowBuilder) Scripts(scripts map[string]string) WorkflowBuilder {
	for name, scriptData := range scripts {
		tempName := strings.TrimSuffix(name, ".py")
		tempScript := make(map[string]interface{})

		// TODO verify that the name is ok
		tempScript["name"] = tempName
		tempScript["script"] = make(map[string]interface{})
		tempScript["script"].(map[string]interface{})["image"] = defaultPython3ScriptImage
		tempScript["script"].(map[string]interface{})["command"] = []string{"pyhton"}
		tempScript["script"].(map[string]interface{})["source"] = scriptData

		tempStep := make(map[string]interface{})
		tempStep["name"] = tempName
		tempStep["template"] = tempName

		// add script template to templates
		wb.defaultContent["spec"].(map[string]interface{})["templates"] = append(wb.defaultContent["spec"].(map[string]interface{})["templates"].([]interface{}), tempScript)
		// add step to entry template steps
		wb.defaultEntryTemplate["steps"].([][]map[string]interface{})[0] = append(wb.defaultEntryTemplate["steps"].([][]map[string]interface{})[0], tempStep)

		// parameterizing the value as an output result for argo
		paramValue := fmt.Sprintf("{{steps.%s.outputs.result}}", tempName)

		scriptParamArgument := make(map[string]interface{})
		scriptParamArgument["name"] = tempName
		scriptParamArgument["value"] = paramValue

		scriptParamInput := make(map[string]interface{})
		scriptParamInput["name"] = tempName

		// create arguments.parameters entry in submit step
		wb.defaultSubmitStep["arguments"].(map[string]interface{})["parameters"] = append(wb.defaultSubmitStep["arguments"].(map[string]interface{})["parameters"].([]map[string]interface{}), scriptParamArgument)
		// create input.parameters entry in submit template
		wb.defaultSubmitTemplate["inputs"].(map[string]interface{})["parameters"] = append(wb.defaultSubmitTemplate["inputs"].(map[string]interface{})["parameters"].([]map[string]interface{}), scriptParamInput)
	}
	return wb
}
