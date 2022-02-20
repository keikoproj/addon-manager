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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/pkg/apis/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"
)

const (
	WfInstanceIdLabelKey           = "workflows.argoproj.io/controller-instanceid"
	WfInstanceId                   = "addon-manager-workflow-controller"
	WfDefaultActiveDeadlineSeconds = 300
)

// AddonLifecycle represents the following workflows
type AddonLifecycle interface {
	Install(context.Context, *addonmgrv1alpha1.WorkflowType, string) (addonmgrv1alpha1.ApplicationAssemblyPhase, error)
	Delete(context.Context, string) error
}

type workflowLifecycle struct {
	client.Client
	dynClient dynamic.Interface
	addon     *addonmgrv1alpha1.Addon
	recorder  record.EventRecorder
	scheme    *runtime.Scheme
}

// NewWorkflowLifecycle returns a AddonLifecycle object
func NewWorkflowLifecycle(client client.Client, dynClient dynamic.Interface, addon *addonmgrv1alpha1.Addon, scheme *runtime.Scheme, recorder record.EventRecorder) AddonLifecycle {
	return &workflowLifecycle{
		Client:    client,
		dynClient: dynClient,
		addon:     addon,
		scheme:    scheme,
		recorder:  recorder,
	}
}

func (w *workflowLifecycle) Install(ctx context.Context, wt *addonmgrv1alpha1.WorkflowType, name string) (addonmgrv1alpha1.ApplicationAssemblyPhase, error) {
	wp := &unstructured.Unstructured{}
	err := w.parse(wt, wp, name)
	if err != nil {
		return addonmgrv1alpha1.Failed, fmt.Errorf("invalid workflow. %v", err)
	}

	if !w.configureGlobalWFParameters(w.addon, wp) {
		return addonmgrv1alpha1.Failed, errors.New("invalid workflow parameter")
	}

	err = w.configureWorkflowArtifacts(wp, wt)
	if err != nil {
		return addonmgrv1alpha1.Failed, err
	}

	if err := w.injectTTLs(wp); err != nil {
		return addonmgrv1alpha1.Failed, err
	}

	if err := w.injectActiveDeadlineSeconds(wp); err != nil {
		return addonmgrv1alpha1.Failed, err
	}

	w.injectInstanceId(wp)

	return w.submit(ctx, wp)
}

// Appends addon.spec.params to workflow.spec.arguments.parameters
func (w *workflowLifecycle) configureGlobalWFParameters(addon *addonmgrv1alpha1.Addon, wf *unstructured.Unstructured) bool {
	// get workflow argument parameters
	spec, _ := wf.UnstructuredContent()["spec"].(map[string]interface{})
	if spec["arguments"] == nil {
		spec["arguments"] = make(map[string]interface{})
	}

	arguments := spec["arguments"].(map[string]interface{})
	if arguments["parameters"] == nil {
		arguments["parameters"] = make([]interface{}, 0)
	}

	wfParams := arguments["parameters"].([]interface{})
	if wfParams == nil {
		arguments["parameters"] = make([]interface{}, 0)
	}

	// get addon params
	namespaceParam := addon.Spec.Params.Namespace
	contextParams := addon.Spec.Params.Context
	dataParams := addon.Spec.Params.Data
	pkgParams := addon.Spec.PackageSpec

	namespaceMap := make(map[string]interface{})
	namespaceMap["name"] = "namespace"
	namespaceMap["value"] = namespaceParam

	wfParams = append(wfParams, namespaceMap)

	// Copy pkgParams into global workflow variables
	refPkg := reflect.ValueOf(pkgParams)
	for i := 0; i < refPkg.Type().NumField(); i++ {
		pkgParamMap := make(map[string]interface{})
		kind := refPkg.Field(i).Kind()
		if kind == reflect.String {
			tag := refPkg.Type().Field(i).Tag
			jsonTag := strings.Split(tag.Get("json"), ",")[0]
			value := refPkg.Field(i).String()
			pkgParamMap["name"] = jsonTag
			pkgParamMap["value"] = value
			wfParams = append(wfParams, pkgParamMap)
		}
	}

	// Copy general Context string params to global workflow variables (clusterName and clusterRegion currently)
	cp := reflect.ValueOf(contextParams)
	for i := 0; i < cp.Type().NumField(); i++ {
		contextMap := make(map[string]interface{})
		kind := cp.Field(i).Kind()
		if kind == reflect.String {
			fieldName := cp.Type().Field(i).Name
			tag := cp.Type().Field(i).Tag
			jsonTag := strings.Split(tag.Get("json"), ",")[0]
			contextMap["name"] = jsonTag
			contextMap["value"] = cp.FieldByName(fieldName).String()
			wfParams = append(wfParams, contextMap)
		}
	}

	// Copy AdditionalConfigs from Context to global workflow variables
	for name, value := range contextParams.AdditionalConfigs {
		addParam := make(map[string]interface{})
		addParam["name"] = name
		addParam["value"] = string(value)
		wfParams = append(wfParams, addParam)
	}

	// Copy stringParams to global workflow variables
	for name, value := range dataParams {
		addParam := make(map[string]interface{})
		addParam["name"] = name
		addParam["value"] = string(value)
		wfParams = append(wfParams, addParam)
	}

	err := unstructured.SetNestedSlice(wf.UnstructuredContent(), wfParams, "spec", "arguments", "parameters")
	if err != nil {
		return false
	}

	return true
}

func (w *workflowLifecycle) Delete(ctx context.Context, name string) error {
	err := w.dynClient.Resource(common.WorkflowGVR()).Namespace(w.addon.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (w *workflowLifecycle) findWorkflowByName(ctx context.Context, name types.NamespacedName) (*unstructured.Unstructured, error) {
	found := &unstructured.Unstructured{}
	found.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "Workflow",
		Group:   "argoproj.io",
		Version: "v1alpha1",
	})
	err := w.Get(ctx, name, found)
	if err != nil && apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return found, nil
}

func (w *workflowLifecycle) submit(ctx context.Context, wp *unstructured.Unstructured) (addonmgrv1alpha1.ApplicationAssemblyPhase, error) {
	var wfv1 *unstructured.Unstructured
	var err error

	// Check if the Workflow already exists
	wfv1, err = w.findWorkflowByName(ctx, types.NamespacedName{Name: wp.GetName(), Namespace: wp.GetNamespace()})
	if err != nil {
		return addonmgrv1alpha1.Failed, err
	}

	// Check if the same Addon spec was submitted and completed previously
	if wfv1 != nil {
		deleted, err := w.deleteCollisionWorkflows(ctx)
		if err != nil {
			return addonmgrv1alpha1.Failed, err
		}
		if deleted {
			return addonmgrv1alpha1.Pending, nil
		}
	}

	if wfv1 == nil {
		// Create the Workflow
		wfv1 = &unstructured.Unstructured{}

		// Convert proxy to workflow object
		err = w.scheme.Convert(wp, wfv1, 0)
		if err != nil {
			return addonmgrv1alpha1.Failed, err
		}
		wfv1.SetGroupVersionKind(schema.GroupVersionKind{
			Kind:    "Workflow",
			Group:   "argoproj.io",
			Version: "v1alpha1",
		})
		wfv1.SetNamespace(wp.GetNamespace())
		wfv1.SetName(wp.GetName())
		// Set the owner references for workflow
		if err := controllerutil.SetControllerReference(w.addon, wfv1, w.scheme); err != nil {
			return addonmgrv1alpha1.Failed, err
		}

		err = w.Create(ctx, wfv1)
		if err != nil {
			return addonmgrv1alpha1.Failed, err
		}
		// Record an event for created workflow
		w.recorder.Event(w.addon, "Normal", "Created", fmt.Sprintf("Created Workflow %s/%s", wp.GetName(), wp.GetNamespace()))

		return addonmgrv1alpha1.Pending, nil
	}

	workflow, err := w.dynClient.Resource(common.WorkflowGVR()).Namespace(wfv1.GetNamespace()).Get(ctx, wfv1.GetName(), metav1.GetOptions{})
	if err != nil {
		return addonmgrv1alpha1.Failed, fmt.Errorf("could not find workflow %s/%s. %v", wfv1.GetNamespace(), wfv1.GetName(), err)
	}

	// validate workflow status
	var phase = addonmgrv1alpha1.Pending
	status, ok := workflow.UnstructuredContent()["status"].(map[string]interface{})
	if ok && status["phase"] == "Succeeded" {
		phase = addonmgrv1alpha1.Succeeded
	} else if ok && status["phase"] == "Failed" {
		phase = addonmgrv1alpha1.Failed
	}

	return phase, nil
}

func (w *workflowLifecycle) parse(wt *addonmgrv1alpha1.WorkflowType, wf *unstructured.Unstructured, name string) error {
	var data map[string]interface{}

	// Load workflow spec into data obj
	if err := yaml.Unmarshal([]byte(wt.Template), &data); err != nil {
		return fmt.Errorf("invalid workflow yaml spec passed. %v", err)
	}

	// We need to marshal and unmarshal due to conversion issues.
	raw, err := json.Marshal(data)
	if err != nil {
		return errors.New("invalid workflow, unable to marshal data")
	}
	err = wf.UnmarshalJSON(raw)
	if err != nil {
		return errors.New("invalid workflow, unable to unmarshal to workflow")
	}

	// Enforce this to be a workflow type
	wf.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "Workflow",
		Group:   "argoproj.io",
		Version: "v1alpha1",
	})

	wf.SetNamespace(w.addon.GetNamespace())
	wf.SetName(name)

	if _, foundSpec, err := unstructured.NestedFieldNoCopy(wf.Object, "spec"); err != nil || !foundSpec {
		return errors.New("invalid workflow, missing spec")
	}

	return nil
}

func (w *workflowLifecycle) configureWorkflowArtifacts(wf *unstructured.Unstructured, wt *addonmgrv1alpha1.WorkflowType) error {
	spec, _, err := unstructured.NestedFieldNoCopy(wf.UnstructuredContent(), "spec")
	if err != nil {
		return err
	}

	// workflow.spec.arguments.artifacts may exist
	err = w.processWorkflowResources(spec, wt)
	if err != nil {
		return err
	}

	templates, _, err := unstructured.NestedFieldNoCopy(wf.UnstructuredContent(), "spec", "templates")
	if err != nil {
		return err
	}
	for _, template := range templates.([]interface{}) {
		// Process templates with resource
		err := w.processWorkflowResources(template, wt)
		if err != nil {
			return err
		}

		if allSteps, found, err := unstructured.NestedFieldNoCopy(template.(map[string]interface{}), "steps"); found {
			for _, steps := range allSteps.([]interface{}) {
				steps := steps.([]interface{})
				for _, step := range steps {
					err := w.processWorkflowResources(step, wt)
					if err != nil {
						return err
					}
				}
			}
		} else if err != nil {
			return err
		}
	}

	return nil
}

func (w *workflowLifecycle) processWorkflowResources(workflowStepObject interface{}, wt *addonmgrv1alpha1.WorkflowType) error {
	artifacts, foundArtifacts, err := unstructured.NestedFieldNoCopy(workflowStepObject.(map[string]interface{}), "arguments", "artifacts")
	if err != nil {
		return err
	}

	if foundArtifacts {
		for _, artifact := range artifacts.([]interface{}) {
			artifact := artifact.(map[string]interface{})
			data, _, err := unstructured.NestedString(artifact, "raw", "data")
			if err != nil {
				return err
			}

			var objs []string
			for _, obj := range strings.Split(data, "---\n") {
				resource := &unstructured.Unstructured{}
				data, err = w.processArtifact(obj, resource, wt)
				if err != nil {
					return err
				}
				objs = append(objs, data)
			}
			data = strings.Join(objs, "---\n")
			err = unstructured.SetNestedField(artifact, data, "raw", "data")
			if err != nil {
				return err
			}
		}
	} else {
		// Look for manifest resources
		manifests, foundManifests, err := unstructured.NestedFieldNoCopy(workflowStepObject.(map[string]interface{}), "resource", "manifest")
		if err != nil {
			return err
		}

		if foundManifests {
			var objs []string
			for _, obj := range strings.Split(manifests.(string), "---\n") {
				resource := &unstructured.Unstructured{}
				data, err := w.processArtifact(obj, resource, wt)
				if err != nil {
					return err
				}
				objs = append(objs, data)
			}
			manifests = strings.Join(objs, "---\n")
			err = unstructured.SetNestedField(workflowStepObject.(map[string]interface{}), manifests.(string), "resource", "manifest")
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (w *workflowLifecycle) processArtifact(obj string, resource *unstructured.Unstructured, wt *addonmgrv1alpha1.WorkflowType) (string, error) {
	obj = strings.TrimSpace(obj)
	if obj == "" {
		// Ignore empty manifest objects
		return obj, nil
	}
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(obj), &data); err != nil {
		return "", fmt.Errorf("unable to unmarshall artifact: %v. %v", obj, err)
	}

	resource.SetUnstructuredContent(data)

	// Add the default labels to the resource
	w.addDefaultLabelsToResource(resource)

	// Add the provided role annotation to the resource
	w.addRoleAnnotationToResource(resource, wt)

	appendData, err := yaml.Marshal(resource.UnstructuredContent())
	if err != nil {
		return "", fmt.Errorf("unable to marshall resource: %+v", resource)
	}

	return string(appendData), nil
}

func (w *workflowLifecycle) addDefaultLabelsToResource(resource *unstructured.Unstructured) {
	packageSpec := w.addon.GetPackageSpec()
	labels := resource.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}

	// Set default labels
	labels["app.kubernetes.io/name"] = w.addon.Name
	labels["app.kubernetes.io/version"] = packageSpec.PkgVersion
	labels["app.kubernetes.io/part-of"] = w.addon.Name
	labels["app.kubernetes.io/managed-by"] = common.AddonGVR().Group

	resource.SetLabels(labels)
}

func (w *workflowLifecycle) addRoleAnnotationToResource(resource *unstructured.Unstructured, wt *addonmgrv1alpha1.WorkflowType) {
	annotations := resource.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	if wt.Role != "" {
		// TODO change this role name to a config value
		annotations["iam.amazonaws.com/role"] = wt.Role
	}

	resource.SetAnnotations(annotations)
}

func (w *workflowLifecycle) deleteCollisionWorkflows(ctx context.Context) (bool, error) {
	var mostRecentWorkflowTime time.Time
	var mostRecentWorkflow unstructured.Unstructured
	var deleted = false

	workflows, err := w.dynClient.Resource(common.WorkflowGVR()).Namespace(w.addon.GetNamespace()).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list workflows. %v", err)
	}

	// Get the most recently run workflow for this addon
	for _, workflow := range workflows.Items {
		if strings.Contains(workflow.GetName(), w.addon.Name) {
			if workflow.UnstructuredContent()["status"] == nil {
				return false, nil
			}
			startedAt := workflow.UnstructuredContent()["status"].(map[string]interface{})["startedAt"].(string)
			t, err := time.Parse(time.RFC3339, startedAt)
			if err != nil {
				return false, err
			}
			if !t.Before(mostRecentWorkflowTime) {
				mostRecentWorkflowTime = t
				mostRecentWorkflow = workflow
			}
		}
	}

	if mostRecentWorkflow.Object == nil {
		return false, nil
	}

	// If the most recently run workflow doesn't have the current checksum, delete the old checksum workflows
	if !strings.Contains(mostRecentWorkflow.GetName(), w.addon.Status.Checksum) {
		for _, workflow := range workflows.Items {
			phase := workflow.UnstructuredContent()["status"].(map[string]interface{})["phase"].(string)
			if strings.Contains(workflow.GetName(), w.addon.Status.Checksum) && phase != "Pending" {
				_ = w.Delete(ctx, workflow.GetName())
				deleted = true
			}
		}
	}

	return deleted, nil
}

func (w *workflowLifecycle) injectTTLs(wf *unstructured.Unstructured) error {
	// Workflow spec ttlStrategy new in Argo Workflows v3.x
	//spec:
	//ttlStrategy:
	//secondsAfterCompletion: 10 # Time to live after workflow is completed, replaces ttlSecondsAfterFinished
	//secondsAfterSuccess: 5     # Time to live after workflow is successful
	//secondsAfterFailure: 5     # Time to live after workflow fails

	// Default ttl is to cleanup workflows after 3 days
	var ttl, _ = time.ParseDuration("72h")
	val, found, err := unstructured.NestedInt64(wf.UnstructuredContent(), "spec", "ttlStrategy", "secondsAfterCompletion")
	if err != nil {
		return err
	}

	// Make sure workflows by default get cleaned up after 3 days
	if !found || val == 0 {
		err = unstructured.SetNestedField(wf.Object, int64(ttl.Seconds()), "spec", "ttlStrategy", "secondsAfterCompletion")
		if err != nil {
			return err
		}
	}

	return nil
}

func (w *workflowLifecycle) injectInstanceId(wp *unstructured.Unstructured) {
	// Add instanceId labels to all workflows
	labels := wp.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}

	labels[WfInstanceIdLabelKey] = WfInstanceId

	wp.SetLabels(labels)
}

func (w *workflowLifecycle) injectActiveDeadlineSeconds(wf *unstructured.Unstructured) error {
	val, found, err := unstructured.NestedInt64(wf.UnstructuredContent(), "spec", "activeDeadlineSeconds")
	if err != nil {
		return err
	}

	if !found || val == 0 {
		err = unstructured.SetNestedField(wf.Object, int64(WfDefaultActiveDeadlineSeconds), "spec", "activeDeadlineSeconds")
		if err != nil {
			return err
		}
	}

	return nil
}

func IsValidV1WorkFlow(obj interface{}) error {
	wf, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("error : unstructured type.")
	}

	if wf.GetKind() != "Workflow" || wf.GetAPIVersion() != "argoproj.io/v1alpha1" {
		return fmt.Errorf("error : unsupported object from v1workflow update event %v", obj)
	}
	return nil
}

// extract addon-name and lifecyclestep from a workflow name string generated based on
// api types
func ExtractAddOnNameAndLifecycleStep(addonworkflowname string) (string, string, error) {
	if strings.Contains(addonworkflowname, "prereqs") {
		return strings.TrimSpace(addonworkflowname[:strings.Index(addonworkflowname, "prereqs")-1]), "prereqs", nil
	}

	if strings.Contains(addonworkflowname, "install") {
		return strings.TrimSpace(addonworkflowname[:strings.Index(addonworkflowname, "install")-1]), "install", nil

	}
	if strings.Contains(addonworkflowname, "delete") {
		return strings.TrimSpace(addonworkflowname[:strings.Index(addonworkflowname, "delete")-1]), "delete", nil
	}
	if strings.Contains(addonworkflowname, "validate") {
		return strings.TrimSpace(addonworkflowname[:strings.Index(addonworkflowname, "validate")-1]), "validate", nil
	}
	return "", "", fmt.Errorf("no recognized lifecyclestep within ")
}
