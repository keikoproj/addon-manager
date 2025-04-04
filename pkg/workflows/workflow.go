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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	"github.com/keikoproj/addon-manager/api/addon"
	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"

	wfclientset "github.com/argoproj/argo-workflows/v3/pkg/client/clientset/versioned"
)

const (
	WfInstanceIdLabelKey           = "workflows.argoproj.io/controller-instanceid"
	WfInstanceId                   = "addon-manager-workflow-controller"
	WfDefaultActiveDeadlineSeconds = 300
)

// AddonLifecycle represents the following workflows
type AddonLifecycle interface {
	Install(context.Context, *WorkflowProxy) error
	Delete(context.Context, string) error
}

type WorkflowProxy struct {
	name      string
	template  *addonmgrv1alpha1.WorkflowType
	lifecycle addonmgrv1alpha1.LifecycleStep
}

func NewWorkflowProxy(name string, wt *addonmgrv1alpha1.WorkflowType, lifecycle addonmgrv1alpha1.LifecycleStep) *WorkflowProxy {
	return &WorkflowProxy{
		name:      name,
		template:  wt,
		lifecycle: lifecycle,
	}
}

type workflowLifecycle struct {
	dynClient dynamic.Interface
	addon     *addonmgrv1alpha1.Addon
	recorder  record.EventRecorder
	scheme    *runtime.Scheme

	wfclientset wfclientset.Interface
	wfinformer  cache.SharedIndexInformer

	log logr.Logger
}

// NewWorkflowLifecycle returns a AddonLifecycle object
func NewWorkflowLifecycle(wfclientset wfclientset.Interface, wfinformer cache.SharedIndexInformer, dynClient dynamic.Interface, addon *addonmgrv1alpha1.Addon, scheme *runtime.Scheme, recorder record.EventRecorder,
	log logr.Logger) AddonLifecycle {
	return &workflowLifecycle{
		dynClient:   dynClient,
		addon:       addon,
		scheme:      scheme,
		recorder:    recorder,
		wfclientset: wfclientset,
		wfinformer:  wfinformer,
		log:         log,
	}
}

func (w *workflowLifecycle) Install(ctx context.Context, wp *WorkflowProxy) error {
	wf := &unstructured.Unstructured{}
	err := w.parse(wp.template, wf, wp.name)
	if err != nil {
		return fmt.Errorf("invalid workflow. %v", err)
	}

	if !w.configureGlobalWFParameters(w.addon, wf) {
		return errors.New("invalid workflow parameter")
	}

	err = w.configureWorkflowArtifacts(wf, wp.template)
	if err != nil {
		return err
	}

	if err := w.injectTTLs(wf); err != nil {
		return err
	}

	if err := w.injectActiveDeadlineSeconds(wf); err != nil {
		return err
	}

	w.injectInstanceId(wf)
	w.addDefaultLabelsToResource(wf)

	return w.submit(ctx, wf)
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

func (w *workflowLifecycle) submit(ctx context.Context, wp *unstructured.Unstructured) error {
	wfName := types.NamespacedName{Name: wp.GetName(), Namespace: wp.GetNamespace()}

	// Create the Workflow
	// Convert proxy to workflow object
	wfv1, err := common.WorkFlowFromUnstructured(wp)
	if err != nil {
		return err
	}

	// Set the owner references for workflow
	if err := controllerutil.SetControllerReference(w.addon, wfv1, w.scheme); err != nil {
		return err
	}

	wfv1, err = w.wfclientset.ArgoprojV1alpha1().Workflows(wp.GetNamespace()).Create(ctx, wfv1, metav1.CreateOptions{})
	if err != nil && apierrors.IsAlreadyExists(err) {
		w.log.Info("Workflow already exists.", "workflow", wfName)
		return nil
	} else if err != nil {
		msg := fmt.Sprintf("failed creating wf %s err %v", wp.GetName(), err)
		w.log.Error(err, msg)
		return fmt.Errorf("%s", msg)
	}
	// Record an event for created workflow
	w.recorder.Event(w.addon, "Normal", "Created", fmt.Sprintf("Created Workflow %s", wfName))

	return nil
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
	labels[addon.ResourceDefaultOwnLabel] = w.addon.Name
	labels[addon.ResourceDefaultVersionLabel] = packageSpec.PkgVersion
	labels[addon.ResourceDefaultPartLabel] = w.addon.Name
	labels[addon.ResourceDefaultManageByLabel] = addon.Group

	resource.SetLabels(labels)
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
