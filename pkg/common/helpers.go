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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	wfv1versioned "github.com/argoproj/argo-workflows/v3/pkg/client/clientset/versioned"
	addonv1versioned "github.com/keikoproj/addon-manager/pkg/client/clientset/versioned"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
)

// ContainsString helper function to check string in a slice of strings.
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// RemoveString helper function to remove a string in a slice of strings.
func RemoveString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

// GetCurretTimestamp -- get current timestamp in millisecond
func GetCurretTimestamp() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

// IsExpired --- check if reached ttl time
func IsExpired(startTime int64, ttlTime int64) bool {
	if GetCurretTimestamp()-startTime >= ttlTime {
		return true
	}
	return false
}

// NewWFClient -- declare new workflow client
func NewWFClient(cfg *rest.Config) wfv1versioned.Interface {
	cli, err := wfv1versioned.NewForConfig(cfg)
	if err != nil {
		return nil
	}
	return cli
}

// NewAddonClient - declare new addon client
func NewAddonClient(cfg *rest.Config) addonv1versioned.Interface {
	cli, err := addonv1versioned.NewForConfig(cfg)
	if err != nil {
		return nil
	}
	return cli
}

func WorkFlowFromUnstructured(un *unstructured.Unstructured) (*wfv1.Workflow, error) {
	var wf wfv1.Workflow
	err := FromUnstructuredObj(un, &wf)
	return &wf, err
}

func FromUnstructured(un *unstructured.Unstructured) (*addonv1.Addon, error) {
	var addon addonv1.Addon
	err := FromUnstructuredObj(un, &addon)
	return &addon, err
}

// FromUnstructuredObj convert unstructured to objects
func FromUnstructuredObj(un *unstructured.Unstructured, v interface{}) error {
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(un.Object, v)
	if err != nil {
		if err.Error() == "cannot convert int64 to v1alpha1.AnyString" {
			data, err := json.Marshal(un)
			if err != nil {
				return err
			}
			return json.Unmarshal(data, v)
		}
		return err
	}
	return nil
}

func ConvertWorkflowPhaseToAddonPhase(lifecycle addonv1.LifecycleStep, phase wfv1.WorkflowPhase) addonv1.ApplicationAssemblyPhase {

	switch phase {
	case wfv1.WorkflowPending, wfv1.WorkflowRunning:
		if lifecycle == addonv1.Delete {
			return addonv1.Deleting
		}
		return addonv1.Pending
	case wfv1.WorkflowSucceeded:
		if lifecycle == addonv1.Delete {
			return addonv1.DeleteSucceeded
		}
		return addonv1.Succeeded
	case wfv1.WorkflowFailed, wfv1.WorkflowError:
		if lifecycle == addonv1.Delete {
			return addonv1.DeleteFailed
		}
		return addonv1.Failed
	default:
		return ""
	}
}

// ExtractChecksumAndLifecycleStep extracts the checksum and lifecycle step from the workflow name
func ExtractChecksumAndLifecycleStep(addonWorkflowName string) (string, addonv1.LifecycleStep, error) {
	// addonWorkflowName is of the form <addon-name>-<lifecycle>-<checksum>-wf
	// e.g. my-addon-prereqs-12345678-wf
	wfParts := strings.Split(addonWorkflowName, "-")
	if len(wfParts) < 4 || strings.TrimSpace(wfParts[len(wfParts)-1]) != "wf" {
		return "", "", fmt.Errorf("invalid workflow name %s", addonWorkflowName)
	}

	var checksum = strings.TrimSpace(wfParts[len(wfParts)-2])
	var lifecycle addonv1.LifecycleStep
	switch strings.TrimSpace(wfParts[len(wfParts)-3]) {
	case "prereqs":
		lifecycle = addonv1.Prereqs
	case "install":
		lifecycle = addonv1.Install
	case "validate":
		lifecycle = addonv1.Validate
	case "delete":
		lifecycle = addonv1.Delete
	default:
		return "", "", fmt.Errorf("invalid lifecycle in workflow name %s", addonWorkflowName)
	}

	return checksum, lifecycle, nil
}
