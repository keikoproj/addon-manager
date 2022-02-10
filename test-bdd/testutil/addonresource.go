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
	"context"
	"io"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"

	"github.com/keikoproj/addon-manager/pkg/common"
)

var addonGroupSchema = common.AddonGVR()

// CreateAddon parses the raw data from the path into an Unstructured object (Addon) and submits and returns that object
func CreateAddon(kubeClient dynamic.Interface, relativePath string, nameSuffix string) (*unstructured.Unstructured, error) {
	ctx := context.TODO()
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
	addonObject, err := kubeClient.Resource(addonGroupSchema).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})

	if err == nil {
		resourceVersion := addonObject.GetResourceVersion()
		addon.SetResourceVersion(resourceVersion)
		_, err = kubeClient.Resource(addonGroupSchema).Namespace(namespace).Update(ctx, addon, metav1.UpdateOptions{})
		if err != nil {
			return addon, err
		}

	} else {
		_, err = kubeClient.Resource(addonGroupSchema).Namespace(namespace).Create(ctx, addon, metav1.CreateOptions{})
		if err != nil {
			return addon, err
		}
	}
	return addon, nil
}

// DeleteAddon deletes the Addon using the name and namespace parsed from the raw data at the given path
func DeleteAddon(kubeClient dynamic.Interface, relativePath string) (*unstructured.Unstructured, error) {
	ctx := context.TODO()
	addon, err := parseAddonYaml(relativePath)
	if err != nil {
		return addon, err
	}
	name := addon.GetName()
	namespace := addon.GetNamespace()

	if err := kubeClient.Resource(addonGroupSchema).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
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

func CreateLoadTestsAddon(lock *sync.Mutex, kubeClient dynamic.Interface, relativePath string, nameSuffix string) (*unstructured.Unstructured, error) {
	lock.Lock()
	defer lock.Unlock()

	ctx := context.TODO()
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
	addonObject, err := kubeClient.Resource(addonGroupSchema).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})

	pkgName, pkgVersion, ns := name, "v"+nameSuffix, "addon-event-router-ns"+nameSuffix
	unstructured.SetNestedField(addon.UnstructuredContent(), pkgName, "spec", "pkgName")
	unstructured.SetNestedField(addon.UnstructuredContent(), pkgVersion, "spec", "pkgVersion")
	unstructured.SetNestedField(addon.UnstructuredContent(), ns, "spec", "params", "namespace")

	if err == nil {
		resourceVersion := addonObject.GetResourceVersion()
		addon.SetResourceVersion(resourceVersion)
		_, err = kubeClient.Resource(addonGroupSchema).Namespace(namespace).Update(ctx, addon, metav1.UpdateOptions{})
		if err != nil {
			return addon, err
		}

	} else {
		_, err = kubeClient.Resource(addonGroupSchema).Namespace(namespace).Create(ctx, addon, metav1.CreateOptions{})
		if err != nil {
			return addon, err
		}
	}
	return addon, nil
}
