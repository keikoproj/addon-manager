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

package v1alpha1

import (
	"fmt"
	"hash/adler32"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
)

// ClusterContext represents a minimal context that can be provided to an addon
type ClusterContext struct {
	// ClusterName name of the cluster
	// +optional
	ClusterName string `json:"clusterName,omitempty"`
	// ClusterRegion region of the cluster
	// +optional
	ClusterRegion string `json:"clusterRegion,omitempty"`
	// AdditionalConfigs are a map of string values that correspond to additional context data that can be passed along
	// +optional
	AdditionalConfigs map[string]FlexString `json:"additionalConfigs,omitempty" protobuf:"bytes,2,rep,name=data"`
}

// AddonParams are the parameters which will be available to the template workflows
type AddonParams struct {
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace,omitempty"`
	// Context values passed directly to the addon
	// +optional
	Context ClusterContext `json:"context,omitempty"`
	// Data values that will be parameters injected into workflows
	// +optional
	Data map[string]FlexString `json:"data,omitempty"`
}

// FlexString is a ptr to string type that is used to provide additional configs
type FlexString string

// UnmarshalJSON overrides unmarshaler.UnmarshalJSON in converter
func (fs *FlexString) UnmarshalJSON(b []byte) error {
	var out string
	if b[0] == '"' {
		if err := json.Unmarshal(b, &out); err != nil {
			return err
		}
		*fs = FlexString(out)
		return nil
	}

	var bo bool
	if err := json.Unmarshal(b, &bo); err == nil {
		if bo == true {
			out = "true"
		} else {
			out = "false"
		}
		*fs = FlexString(out)
		return nil
	}

	var i int
	if err := json.Unmarshal(b, &i); err == nil {
		out = strconv.Itoa(i)
		*fs = FlexString(out)
		return nil
	}

	return fmt.Errorf("unable to unmarshal from bool or int into string")
}

// KustomizeTemplate is used to specify override patch templates in Kustomize format
type KustomizeTemplate struct {
	// Template patch yamls as per Kustomize spec
	// +optional
	Template map[string]string `json:"template,omitempty" protobuf:"bytes,2,rep,name=template"`
}

// KustomizeSpec is used to specify common Kustomize spec features
type KustomizeSpec struct {
	// Common labels as per Kustomize spec
	// +optional
	Labels map[string]string `json:"labels,omitempty" protobuf:"bytes,1,rep,name=labels"`

	// Common annotations as per Kustomize spec
	// +optional
	Annotations map[string]string `json:"annotations,omitempty" protobuf:"bytes,1,rep,name=annotations"`

	// List of resource kinds
	// +optional
	Resources []string `json:"resources,omitempty"`

	// Overlay templates, these are patch objects as per kustomize spec
	// +optional
	Overlay KustomizeTemplate `json:"overlay,omitempty"`
}

// PackageType is a specific deployer type that will be used for deploying templates
type PackageType string

const (
	// HelmPkg is a deployer package type representing Helm package structure
	HelmPkg PackageType = "helm"
	// ShipPkg is a deployer package type representing Ship package structure
	ShipPkg PackageType = "ship"
	// KustomizePkg is a deployer package type representing Kustomize package structure
	KustomizePkg PackageType = "kustomize"
	// CnabPkg is a deployer package type representing CNAB package structure
	CnabPkg PackageType = "cnab"
	// CompositePkg is a package type representing a composite package structure, just yamls
	CompositePkg PackageType = "composite"
	// Error used to indicate system error
	Error ApplicationAssemblyPhase = "error"
)

// CmdType represents a function that can be performed with arguments
type CmdType int

const (
	cert CmdType = iota
	random
)

// ApplicationAssemblyPhase tracks the Addon CRD phases: pending, succeeded, failed, deleting, deleteFailed
type ApplicationAssemblyPhase string

// Constants
const (
	// Pending Used to indicate that not all of application's components have been deployed yet.
	Pending ApplicationAssemblyPhase = "Pending"
	// Succeeded Used to indicate that all of application's components have already been deployed.
	Succeeded ApplicationAssemblyPhase = "Succeeded"
	// Failed Used to indicate that deployment of application's components failed. Some components
	// might be present, but deployment of the remaining ones will not be re-attempted.
	Failed ApplicationAssemblyPhase = "Failed"
	// DepPending indicates Dep package is being pending
	DepPending ApplicationAssemblyPhase = "DepPending"
	// ValidationFailed Used to indicate validation failed: dep package not installed at all
	ValidationFailed ApplicationAssemblyPhase = "Validation Failed"
	// Deleting Used to indicate that all application's components are being deleted.
	Deleting ApplicationAssemblyPhase = "Deleting"
	// DeleteFailed Used to indicate that delete failed.
	DeleteFailed ApplicationAssemblyPhase = "Delete Failed"
	// Running indicating associated wf is running
	Running ApplicationAssemblyPhase = "Running"
)

// DeploymentPhase represents the status of observed resources
type DeploymentPhase string

const (
	// InProgress deployment phase for resources in addon
	InProgress DeploymentPhase = "InProgress"
	// Ready deployment phase for resources in addon
	Ready DeploymentPhase = "Ready"
	// Unknown deployment phase for resources in addon
	Unknown DeploymentPhase = "Unknown"
)

// LifecycleStep is a string representation of the lifecycle steps available in Addon spec: prereqs, install, delete, validate
type LifecycleStep string

const (
	// Prereqs constant
	Prereqs LifecycleStep = "prereqs"
	// Install constant
	Install LifecycleStep = "install"
	// Delete constant
	Delete LifecycleStep = "delete"
	// Validate constant
	Validate LifecycleStep = "validate"
)

// AddonOverridesSpec represents a template of the resources that can be deployed or patched alongside the main deployment
type AddonOverridesSpec struct {
	// Kustomize specs
	// +optional
	Kustomize KustomizeSpec `json:"kustomize,omitempty"`
	// Template specs
	// +optional
	Template map[string]string `json:"template,omitempty" protobuf:"bytes,2,rep,name=template"`
}

// SecretCmdSpec is a secret list and/or generator for secrets using the available commands: random, cert.
type SecretCmdSpec struct {
	Name string   `json:"name"`
	Cmd  CmdType  `json:"cmd,omitempty"`
	Args []string `json:"args,omitempty" protobuf:"bytes,4,rep,name=args"`
}

// WorkflowType allows user to specify workflow templates with optional namePrefix, workflowRole or role.
type WorkflowType struct {
	// NamePrefix is a prefix for the name of workflow
	// +kubebuilder:validation:MaxLength=10
	// +optional
	NamePrefix string `json:"namePrefix,omitempty"`
	// Role used to denote the role annotation that should be used by the deployment resource
	// +optional
	Role string `json:"role,omitempty"`
	// WorkflowRole used to denote the role annotation that should be used by the workflow
	// +optional
	WorkflowRole string `json:"workflowRole,omitempty"`
	// Template is used to provide the workflow spec
	Template string `json:"template"`
}

// LifecycleWorkflowSpec is where all of the lifecycle workflow templates will be specified under
type LifecycleWorkflowSpec struct {
	Prereqs  WorkflowType `json:"prereqs,omitempty"`
	Install  WorkflowType `json:"install,omitempty"`
	Delete   WorkflowType `json:"delete,omitempty"`
	Validate WorkflowType `json:"validate,omitempty"`
}

// PackageSpec is the package level details needed by addon
type PackageSpec struct {
	PkgChannel     string            `json:"pkgChannel,omitempty"`
	PkgName        string            `json:"pkgName"`
	PkgVersion     string            `json:"pkgVersion"`
	PkgType        PackageType       `json:"pkgType"`
	PkgDescription string            `json:"pkgDescription"`
	PkgDeps        map[string]string `json:"pkgDeps,omitempty"`
}

// AddonSpec defines the desired state of Addon
type AddonSpec struct {
	PackageSpec `json:",inline"`

	// Parameters that will be injected into the workflows for addon
	// +optional
	Params AddonParams `json:"params,omitempty"`
	// Selector that is used to filter the resource watching
	// +optional
	Selector metav1.LabelSelector `json:"selector,omitempty"`
	// Overrides are kustomize patches that can be applied to templates
	// +optional
	Overrides AddonOverridesSpec `json:"overrides,omitempty"`
	// Secrets is a list of secret names expected to exist in the target namespace
	// +optional
	Secrets []SecretCmdSpec `json:"secrets,omitempty"`

	// +optional
	Lifecycle LifecycleWorkflowSpec `json:"lifecycle,omitempty"`
}

// AddonStatusLifecycle defines the lifecycle status for steps.
type AddonStatusLifecycle struct {
	Prereqs   ApplicationAssemblyPhase `json:"prereqs,omitempty"`
	Installed ApplicationAssemblyPhase `json:"installed,omitempty"`
}

// ObjectStatus is a generic status holder for objects
// +k8s:deepcopy-gen=true
type ObjectStatus struct {
	// Link to object
	Link string `json:"link,omitempty"`
	// Name of object
	Name string `json:"name,omitempty"`
	// Kind of object
	Kind string `json:"kind,omitempty"`
	// Object group
	Group string `json:"group,omitempty"`
	// Status. Values: InProgress, Ready, Unknown
	Status string `json:"status,omitempty"`
}

// AddonStatus defines the observed state of Addon
type AddonStatus struct {
	Checksum  string               `json:"checksum"`
	Lifecycle AddonStatusLifecycle `json:"lifecycle"`
	Resources []ObjectStatus       `json:"resources"`
	Reason    string               `json:"reason"`
	StartTime int64                `json:"starttime"`
}

// +kubebuilder:object:root=true

// Addon is the Schema for the addons API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=addons
// +kubebuilder:printcolumn:name="PACKAGE",type="string",JSONPath=".spec.pkgName"
// +kubebuilder:printcolumn:name="VERSION",type="string",JSONPath=".spec.pkgVersion"
// +kubebuilder:printcolumn:name="STATUS",type="string",JSONPath=".status.lifecycle.installed"
// +kubebuilder:printcolumn:name="REASON",type="string",JSONPath=".status.reason"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Addon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AddonSpec   `json:"spec,omitempty"`
	Status AddonStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// AddonList contains a list of Addon
type AddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Addon `json:"items"`
}

// GetPackageSpec returns the addon package details from addon spec
func (a *Addon) GetPackageSpec() PackageSpec {
	return PackageSpec{
		PkgName:        a.Spec.PkgName,
		PkgVersion:     a.Spec.PkgVersion,
		PkgDeps:        a.Spec.PkgDeps,
		PkgChannel:     a.Spec.PkgChannel,
		PkgDescription: a.Spec.PkgDescription,
		PkgType:        a.Spec.PkgType,
	}
}

// GetAllAddonParameters returns an object copying the params submitted as part of the addon spec
func (a *Addon) GetAllAddonParameters() map[string]string {
	params := make(map[string]string)
	params["namespace"] = a.Spec.Params.Namespace
	params["clusterName"] = a.Spec.Params.Context.ClusterName
	params["clusterRegion"] = a.Spec.Params.Context.ClusterRegion
	for k, v := range a.Spec.Params.Context.AdditionalConfigs {
		params[k] = string(v)
	}
	for k, v := range a.Spec.Params.Data {
		params[k] = string(v)
	}
	return params
}

// GetWorkflowType returns the WorkflowType under the addon lifecycle spec
func (a *Addon) GetWorkflowType(step LifecycleStep) (*WorkflowType, error) {
	var wt *WorkflowType
	switch step {
	case Install:
		wt = &a.Spec.Lifecycle.Install
	case Prereqs:
		wt = &a.Spec.Lifecycle.Prereqs
	case Delete:
		wt = &a.Spec.Lifecycle.Delete
	case Validate:
		wt = &a.Spec.Lifecycle.Validate
	default:
		return nil, fmt.Errorf("no WorkflowType of type %s exists", step)
	}

	return wt, nil
}

// GetFormattedWorkflowName used the addon name, workflow prefix, addon checksum, and lifecycle step to compose the workflow name
func (a *Addon) GetFormattedWorkflowName(lifecycleStep LifecycleStep) string {
	wt, err := a.GetWorkflowType(lifecycleStep)
	if err != nil {
		return ""
	}

	prefix := a.GetName()
	if wt.NamePrefix != "" {
		prefix = fmt.Sprintf("%s-%s", prefix, wt.NamePrefix)
	}
	wfIdentifierName := fmt.Sprintf("%s-%s-%s-wf", prefix, lifecycleStep, a.CalculateChecksum())
	return wfIdentifierName
}

// CalculateChecksum converts the AddonSpec into a hash string (using Alder32 algo)
func (a *Addon) CalculateChecksum() string {
	return fmt.Sprintf("%x", adler32.Checksum([]byte(fmt.Sprintf("%+v", a.Spec))))
}

// GetInstallStatus returns the install phase for addon
func (a *Addon) GetInstallStatus() ApplicationAssemblyPhase {
	return a.Status.Lifecycle.Installed
}

func (p ApplicationAssemblyPhase) Succeeded() bool {
	switch p {
	case Succeeded:
		return true
	default:
		return false
	}
}

func (p ApplicationAssemblyPhase) Completed() bool {
	switch p {
	case Succeeded, Failed, Error:
		return true
	default:
		return false
	}
}

func (p ApplicationAssemblyPhase) Processing() bool {
	switch p {
	case Pending, Running:
		return true
	default:
		return false
	}
}

func (p ApplicationAssemblyPhase) Deleting() bool {
	switch p {
	case Deleting:
		return true
	default:
		return false
	}
}

func (p ApplicationAssemblyPhase) DepPending() bool {
	switch p {
	case DepPending, ValidationFailed:
		return true
	default:
		return false
	}
}
