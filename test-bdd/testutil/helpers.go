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
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/keikoproj/addon-manager/pkg/common"
)

// PathToOSFile takes a relatice path and returns the full path on the OS
func PathToOSFile(relativPath string) (*os.File, error) {
	path, err := filepath.Abs(relativPath)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed generate absolut file path of %s", relativPath))
	}

	manifest, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to open file %s", path))
	}

	return manifest, nil
}

// KubectlApply runs 'kubectl apply -f <path>' with the given path
func KubectlApply(manifestRelativePath string) error {
	kubectlBinaryPath, err := exec.LookPath("kubectl")
	if err != nil {
		panic(err)
	}

	path, err := filepath.Abs(manifestRelativePath)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed generate absolut file path of %s", manifestRelativePath))
	}

	applyArgs := []string{"apply", "-f", path}
	cmd := exec.Command(kubectlBinaryPath, applyArgs...)
	fmt.Printf("Executing: %v %v", kubectlBinaryPath, applyArgs)

	err = cmd.Start()
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Could not exec kubectl: "))
	}

	err = cmd.Wait()
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Command resulted in error: "))
	}

	return nil
}

func isNodeReady(n corev1.Node) bool {
	for _, condition := range n.Status.Conditions {
		if condition.Type == "Ready" {
			if condition.Status == "True" {
				return true
			}
		}
	}
	return false
}

// ConcatonateList joins lists to strings delimited with `delimiter`
func ConcatonateList(list []string, delimiter string) string {
	return strings.Trim(strings.Join(strings.Fields(fmt.Sprint(list)), delimiter), "[]")
}

// ReadFile reads the raw content from a file path
func ReadFile(path string) ([]byte, error) {
	f, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Printf("failed to read file %v", path)
		return nil, err
	}
	return f, nil
}

// CRDExists returns true if a schema with the given name was found
func CRDExists(kubeClient dynamic.Interface, name string) bool {
	ctx := context.TODO()
	CRDSchema := common.CRDGVR()
	_, err := kubeClient.Resource(CRDSchema).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		fmt.Println(err)
		return false
	}
	return true
}

// ParseCustomResourceYaml parsed the YAMl for a CRD into an Unstructured object
func ParseCustomResourceYaml(raw string) (*unstructured.Unstructured, error) {
	var err error
	cr := unstructured.Unstructured{}
	data := []byte(raw)
	err = yaml.Unmarshal(data, &cr.Object)
	if err != nil {
		fmt.Println(err)
		return &cr, err
	}
	return &cr, nil
}
