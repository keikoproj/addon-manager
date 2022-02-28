/*
Copyright 2016 Skippbox, Ltd.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonv1versioned "github.com/keikoproj/addon-manager/pkg/client/clientset/versioned"
)

type Config struct {
	Namespace string

	DynCli     dynamic.Interface
	KubeClient kubernetes.Interface
	client.Client
	Scheme *runtime.Scheme

	ctx context.Context

	addoncli addonv1versioned.Interface
}
