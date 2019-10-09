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
	"context"
	"testing"
	"time"

	"github.com/ghodss/yaml"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/record"
	runtimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/keikoproj/addon-manager/api/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"
)

var sch = runtime.NewScheme()
var fclient = runtimefake.NewFakeClientWithScheme(sch)
var dynClient = dynfake.NewSimpleDynamicClient(sch)
var rcdr = record.NewBroadcasterForTests(1*time.Second).NewRecorder(sch, v1.EventSource{Component: "addons"})

var wfSpecTemplate = `
apiVersion: argoproj.io/v1alpha1
kind: Workflow
spec:
  entrypoint: entry
  serviceAccountName: addon-manager-workflow-installer-sa
  templates:
  - name: entry
    steps:
    - - name: install-deployment
        template: submit

  - name: submit
    resource:
      action: apply
      manifest: |
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: event-router
          namespace: "{{workflow.parameters.namespace}}"
          labels:
            app: event-router
        spec:
          replicas: 1
          selector:
            matchLabels:
              app: event-router
          template:
            metadata:
              labels:
                app: event-router
            spec:
              containers:
                - name: kube-event-router
                  image: gcr.io/heptio-images/eventrouter:v0.2
                  imagePullPolicy: IfNotPresent
                  volumeMounts:
                  - name: config-volume
                    mountPath: /etc/eventrouter
              serviceAccount: event-router-sa
              volumes:
                - name: config-volume
                  configMap:
                    name: event-router-cm
`

var wfPrereqsTemplate = `
apiVersion: argoproj.io/v1alpha1
kind: Workflow
spec:
  entrypoint: entry
  serviceAccountName: addon-manager-workflow-installer-sa
  templates:
  - name: entry
    steps:
    - - name: prereq-resources
        template: submit

  - name: submit
    resource:
      action: apply
      manifest: |
        apiVersion: v1
        kind: Namespace
        metadata:
          name: "{{workflow.parameters.namespace}}"
        ---
        apiVersion: v1
        kind: ServiceAccount
        metadata:
          name: event-router-sa
          namespace: "{{workflow.parameters.namespace}}"
        ---
        apiVersion: v1
        data:
          config.json: |-
            {
              "sink": "stdout"
            }
        kind: ConfigMap
        metadata:
          name: event-router-cm
          namespace: "{{workflow.parameters.namespace}}"
        ---
        apiVersion: rbac.authorization.k8s.io/v1beta1
        kind: ClusterRole
        metadata:
          name: event-router-cr
        rules:
        - apiGroups: [""]
          resources: ["events"]
          verbs: ["get", "watch", "list"]
        ---
        apiVersion: rbac.authorization.k8s.io/v1beta1
        kind: ClusterRoleBinding
        metadata:
          name: event-router-crb
        roleRef:
          apiGroup: rbac.authorization.k8s.io
          kind: ClusterRole
          name: event-router-cr
        subjects:
        - kind: ServiceAccount
          name: event-router-sa
          namespace: "{{workflow.parameters.namespace}}"
`
var wfArtifactsTemplate = `
apiVersion: argoproj.io/v1alpha1
kind: Workflow
spec:
  entrypoint: entry
  serviceAccountName: addon-manager-workflow-installer-sa
  templates:
  - name: entry
    steps:
    - - name: prereq-resources
        template: submit
        arguments:
          artifacts:
          - name: doc
            path: /tmp/doc
            raw:
              data: |
                apiVersion: v1
                kind: Namespace
                metadata:
                  name: "{{workflow.parameters.namespace}}"
                ---
                apiVersion: v1
                kind: ServiceAccount
                metadata:
                  labels:
                    k8s-addon: cluster-autoscaler.addons.k8s.io
                    k8s-app: cluster-autoscaler
                  name: cluster-autoscaler
                  namespace: "{{workflow.parameters.namespace}}"
                ---
                apiVersion: rbac.authorization.k8s.io/v1beta1
                kind: ClusterRole
                metadata:
                  name: cluster-autoscaler
                  labels:
                    k8s-addon: cluster-autoscaler.addons.k8s.io
                    k8s-app: cluster-autoscaler
                rules:
                - apiGroups: [""]
                  resources: ["events","endpoints"]
                  verbs: ["create", "patch"]
                - apiGroups: [""]
                  resources: ["pods/eviction"]
                  verbs: ["create"]
                - apiGroups: [""]
                  resources: ["pods/status"]
                  verbs: ["update"]
                - apiGroups: [""]
                  resources: ["endpoints"]
                  resourceNames: ["cluster-autoscaler"]
                  verbs: ["get","update"]
                - apiGroups: [""]
                  resources: ["nodes"]
                  verbs: ["watch","list","get","update"]
                - apiGroups: [""]
                  resources: ["pods","services","replicationcontrollers","persistentvolumeclaims","persistentvolumes"]
                  verbs: ["watch","list","get"]
                - apiGroups: ["extensions"]
                  resources: ["replicasets","daemonsets"]
                  verbs: ["watch","list","get"]
                - apiGroups: ["policy"]
                  resources: ["poddisruptionbudgets"]
                  verbs: ["watch","list"]
                - apiGroups: ["apps"]
                  resources: ["statefulsets", "replicasets"]
                  verbs: ["watch","list","get"]
                - apiGroups: ["storage.k8s.io"]
                  resources: ["storageclasses"]
                  verbs: ["watch","list","get"]
                ---
                apiVersion: rbac.authorization.k8s.io/v1
                kind: Role
                metadata:
                  name: cluster-autoscaler
                  namespace: "{{workflow.parameters.namespace}}"
                  labels:
                    k8s-addon: cluster-autoscaler.addons.k8s.io
                    k8s-app: cluster-autoscaler
                rules:
                - apiGroups: [""]
                  resources: ["configmaps"]
                  verbs: ["create"]
                - apiGroups: [""]
                  resources: ["configmaps"]
                  resourceNames: ["cluster-autoscaler-status"]
                  verbs: ["delete","get","update"]
                ---
                apiVersion: rbac.authorization.k8s.io/v1
                kind: ClusterRoleBinding
                metadata:
                  name: cluster-autoscaler
                  labels:
                    k8s-addon: cluster-autoscaler.addons.k8s.io
                    k8s-app: cluster-autoscaler
                roleRef:
                  apiGroup: rbac.authorization.k8s.io
                  kind: ClusterRole
                  name: cluster-autoscaler
                subjects:
                  - kind: ServiceAccount
                    name: cluster-autoscaler
                    namespace: "{{workflow.parameters.namespace}}"

                ---
                apiVersion: rbac.authorization.k8s.io/v1
                kind: RoleBinding
                metadata:
                  name: cluster-autoscaler
                  namespace: "{{workflow.parameters.namespace}}"
                  labels:
                    k8s-addon: cluster-autoscaler.addons.k8s.io
                    k8s-app: cluster-autoscaler
                roleRef:
                  apiGroup: rbac.authorization.k8s.io
                  kind: Role
                  name: cluster-autoscaler
                subjects:
                  - kind: ServiceAccount
                    name: cluster-autoscaler
                    namespace: "{{workflow.parameters.namespace}}"
  - name: submit
    inputs:
      artifacts:
        - name: doc
          path: /tmp/doc
    container:
      image: expert360/kubectl-awscli:v1.11.2
      command: [sh, -c]
      args: ["kubectl apply -f /tmp/doc"]
`

func init() {
	wf := &unstructured.Unstructured{}
	wf.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "Workflow",
		Group:   "argoproj.io",
		Version: "v1alpha1",
	})
	wfList := &unstructured.UnstructuredList{}
	wfList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "WorkflowList",
		Group:   "argoproj.io",
		Version: "v1alpha1",
	})
	sch.AddKnownTypes(common.AddonGVR().GroupVersion(), &v1alpha1.Addon{}, &v1alpha1.AddonList{})
	sch.AddKnownTypes(common.WorkflowGVR().GroupVersion(), wf, wfList)
	metav1.AddToGroupVersion(sch, common.WorkflowGVR().GroupVersion())
}

func TestNewWorkflowLifecycle(t *testing.T) {
	g := NewGomegaWithT(t)

	a := &v1alpha1.Addon{}

	wfl := NewWorkflowLifecycle(fclient, dynClient, a, rcdr, sch)

	var expected AddonLifecycle = &workflowLifecycle{}
	g.Expect(wfl).To(BeAssignableToTypeOf(expected))
}

func TestWorkflowLifecycle_Install_Resources(t *testing.T) {
	g := NewGomegaWithT(t)

	addon := &v1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "addon-wf-1",
			Namespace: "default",
		},
		Spec: v1alpha1.AddonSpec{
			PackageSpec: v1alpha1.PackageSpec{
				PkgName:        "my-addon",
				PkgVersion:     "1.0.0",
				PkgType:        v1alpha1.HelmPkg,
				PkgDescription: "",
				PkgDeps:        map[string]string{"core/A": "*", "core/B": "v1.0.0"},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "my-app",
				},
			},
			Lifecycle: v1alpha1.LifecycleWorkflowSpec{
				Prereqs: v1alpha1.WorkflowType{
					Template: wfPrereqsTemplate,
				},
				Install: v1alpha1.WorkflowType{
					NamePrefix: "test",
					Role:       "myrole",
					Template:   wfSpecTemplate,
				},
			},
		},
	}

	wfl := NewWorkflowLifecycle(fclient, dynClient, addon, rcdr, sch)
	for _, lifecycle := range []v1alpha1.LifecycleStep{v1alpha1.Prereqs, v1alpha1.Install} {

		wfName := addon.GetFormattedWorkflowName(lifecycle)
		wt, _ := addon.GetWorkflowType(lifecycle)

		phase, err := wfl.Install(context.Background(), wt, wfName)

		g.Expect(err).To(Not(HaveOccurred()))
		g.Expect(phase).To(Equal(v1alpha1.Pending))

		var wfv1 = &unstructured.Unstructured{}
		wfv1.SetGroupVersionKind(schema.GroupVersionKind{
			Kind:    "Workflow",
			Group:   "argoproj.io",
			Version: "v1alpha1",
		})
		var wfv1Key = types.NamespacedName{Name: wfName, Namespace: "default"}
		g.Eventually(func() error { return fclient.Get(context.TODO(), wfv1Key, wfv1) }, timeout).
			Should(Succeed())

		// Verify labels and annotations were added to resources
		templates, found, _ := unstructured.NestedSlice(wfv1.UnstructuredContent(), "spec", "templates")
		g.Expect(found).To(BeTrue())

		step := templates[1]
		manifest, found, _ := unstructured.NestedString(step.(map[string]interface{}), "resource", "manifest")
		g.Expect(found).To(BeTrue())

		u := &unstructured.Unstructured{}
		_ = yaml.Unmarshal([]byte(manifest), u)
		labels := u.GetLabels()
		g.Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/name", addon.GetName()))
		g.Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/version", addon.Spec.PkgVersion))
		g.Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/part-of", addon.GetName()))
		g.Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "addonmgr.keikoproj.io"))
	}
}

func TestWorkflowLifecycle_Install_Artifacts(t *testing.T) {
	g := NewGomegaWithT(t)

	addon := &v1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "addon-wf-2",
			Namespace: "default",
		},
		Spec: v1alpha1.AddonSpec{
			PackageSpec: v1alpha1.PackageSpec{
				PkgName:        "my-addon-2",
				PkgVersion:     "1.2.0",
				PkgType:        v1alpha1.HelmPkg,
				PkgDescription: "",
				PkgDeps:        map[string]string{"core/A": "*", "core/B": "v1.0.0"},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "my-app-2",
				},
			},
			Lifecycle: v1alpha1.LifecycleWorkflowSpec{
				Prereqs: v1alpha1.WorkflowType{
					Template: wfArtifactsTemplate,
				},
				Install: v1alpha1.WorkflowType{
					Template: wfArtifactsTemplate,
				},
			},
		},
	}

	wfl := NewWorkflowLifecycle(fclient, dynClient, addon, rcdr, sch)
	for _, lifecycle := range []v1alpha1.LifecycleStep{v1alpha1.Prereqs, v1alpha1.Install} {

		wfName := addon.GetFormattedWorkflowName(lifecycle)
		wt, _ := addon.GetWorkflowType(lifecycle)

		phase, err := wfl.Install(context.Background(), wt, wfName)

		g.Expect(err).To(Not(HaveOccurred()))
		g.Expect(phase).To(Equal(v1alpha1.Pending))

		var wfv1 = &unstructured.Unstructured{}
		wfv1.SetGroupVersionKind(schema.GroupVersionKind{
			Kind:    "Workflow",
			Group:   "argoproj.io",
			Version: "v1alpha1",
		})
		var wfv1Key = types.NamespacedName{Name: wfName, Namespace: "default"}
		g.Eventually(func() error { return fclient.Get(context.TODO(), wfv1Key, wfv1) }, timeout).
			Should(Succeed())

		// Verify labels and annotations were added to resources
		templates, found, _ := unstructured.NestedSlice(wfv1.UnstructuredContent(), "spec", "templates")
		g.Expect(found).To(BeTrue())
		template := templates[0]
		var data string
		if steps, found, _ := unstructured.NestedSlice(template.(map[string]interface{}), "steps"); found {
			for _, step := range steps {
				for _, stepTemplate := range step.([]interface{}) {
					if artifacts, found, _ := unstructured.NestedFieldNoCopy(stepTemplate.(map[string]interface{}), "arguments", "artifacts"); found {
						for _, argArtifact := range artifacts.([]interface{}) {
							if manifest, found, _ := unstructured.NestedString(argArtifact.(map[string]interface{}), "raw", "data"); found {
								data = manifest
							} else {
								t.Errorf("No raw data resources found. Expected that we would find resources at .spec.templates[].arguments.artifacts[].raw.data")
							}
						}
					} else {
						t.Errorf("No arguments artifacts found. Expected that we would find .spec.templates[].arguments.artifacts")
					}
				}
			}
		} else {
			t.Errorf("No resources found. Expected that we would find one of resource patterns in workflow.")
		}
		u := &unstructured.Unstructured{}
		_ = yaml.Unmarshal([]byte(data), u)
		labels := u.GetLabels()
		g.Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/name", addon.GetName()))
		g.Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/version", addon.Spec.PkgVersion))
		g.Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/part-of", addon.GetName()))
		g.Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "addonmgr.keikoproj.io"))
	}

}

// Test that an empty workflow type will fail
func TestWorkflowLifecycle_Install_InvalidWorkflowType(t *testing.T) {
	g := NewGomegaWithT(t)

	a := &v1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: v1alpha1.AddonSpec{
			PackageSpec: v1alpha1.PackageSpec{
				PkgName:        "my-addon",
				PkgVersion:     "1.0.0",
				PkgType:        v1alpha1.HelmPkg,
				PkgDescription: "",
				PkgDeps:        map[string]string{"core/A": "*", "core/B": "v1.0.0"},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "my-app",
				},
			},
		},
	}

	wfl := NewWorkflowLifecycle(fclient, dynClient, a, rcdr, sch)

	// Empty workflow type should fail
	wt := &v1alpha1.WorkflowType{}

	phase, err := wfl.Install(context.Background(), wt, "addon-wf-test")

	g.Expect(err).To(HaveOccurred())
	g.Expect(phase).To(Equal(v1alpha1.Failed))
}

func TestWorkflowLifecycle_Delete_NotExists(t *testing.T) {
	g := NewGomegaWithT(t)

	a := &v1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: v1alpha1.AddonSpec{
			PackageSpec: v1alpha1.PackageSpec{
				PkgName:        "my-addon",
				PkgVersion:     "1.0.0",
				PkgType:        v1alpha1.HelmPkg,
				PkgDescription: "",
				PkgDeps:        map[string]string{"core/A": "*", "core/B": "v1.0.0"},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "my-app",
				},
			},
		},
	}

	wfl := NewWorkflowLifecycle(fclient, dynClient, a, rcdr, sch)

	g.Expect(wfl.Delete("addon-wf-test")).To(HaveOccurred())
}

func TestNewWorkflowLifecycle_Delete(t *testing.T) {
	g := NewGomegaWithT(t)

	a := &v1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: v1alpha1.AddonSpec{
			PackageSpec: v1alpha1.PackageSpec{
				PkgName:        "my-addon",
				PkgVersion:     "1.0.0",
				PkgType:        v1alpha1.HelmPkg,
				PkgDescription: "",
				PkgDeps:        map[string]string{"core/A": "*", "core/B": "v1.0.0"},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "my-app",
				},
			},
		},
	}

	wfl := NewWorkflowLifecycle(fclient, dynClient, a, rcdr, sch)

	wf := &unstructured.Unstructured{}
	wf.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "Workflow",
		Group:   "argoproj.io",
		Version: "v1alpha1",
	})

	wf.SetNamespace("default")
	wf.SetName("addon-wf-test")

	_, err := dynClient.Resource(common.WorkflowGVR()).Namespace("default").Create(wf, metav1.CreateOptions{})
	g.Expect(err).To(Not(HaveOccurred()))

	// Now try to delete
	g.Expect(wfl.Delete("addon-wf-test")).To(Not(HaveOccurred()))
}
