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

package addon

import (
	"reflect"
	"testing"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"

	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/v1alpha1"
)

var dynClient = fake.NewSimpleDynamicClient(runtime.NewScheme())

func TestNewAddonValidator(t *testing.T) {
	var cache = NewAddonVersionCacheClient()
	var addon = &addonmgrv1alpha1.Addon{}
	type args struct {
		addon *addonmgrv1alpha1.Addon
	}

	tests := []struct {
		name string
		args args
		want *addonValidator
	}{
		{name: "test-valid", args: args{addon: addon}, want: &addonValidator{cache: cache, addon: addon, dynClient: dynClient}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewAddonValidator(tt.args.addon, cache, dynClient); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewAddonValidator() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_addonValidator_Validate_Workflow_Template(t *testing.T) {
	var cache = NewAddonVersionCacheClient()
	type fields struct {
		addon *addonmgrv1alpha1.Addon
	}
	tests := []struct {
		name    string
		fields  fields
		want    bool
		wantErr bool
	}{
		{name: "workflow-template-valid", fields: fields{addon: &addonmgrv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
			Spec: addonmgrv1alpha1.AddonSpec{
				PackageSpec: addonmgrv1alpha1.PackageSpec{
					PkgType:    addonmgrv1alpha1.CompositePkg,
					PkgName:    "test/addon-1",
					PkgVersion: "1.0.0",
				},
				Params: addonmgrv1alpha1.AddonParams{
					Namespace: "addon-test-ns",
				},
				Lifecycle: addonmgrv1alpha1.LifecycleWorkflowSpec{
					Install: addonmgrv1alpha1.WorkflowType{
						NamePrefix: "test",
						Role:       "arn:12345",
						Template: `
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
  - name: submit
    inputs:
      artifacts:
        - name: doc
          path: /tmp/doc
    container:
      image: expert360/kubectl-awscli:v1.11.2
      command: [sh, -c]
      args: ["kubectl apply -f /tmp/doc"]
`,
					},
				},
			},
		}}, want: true, wantErr: false},
		{name: "workflow-template-valid-empty-template", fields: fields{addon: &addonmgrv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
			Spec: addonmgrv1alpha1.AddonSpec{
				PackageSpec: addonmgrv1alpha1.PackageSpec{
					PkgType:    addonmgrv1alpha1.CompositePkg,
					PkgName:    "test/addon-1",
					PkgVersion: "1.0.0",
				},
				Params: addonmgrv1alpha1.AddonParams{
					Namespace: "addon-test-ns",
				},
				Lifecycle: addonmgrv1alpha1.LifecycleWorkflowSpec{
					Install: addonmgrv1alpha1.WorkflowType{
						NamePrefix: "test",
						Role:       "arn:12345",
						Template:   "",
					},
				},
			},
		}}, want: true, wantErr: false},
		{name: "workflow-template-invalid-kind", fields: fields{addon: &addonmgrv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
			Spec: addonmgrv1alpha1.AddonSpec{
				PackageSpec: addonmgrv1alpha1.PackageSpec{
					PkgType:    addonmgrv1alpha1.CompositePkg,
					PkgName:    "test/addon-1",
					PkgVersion: "1.0.0",
				},
				Params: addonmgrv1alpha1.AddonParams{
					Namespace: "addon-test-ns",
				},
				Lifecycle: addonmgrv1alpha1.LifecycleWorkflowSpec{
					Prereqs: addonmgrv1alpha1.WorkflowType{
						NamePrefix: "test",
						Role:       "arn:12345",
						Template: `
apiVersion: argoproj.io/v1alpha1
kind: Workflowz
spec:
  entrypoint: entry
  serviceAccountName: addon-manager-workflow-installer-sa
`,
					},
				},
			},
		}}, want: false, wantErr: true},
		{name: "workflow-template-invalid-missing-spec", fields: fields{addon: &addonmgrv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
			Spec: addonmgrv1alpha1.AddonSpec{
				PackageSpec: addonmgrv1alpha1.PackageSpec{
					PkgType:    addonmgrv1alpha1.CompositePkg,
					PkgName:    "test/addon-1",
					PkgVersion: "1.0.0",
				},
				Params: addonmgrv1alpha1.AddonParams{
					Namespace: "addon-test-ns",
				},
				Lifecycle: addonmgrv1alpha1.LifecycleWorkflowSpec{
					Delete: addonmgrv1alpha1.WorkflowType{
						NamePrefix: "test",
						Role:       "arn:12345",
						Template: `
apiVersion: argoproj.io/v1alpha1
kind: Workflow
`,
					},
				},
			},
		}}, want: false, wantErr: true},
		{name: "workflow-invalid-missing-namespace", fields: fields{addon: &addonmgrv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
			Spec: addonmgrv1alpha1.AddonSpec{
				PackageSpec: addonmgrv1alpha1.PackageSpec{
					PkgType:    addonmgrv1alpha1.CompositePkg,
					PkgName:    "test/addon-1",
					PkgVersion: "1.0.0",
				},
				Params: addonmgrv1alpha1.AddonParams{
					Data: map[string]addonmgrv1alpha1.FlexString{
						"foo": "difval",
					},
				},
				Lifecycle: addonmgrv1alpha1.LifecycleWorkflowSpec{
					Delete: addonmgrv1alpha1.WorkflowType{
						NamePrefix: "test",
						Role:       "arn:12345",
						Template: `
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
		  parameters:
			- name: foo
			  value: bar
`,
					},
				},
			},
		}}, want: false, wantErr: true},
		{name: "workflow-invalid-overlapping-params", fields: fields{addon: &addonmgrv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
			Spec: addonmgrv1alpha1.AddonSpec{
				PackageSpec: addonmgrv1alpha1.PackageSpec{
					PkgType:    addonmgrv1alpha1.CompositePkg,
					PkgName:    "test/addon-1",
					PkgVersion: "1.0.0",
				},
				Params: addonmgrv1alpha1.AddonParams{
					Namespace: "addon-test-ns",
					Data: map[string]addonmgrv1alpha1.FlexString{
						"foo": "difval",
					},
				},
				Lifecycle: addonmgrv1alpha1.LifecycleWorkflowSpec{
					Delete: addonmgrv1alpha1.WorkflowType{
						NamePrefix: "test",
						Role:       "arn:12345",
						Template: `
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
		  parameters:
			- name: foo
			  value: bar
`,
					},
				},
			},
		}}, want: false, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			av := &addonValidator{
				addon:     tt.fields.addon,
				cache:     cache,
				dynClient: dynClient,
			}
			got, err := av.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("addonValidator.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("addonValidator.Validate() = %v, want %v", got, tt.want)
			}
		})
	}
}
func Test_addonValidator_Validate_Fail_NameLength(t *testing.T) {
	var cache = NewAddonVersionCacheClient()
	type fields struct {
		addon *addonmgrv1alpha1.Addon
	}
	tests := []struct {
		name    string
		fields  fields
		want    bool
		wantErr bool
	}{
		// TODO: Add test cases.
		{name: "addon-fails-with-name-too-long", fields: fields{addon: &addonmgrv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{Name: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Namespace: "default"},
			Spec: addonmgrv1alpha1.AddonSpec{
				PackageSpec: addonmgrv1alpha1.PackageSpec{
					PkgType:    addonmgrv1alpha1.CompositePkg,
					PkgName:    "test/addon-1",
					PkgVersion: "1.0.0",
				},
				Params: addonmgrv1alpha1.AddonParams{
					Namespace: "addon-test-ns",
				},
			},
		}}, want: false, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			av := &addonValidator{
				addon:     tt.fields.addon,
				cache:     cache,
				dynClient: dynClient,
			}
			got, err := av.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("addonValidator.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("addonValidator.Validate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_addonValidator_Validate_NoDeps(t *testing.T) {
	var cache = NewAddonVersionCacheClient()
	type fields struct {
		addon *addonmgrv1alpha1.Addon
	}
	tests := []struct {
		name    string
		fields  fields
		want    bool
		wantErr bool
	}{
		// TODO: Add test cases.
		{name: "addon-validates-no-dependencies", fields: fields{addon: &addonmgrv1alpha1.Addon{Spec: addonmgrv1alpha1.AddonSpec{Params: addonmgrv1alpha1.AddonParams{Namespace: "addon-test-ns"}}}}, want: true, wantErr: false},
		{name: "addon-fails-with-uninstalled-dependencies", fields: fields{addon: &addonmgrv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
			Spec: addonmgrv1alpha1.AddonSpec{
				PackageSpec: addonmgrv1alpha1.PackageSpec{
					PkgType:    addonmgrv1alpha1.CompositePkg,
					PkgName:    "test/addon-1",
					PkgVersion: "1.0.0",
					PkgDeps: map[string]string{
						"core/A": "*",
						"core/B": "v1.0.0",
					},
				},
				Params: addonmgrv1alpha1.AddonParams{
					Namespace: "addon-test-ns",
				},
			},
		}}, want: false, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			av := &addonValidator{
				addon:     tt.fields.addon,
				cache:     cache,
				dynClient: dynClient,
			}
			got, err := av.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("addonValidator.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("addonValidator.Validate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_addonValidator_Validate_With_Installed_Deps(t *testing.T) {
	// Pre-cache installed dependencies
	var cache = NewAddonVersionCacheClient()
	var versionA = Version{
		PackageSpec: addonmgrv1alpha1.PackageSpec{
			PkgName:    "core/A",
			PkgVersion: "v1.0.0",
			PkgDeps: map[string]string{
				"core/C": "v1.2.0",
			},
		},
		PkgPhase: addonmgrv1alpha1.Succeeded,
	}
	var versionB = Version{
		PackageSpec: addonmgrv1alpha1.PackageSpec{
			PkgName:    "core/B",
			PkgVersion: "v1.0.0",
			PkgDeps: map[string]string{
				"core/C": "v1.2.0",
			},
		},
		PkgPhase: addonmgrv1alpha1.Succeeded,
	}
	var versionC = Version{
		PackageSpec: addonmgrv1alpha1.PackageSpec{
			PkgName:    "core/C",
			PkgVersion: "1.2.0",
		},
		PkgPhase: addonmgrv1alpha1.Succeeded,
	}
	cache.AddVersion(versionA)
	cache.AddVersion(versionB)
	cache.AddVersion(versionC)

	type fields struct {
		addon *addonmgrv1alpha1.Addon
	}
	tests := []struct {
		name    string
		fields  fields
		want    bool
		wantErr bool
	}{
		// TODO: Add test cases.
		{name: "addon-validates-dependencies", fields: fields{addon: &addonmgrv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
			Spec: addonmgrv1alpha1.AddonSpec{
				PackageSpec: addonmgrv1alpha1.PackageSpec{PkgType: addonmgrv1alpha1.CompositePkg,
					PkgName:    "test/addon-1",
					PkgVersion: "1.0.0",
					PkgDeps: map[string]string{
						"core/A": "*",
						"core/B": "v1.0.0",
					},
				},
				Params: addonmgrv1alpha1.AddonParams{
					Namespace: "addon-test-ns",
				},
			},
		}}, want: true, wantErr: false},
		{name: "addon-fails-with-uninstalled-dependencies", fields: fields{addon: &addonmgrv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
			Spec: addonmgrv1alpha1.AddonSpec{
				PackageSpec: addonmgrv1alpha1.PackageSpec{
					PkgType:    addonmgrv1alpha1.CompositePkg,
					PkgName:    "test/addon-1",
					PkgVersion: "1.0.0",
					PkgDeps: map[string]string{
						"core/A": "*",
						"core/B": "v1.0.1",
					},
				},
				Params: addonmgrv1alpha1.AddonParams{
					Namespace: "addon-test-ns",
				},
			},
		}}, want: false, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			av := &addonValidator{
				addon:     tt.fields.addon,
				cache:     cache,
				dynClient: dynClient,
			}
			got, err := av.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("addonValidator.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("addonValidator.Validate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_addonValidator_validateDependencies(t *testing.T) {
	var cache = NewAddonVersionCacheClient()
	type fields struct {
		addon *addonmgrv1alpha1.Addon
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			av := &addonValidator{
				addon:     tt.fields.addon,
				cache:     cache,
				dynClient: dynClient,
			}
			if err := av.validateDependencies(); (err != nil) != tt.wantErr {
				t.Errorf("addonValidator.validateDependencies() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_resolveDependencies(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	cached := NewAddonVersionCacheClient()

	// Add core/A
	cached.AddVersion(Version{
		PackageSpec: addonmgrv1alpha1.PackageSpec{
			PkgName:    "core/A",
			PkgVersion: "1.0.3",
			PkgDeps: map[string]string{
				"core/C": "*",
			},
		},
		PkgPhase: addonmgrv1alpha1.Pending,
	})

	// Add core/B
	cached.AddVersion(Version{
		PackageSpec: addonmgrv1alpha1.PackageSpec{
			PkgName:    "core/B",
			PkgVersion: "1.0.0",
			PkgDeps: map[string]string{
				"core/C": "*",
			},
		},
		PkgPhase: addonmgrv1alpha1.Pending,
	})

	// Add core/C
	cached.AddVersion(Version{
		PackageSpec: addonmgrv1alpha1.PackageSpec{
			PkgName:    "core/C",
			PkgVersion: "1.0.1",
		},
		PkgPhase: addonmgrv1alpha1.Succeeded,
	})

	av := &addonValidator{
		addon: &addonmgrv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
			Spec: addonmgrv1alpha1.AddonSpec{
				PackageSpec: addonmgrv1alpha1.PackageSpec{PkgType: addonmgrv1alpha1.CompositePkg,
					PkgName:    "test/addon-1",
					PkgVersion: "1.0.0",
					PkgDeps: map[string]string{
						"core/A": "*",
						"core/B": "v1.0.0",
					},
				},
				Params: addonmgrv1alpha1.AddonParams{
					Namespace: "addon-test-ns",
				},
			},
		},
		cache:     cached,
		dynClient: dynClient,
	}

	var visited = make(map[string]*Version)

	g.Expect(av.resolveDependencies(&Version{
		PackageSpec: av.addon.GetPackageSpec(),
		PkgPhase:    addonmgrv1alpha1.Pending,
	}, visited, 0)).Should(gomega.BeNil(), "Should validate")
}

func Test_resolveDependencies_Fail(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	cached := NewAddonVersionCacheClient()

	// Add core/A
	cached.AddVersion(Version{
		PackageSpec: addonmgrv1alpha1.PackageSpec{
			PkgName:    "core/A",
			PkgVersion: "1.0.3",
			PkgDeps: map[string]string{
				"core/C": "*",
			},
		},
		PkgPhase: addonmgrv1alpha1.Pending,
	})

	// Add core/B
	cached.AddVersion(Version{
		PackageSpec: addonmgrv1alpha1.PackageSpec{
			PkgName:    "core/B",
			PkgVersion: "1.0.0",
			PkgDeps: map[string]string{
				"core/C": "*",
			},
		},
		PkgPhase: addonmgrv1alpha1.Pending,
	})

	// Add core/C
	cached.AddVersion(Version{
		PackageSpec: addonmgrv1alpha1.PackageSpec{
			PkgName:    "core/C",
			PkgVersion: "1.0.1",
			PkgDeps: map[string]string{
				"core/A": "*", // Invalid cyclic dependency
			},
		},
		PkgPhase: addonmgrv1alpha1.Succeeded,
	})

	av := &addonValidator{
		addon: &addonmgrv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
			Spec: addonmgrv1alpha1.AddonSpec{
				PackageSpec: addonmgrv1alpha1.PackageSpec{
					PkgType:    addonmgrv1alpha1.CompositePkg,
					PkgName:    "test/addon-1",
					PkgVersion: "1.0.0",
					PkgDeps: map[string]string{
						"core/A": "*",
						"core/B": "v1.0.0",
					},
				},
				Params: addonmgrv1alpha1.AddonParams{
					Namespace: "addon-test-ns",
				},
			},
		},
		cache:     cached,
		dynClient: dynClient,
	}

	var visited = make(map[string]*Version)

	g.Expect(av.resolveDependencies(&Version{
		PackageSpec: av.addon.GetPackageSpec(),
		PkgPhase:    addonmgrv1alpha1.Pending,
	}, visited, 0)).ShouldNot(gomega.Succeed(), "Should not validate")
}

func Test_validateDuplicate_Fail(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	cached := NewAddonVersionCacheClient()

	// Add core/A
	cached.AddVersion(Version{
		Name:      "core-a",
		Namespace: "default",
		PackageSpec: addonmgrv1alpha1.PackageSpec{
			PkgName:    "core/A",
			PkgVersion: "1.0.3",
			PkgDeps: map[string]string{
				"core/C": "*",
			},
		},
		PkgPhase: addonmgrv1alpha1.Succeeded,
	})

	// Add core/B
	cached.AddVersion(Version{
		Name:      "core-b",
		Namespace: "default",
		PackageSpec: addonmgrv1alpha1.PackageSpec{
			PkgName:    "core/B",
			PkgVersion: "1.0.0",
			PkgDeps: map[string]string{
				"core/C": "*",
			},
		},
		PkgPhase: addonmgrv1alpha1.Succeeded,
	})

	// Add test-addon-1
	cached.AddVersion(Version{
		Name:      "test-addon-1",
		Namespace: "default",
		PackageSpec: addonmgrv1alpha1.PackageSpec{
			PkgName:    "test/addon-1",
			PkgVersion: "1.0.0",
			PkgDeps:    map[string]string{},
		},
		PkgPhase: addonmgrv1alpha1.Succeeded,
	})

	av := &addonValidator{
		addon: &addonmgrv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{Name: "test-addon-2", Namespace: "default"},
			Spec: addonmgrv1alpha1.AddonSpec{
				PackageSpec: addonmgrv1alpha1.PackageSpec{
					PkgType: addonmgrv1alpha1.CompositePkg,
					// Duplicate package name and version
					PkgName:    "test/addon-1",
					PkgVersion: "1.0.0",
					PkgDeps: map[string]string{
						"core/A": "*",
						"core/B": "v1.0.0",
					},
				},
				Params: addonmgrv1alpha1.AddonParams{
					Namespace: "addon-test-ns",
				},
			},
		},
		cache:     cached,
		dynClient: dynClient,
	}

	g.Expect(av.validateDuplicate(&Version{
		Name:        av.addon.Name,
		Namespace:   av.addon.Namespace,
		PackageSpec: av.addon.GetPackageSpec(),
		PkgPhase:    addonmgrv1alpha1.Pending,
	})).ShouldNot(gomega.Succeed(), "Should not validate")
}
