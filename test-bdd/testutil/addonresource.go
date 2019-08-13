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
	"bytes"
	"io"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"

	"github.com/orkaproj/addon-manager/pkg/common"
)

var addonGroupSchema = common.AddonGVR()

// CreateAddon parses the raw data from the path into an Unstructured object (Addon) and submits and returns that object
func CreateAddon(kubeClient dynamic.Interface, relativePath string, nameSuffix string) (*unstructured.Unstructured, error) {
	addon, err := parseAddonYaml(relativePath)
	if err != nil {
		return addon, err
	}

	name := addon.GetName()
	namespace := addon.GetNamespace()

	if nameSuffix != "" {
		addon.SetName(name + nameSuffix)
		name = addon.GetName()
	}

	// make sure the addonGroupScheme is valid if failing
	addonObject, err := kubeClient.Resource(addonGroupSchema).Namespace(namespace).Get(name, metav1.GetOptions{})

	if err == nil {
		resourceVersion := addonObject.GetResourceVersion()
		addon.SetResourceVersion(resourceVersion)
		_, err = kubeClient.Resource(addonGroupSchema).Namespace(namespace).Update(addon, metav1.UpdateOptions{})
		if err != nil {
			return addon, err
		}

	} else {
		_, err = kubeClient.Resource(addonGroupSchema).Namespace(namespace).Create(addon, metav1.CreateOptions{})
		if err != nil {
			return addon, err
		}
	}
	return addon, nil
}

// DeleteAddon deletes the Addon using the name and namespace parsed from the raw data at the given path
func DeleteAddon(kubeClient dynamic.Interface, relativePath string) (*unstructured.Unstructured, error) {
	addon, err := parseAddonYaml(relativePath)
	if err != nil {
		return addon, err
	}
	name := addon.GetName()
	namespace := addon.GetNamespace()

	if err := kubeClient.Resource(addonGroupSchema).Namespace(namespace).Delete(name, &metav1.DeleteOptions{}); err != nil {
		return addon, err
	}

	return addon, nil
}

func parseAddonYaml(relativePath string) (*unstructured.Unstructured, error) {
	var err error

	var addon *unstructured.Unstructured

	if _, err = PathToOSFile(relativePath); err != nil {
		return nil, err
	}

	fileData, err := ReadFile(relativePath)
	if err != nil {
		return nil, err
	}

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(fileData), 100)
	for {
		var out unstructured.Unstructured
		err = decoder.Decode(&out)
		if err != nil {
			// this would indicate it's malformed YAML.
			break
		}

		if out.GetKind() == "Addon" {
			var marshaled []byte
			marshaled, err = out.MarshalJSON()
			json.Unmarshal(marshaled, &addon)
			break
		}
	}

	if err != io.EOF && err != nil {
		return nil, err
	}
	return addon, nil
}

// func WaitForAddonReadiness(k dynamic.Interface, namespace string, name string) bool {
// 	// poll every 20 seconds
// 	var pollingInterval = time.Second * 10
// 	// timeout after 24 occurences = 240 seconds = 4 minutes
// 	var timeoutCounter = 24
// 	var pollingCounter = 0

// 	for {
// 		out, _ := k.Resource(addonGroupSchema).Namespace(namespace).Get(name, metav1.GetOptions{})
// 		status := out.Object["status"].(map[string]interface{})
// 		lifecycle := status["lifecycle"].(map[string]interface{})

// 		log.Printf("Addon %v has lifecycle: %#v", name, lifecycle)
// 		if strings.ToLower(currentState) == "Succeeded" {
// 			return true
// 		}
// 		time.Sleep(pollingInterval)
// 		log.Println("Addon not ready yet, retrying")
// 		pollingCounter++
// 		if pollingCounter == timeoutCounter {
// 			break
// 		}
// 	}
// 	return false
// }

// func WaitForAddonDeletion(k dynamic.Interface, namespace string, name string) bool {
// 	// poll every 20 seconds
// 	var pollingInterval = time.Second * 10
// 	// timeout after 24 occurences = 240 seconds = 4 minutes
// 	var timeoutCounter = 24
// 	var pollingCounter = 0

// 	for {
// 		_, err := k.Resource(addonGroupSchema).Namespace(namespace).Get(name, metav1.GetOptions{})
// 		if errors.IsNotFound(err) {
// 			return true
// 		}
// 		time.Sleep(pollingInterval)
// 		log.Println("InstanceGroup still exists, retrying")
// 		pollingCounter++
// 		if pollingCounter == timeoutCounter {
// 			break
// 		}
// 	}
// 	return false
// }
