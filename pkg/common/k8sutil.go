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
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
)

func NewWorkflowInformer(dclient dynamic.Interface, ns string, resyncPeriod time.Duration) cache.SharedIndexInformer {
	var nsInformers = dynamicinformer.NewFilteredDynamicSharedInformerFactory(dclient, resyncPeriod, ns, nil)
	return nsInformers.ForResource(WorkflowGVR()).Informer()
}

// ToUnstructured converts an workflow to an Unstructured object
func ToUnstructured(wf *wfv1.Workflow) (*unstructured.Unstructured, error) {
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(wf)
	if err != nil {
		return nil, err
	}
	un := &unstructured.Unstructured{Object: obj}
	// we need to add these values so that the `EventRecorder` does not error
	un.SetKind("Workflow")
	un.SetAPIVersion("argoproj.io/v1alpha1")
	return un, nil
}
