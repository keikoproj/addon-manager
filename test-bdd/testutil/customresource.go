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

package testutil

import (
	"context"
	"io"
	"os"

	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextcs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// CreateCRD creates the CRD parsed from the path given
func CreateCRD(kubeClient apiextcs.Interface, relativePath string) error {
	ctx := context.TODO()
	CRD, err := parseCRDYaml(relativePath)
	if err != nil {
		return err
	}

	_, err = kubeClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, CRD.Name, metav1.GetOptions{})

	if err == nil {
		err = KubectlApply(relativePath)
		if err != nil {
			return err
		}

	} else {
		_, err = kubeClient.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, CRD, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteCRD deletes the CRD parsed from the path given
func DeleteCRD(kubeClient apiextcs.Interface, relativePath string) error {
	ctx := context.TODO()
	CRD, err := parseCRDYaml(relativePath)
	if err != nil {
		return err
	}

	if err := kubeClient.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, CRD.Name, metav1.DeleteOptions{}); err != nil {
		return err
	}

	return nil
}

func parseCRDYaml(relativePath string) (*apiextensions.CustomResourceDefinition, error) {
	var manifest *os.File
	var err error

	var crd apiextensions.CustomResourceDefinition
	if manifest, err = PathToOSFile(relativePath); err != nil {
		return nil, err
	}

	decoder := yaml.NewYAMLOrJSONDecoder(manifest, 100)
	for {
		var out unstructured.Unstructured
		err = decoder.Decode(&out)
		if err != nil {
			// this would indicate it's malformed YAML.
			break
		}

		if out.GetKind() == "CustomResourceDefinition" {
			var marshaled []byte
			marshaled, err = out.MarshalJSON()
			json.Unmarshal(marshaled, &crd)
			break
		}
	}

	if err != io.EOF && err != nil {
		return nil, err
	}
	return &crd, nil
}
