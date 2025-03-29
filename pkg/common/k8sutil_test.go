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
	"testing"
	"time"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic/fake"
)

func TestToUnstructured(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Create a test workflow
	wf := &wfv1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workflow",
			Namespace: "default",
		},
		Spec: wfv1.WorkflowSpec{
			Templates: []wfv1.Template{
				{
					Name: "test-template",
				},
			},
		},
	}

	// Convert to unstructured
	un, err := ToUnstructured(wf)
	g.Expect(err).To(gomega.BeNil())
	g.Expect(un).ToNot(gomega.BeNil())
	g.Expect(un.GetKind()).To(gomega.Equal("Workflow"))
	g.Expect(un.GetAPIVersion()).To(gomega.Equal("argoproj.io/v1alpha1"))
	g.Expect(un.GetName()).To(gomega.Equal("test-workflow"))
}

func TestNewWorkflowInformer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Create a fake dynamic client
	dclient := fake.NewSimpleDynamicClient(GetAddonMgrScheme())

	// Create the informer
	informer := NewWorkflowInformer(dclient, "default", 10*time.Minute)

	// Verify it's not nil
	g.Expect(informer).ToNot(gomega.BeNil())
}
