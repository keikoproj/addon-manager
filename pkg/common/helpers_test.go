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
package common

import (
	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"github.com/onsi/gomega"
	"reflect"
	"testing"

	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestContainsString(t *testing.T) {
	a := []string{"this", "is", "a", "test", "slice"}
	got := ContainsString(a, "test")
	if !got {
		t.Errorf("common.ContainsString = %v, want %v", got, true)
	}
}

func TestNotContainsString(t *testing.T) {
	a := []string{"this", "is", "a", "test", "slice"}
	got := ContainsString(a, "toast")
	if got {
		t.Errorf("common.ContainsString = %v, want %v", got, true)
	}
}

func TestEmptyContainsString(t *testing.T) {
	a := []string{}
	got := ContainsString(a, "")
	if got {
		t.Errorf("common.ContainsString = %v, want %v", got, true)
	}
}

func TestRemoveStringPresent(t *testing.T) {
	a := []string{"this", "is", "a", "test", "slice"}
	expected := []string{"this", "is", "a", "slice"}
	got := RemoveString(a, "test")
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("common.RemoveString = %v, want %v", got, expected)
	}
}

func TestRemoveStringNotPresent(t *testing.T) {
	a := []string{"this", "is", "a", "test", "slice"}
	expected := []string{"this", "is", "a", "test", "slice"}
	got := RemoveString(a, "toast")
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("common.RemoveString = %v, want %v", got, expected)
	}
}

func TestFromUnstructuredObj(t *testing.T) {
	un := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "addonmgr.keikoproj.io/v1alpha1",
			"kind":       "Addons",
			"Spec": map[string]interface{}{
				"pkgName":    "event-router",
				"pkgVersion": "v0.2",
				"pkgType":    "composite",
				"params": map[string]interface{}{
					"namespace": "addon-event-router-ns",
					"context": map[string]interface{}{
						"clusterName":   "cluster-name",
						"clusterRegion": "us-west-2",
					},
				},
			},
		},
	}
	x := &addonv1.Addon{}
	err := FromUnstructuredObj(un, x)
	if err != nil {
		t.Errorf("failed converting unstructure obj to addon instance, %#v", err)
	}

	converted, err := FromUnstructured(un)
	if err != nil || converted == nil {
		t.Errorf("failed converting unstructure to addon instance, %#v", err)
	}

	un = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Workflow",
			"Spec": map[string]interface{}{
				"entrypoint": "whalesay",
				"templates": map[string]interface{}{
					"name": "whalesay",
				},
			},
		}}

	wf, err := WorkFlowFromUnstructured(un)
	if err != nil || wf == nil {
		t.Errorf("failed converting unstructure to workflow instance, %#v", err)
	}
}

func Test_ConvertWorkflowPhasetoAddonPhase(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	addonPhase := ConvertWorkflowPhaseToAddonPhase(wfv1.WorkflowSucceeded)
	g.Expect(addonPhase).To(gomega.Equal(addonv1.Succeeded))

	addonPhase = ConvertWorkflowPhaseToAddonPhase(wfv1.WorkflowFailed)
	g.Expect(addonPhase).To(gomega.Equal(addonv1.Failed))

	addonPhase = ConvertWorkflowPhaseToAddonPhase(wfv1.WorkflowError)
	g.Expect(addonPhase).To(gomega.Equal(addonv1.Failed))

	addonPhase = ConvertWorkflowPhaseToAddonPhase(wfv1.WorkflowRunning)
	g.Expect(addonPhase).To(gomega.Equal(addonv1.Pending))

	addonPhase = ConvertWorkflowPhaseToAddonPhase(wfv1.WorkflowPending)
	g.Expect(addonPhase).To(gomega.Equal(addonv1.Pending))

	addonPhase = ConvertWorkflowPhaseToAddonPhase(wfv1.WorkflowUnknown)
	g.Expect(addonPhase).To(gomega.BeEmpty())
}
