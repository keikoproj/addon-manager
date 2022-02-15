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
	"fmt"
	"log"
	"net"
	"os"
	"time"

	wfv1versioned "github.com/argoproj/argo-workflows/v3/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
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
		fmt.Printf("error while creating wfv1 client %v ", err)
		return nil
	}
	return cli
}

// return cluster config
func InClusterConfig() (*rest.Config, error) {
	k8shost := "KUBERNETES_SERVICE_HOST"
	k8sport := "KUBERNETES_SERVICE_PORT"
	if len(os.Getenv(k8shost)) == 0 {
		addrs, err := net.LookupHost("kubernetes.default.svc")
		if err != nil {
			log.Fatalf(err.Error())
		}
		os.Setenv(k8shost, addrs[0])
	}
	if len(os.Getenv(k8sport)) == 0 {
		os.Setenv(k8sport, "443")
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// NewK8sClient defines kubernetes client
func NewK8sClient(kubeconfigPath string) (kubernetes.Interface, error) {
	if kubeconfigPath == "" {
		return nil, fmt.Errorf("kubeconfig should be configured a valid value")
	}
	var kubeconfig *rest.Config
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("unable to load kubeconfig from %s: %v", kubeconfigPath, err)
	}
	kubeconfig = config

	client, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create a client: %v", err)
	}
	return client, nil
}
