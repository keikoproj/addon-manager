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

package main

import (
	"context"
	"flag"

	"github.com/keikoproj/addon-manager/controllers"
	"github.com/keikoproj/addon-manager/pkg/common"
	"github.com/keikoproj/addon-manager/pkg/utils"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	debug                bool
	metricsAddr          string
	enableLeaderElection bool
)

func init() {
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&debug, "debug", false, "Debug logging")
	flag.Parse()
}

func main() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(debug)))

	var kubeClient kubernetes.Interface
	var cfg *rest.Config
	cfg, err := rest.InClusterConfig()
	if err != nil {
		kubeClient = utils.GetClientOutOfCluster()
	} else {
		kubeClient = utils.GetClient()
	}

	dynCli, err := dynamic.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	wfcli := common.NewWFClient(cfg)
	if wfcli == nil {
		panic("workflow client could not be nil")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addoncli := common.NewAddonClient(cfg)
	controllers.Start(ctx, "addon-manager-system", kubeClient, dynCli, addoncli, wfcli)
}
