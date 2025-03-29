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

	"github.com/onsi/gomega"
)

func TestAddonGVR(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	gvr := AddonGVR()

	g.Expect(gvr.Group).To(gomega.Equal("addonmgr.keikoproj.io"))
	g.Expect(gvr.Version).To(gomega.Equal("v1alpha1"))
	g.Expect(gvr.Resource).To(gomega.Equal("addons"))
}

func TestCRDGVR(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	gvr := CRDGVR()

	g.Expect(gvr.Group).To(gomega.Equal("apiextensions.k8s.io"))
	g.Expect(gvr.Version).To(gomega.Equal("v1"))
	g.Expect(gvr.Resource).To(gomega.Equal("customresourcedefinitions"))
}

func TestSecretGVR(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	gvr := SecretGVR()

	g.Expect(gvr.Group).To(gomega.Equal(""))
	g.Expect(gvr.Version).To(gomega.Equal("v1"))
	g.Expect(gvr.Resource).To(gomega.Equal("secrets"))
}

func TestWorkflowGVR(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	gvr := WorkflowGVR()

	g.Expect(gvr.Group).To(gomega.Equal("argoproj.io"))
	g.Expect(gvr.Version).To(gomega.Equal("v1alpha1"))
	g.Expect(gvr.Resource).To(gomega.Equal("workflows"))
}

func TestWorkflowType(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	wf := WorkflowType()

	g.Expect(wf.GetKind()).To(gomega.Equal("Workflow"))
	g.Expect(wf.GetAPIVersion()).To(gomega.Equal("argoproj.io/v1alpha1"))
}
