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
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"
)

const (
	ErrDepNotInstalled = "required dependency is not installed"
	ErrDepPending      = "required dependency is in pending state"
)

type addonValidator struct {
	cache     VersionCacheClient
	addon     *addonmgrv1alpha1.Addon
	dynClient dynamic.Interface
}

// NewAddonValidator returns an object implementing common.Validator
func NewAddonValidator(addon *addonmgrv1alpha1.Addon, cache VersionCacheClient, dynClient dynamic.Interface) common.Validator {
	return &addonValidator{
		cache:     cache,
		addon:     addon,
		dynClient: dynClient,
	}
}

func (av *addonValidator) Validate() (bool, error) {
	var version = &Version{
		Name:        av.addon.GetName(),
		Namespace:   av.addon.GetNamespace(),
		PackageSpec: av.addon.GetPackageSpec(),
		PkgPhase:    av.addon.Status.Lifecycle.Installed,
	}

	// Validate version is not already set in cache, dupe.
	err := av.validateDuplicate(version)
	if err != nil {
		return false, err
	}

	// Validate length of addon name
	err = av.validateAddonNameLength()
	if err != nil {
		return false, err
	}

	// Validate that the addon has been given a namespace
	if av.addon.Spec.Params.Namespace == "" {
		return false, fmt.Errorf("namespace is empty in addon.spec.params.namespace")
	}

	// Validate workflow template is actually a workflow
	err = av.validateWorkflow()
	if err != nil {
		return false, err
	}

	// Validate dependencies are resolvable, no diamond dependency cycles.
	var visited = make(map[string]*Version)
	err = av.resolveDependencies(version, visited, 0)
	if err != nil {
		return false, err
	}

	// Validate dependencies are installed.
	err = av.validateDependencies()
	if err != nil {
		return false, err
	}

	return true, nil
}

func (av *addonValidator) validateDuplicate(version *Version) error {
	if v := av.cache.GetVersion(version.PkgName, version.PkgVersion); v != nil && v.Name != version.Name {
		return fmt.Errorf("package version %s:%s already exists and cannot be installed as a duplicate", av.addon.Spec.PkgName, av.addon.Spec.PkgVersion)
	}

	return nil
}

func (av *addonValidator) validateWorkflow() error {
	var data map[string]interface{}

	workflowTypes := map[addonmgrv1alpha1.LifecycleStep]addonmgrv1alpha1.WorkflowType{
		addonmgrv1alpha1.Prereqs:  av.addon.Spec.Lifecycle.Prereqs,
		addonmgrv1alpha1.Install:  av.addon.Spec.Lifecycle.Install,
		addonmgrv1alpha1.Delete:   av.addon.Spec.Lifecycle.Delete,
		addonmgrv1alpha1.Validate: av.addon.Spec.Lifecycle.Validate,
	}

	for key, wt := range workflowTypes {
		if wt.Template == "" {
			continue
		}

		wf := &unstructured.Unstructured{}

		// Load workflow spec into data obj
		if err := yaml.Unmarshal([]byte(wt.Template), &data); err != nil {
			return fmt.Errorf("invalid workflow template %q. %v", key, err)
		}

		wf.SetUnstructuredContent(data)

		argoGKV := schema.GroupVersionKind{
			Kind:    "Workflow",
			Group:   "argoproj.io",
			Version: "v1alpha1",
		}

		if wf.GroupVersionKind() != argoGKV {
			return fmt.Errorf("invalid workflow, type is not a valid kind %s and api-version %s/%s", argoGKV.Kind, argoGKV.Group, argoGKV.Version)
		}

		if _, ok := data["spec"]; !ok {
			return fmt.Errorf("invalid workflow, missing spec")
		}

		_, found, _ := unstructured.NestedMap(wf.UnstructuredContent(), "spec", "arguments")
		if !found {
			continue
		}

		wfParameters, found, _ := unstructured.NestedSlice(wf.UnstructuredContent(), "spec", "arguments", "parameters")
		if !found {
			continue
		}

		addonParams := av.addon.GetAllAddonParameters()

		// Ensure there are no parameter naming overlaps
		for _, wfParam := range wfParameters {
			wfParamName := wfParam.(map[string]interface{})["name"].(string)
			if _, in := addonParams[wfParamName]; in {
				return fmt.Errorf("invalid workflow, parameter named %q found in addon params and in workflow %s", wfParamName, av.addon.GetFormattedWorkflowName(key))
			}
		}
	}

	return nil
}

func (av *addonValidator) validateAddonNameLength() error {
	if len(av.addon.Name) > 31 {
		return fmt.Errorf("Addon name %s must be less than 32 characters", av.addon.Name)
	}
	return nil
}

func (av *addonValidator) validateDependencies() error {
	// Check addon cache to see that addon pkgName:pkgVersion was installed
	for pkgName, pkgVersion := range av.addon.Spec.PkgDeps {
		pkgName = strings.TrimSpace(pkgName)
		pkgVersion = strings.TrimSpace(pkgVersion)

		if pkgVersion == "*" {
			// Ignore version
			versions := av.cache.GetVersions(pkgName)
			if versions == nil {
				return fmt.Errorf("required dependency %s is not installed", pkgName)
			}

			// Look for any successfully installed version
			var versionFound = false
			for _, v := range versions {
				if v.PkgPhase == addonmgrv1alpha1.Succeeded {
					versionFound = true
					break
				}
			}

			if !versionFound {
				return fmt.Errorf("required dependency %s has no valid versions installed", pkgName)
			}
		} else {
			// Check for specific version
			v := av.cache.GetVersion(pkgName, pkgVersion)
			if v == nil {
				return fmt.Errorf(ErrDepNotInstalled+": %q:%q", pkgName, pkgVersion)
			}

			switch v.PkgPhase {
			case addonmgrv1alpha1.Succeeded:
				return nil
			case addonmgrv1alpha1.Pending:
				return fmt.Errorf(ErrDepPending+": %q:%q", pkgName, pkgVersion)
			default:
				return fmt.Errorf(ErrDepNotInstalled+": %q:%q", pkgName, pkgVersion)
			}
		}
	}

	return nil
}

func (av *addonValidator) resolveDependencies(n *Version, visited map[string]*Version, depth int) error {
	if depth >= 256 {
		panic("Recursive max depth of 256 seen, this is bad!")
	}

	// Add to visited
	name := n.PkgName + ":" + n.PkgVersion
	visited[name] = n

	for pkgName, pkgVersion := range n.PkgDeps {
		pkgName = strings.TrimSpace(pkgName)
		pkgVersion = strings.TrimSpace(pkgVersion)

		if pkgName == n.PkgName {
			return fmt.Errorf("invalid package dependency, addon cannot depend on it's own package name %s:%s", pkgName, pkgVersion)
		}

		v := av.cache.GetVersion(pkgName, pkgVersion)
		if v == nil {
			// Unresolvable dependency
			return fmt.Errorf("unable to resolve required dependency %s:%s", pkgName, pkgVersion)
		}

		// Validate it resolves without cyclic dependency
		dname := v.PkgName + ":" + v.PkgVersion
		if _, ok := visited[dname]; ok {
			return fmt.Errorf("circular dependency was found in %s:%s", v.PkgName, v.PkgVersion)
		}

		// Recursive - make sure we catch all cases.
		err := av.resolveDependencies(v, visited, depth+1)
		if err != nil {
			return err
		}

	}

	// Remove visited nodes after
	delete(visited, name)

	return nil
}
