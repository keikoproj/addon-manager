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

package main_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/keikoproj/addon-manager/pkg/common"
	"github.com/keikoproj/addon-manager/test-bdd/testutil"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	apiextcs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func TestLoad(t *testing.T) {
	RegisterFailHandler(Fail)
	junitReporter := reporters.NewJUnitReporter("../test-bdd/junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Addon Type LoadTests", []Reporter{junitReporter})
}

var _ = Describe("Large Volume Addons should only use limited CPU and Memory.", func() {
	cfg := config.GetConfigOrDie()

	dynClient := dynamic.NewForConfigOrDie(cfg)
	extClient := apiextcs.NewForConfigOrDie(cfg)

	var addonName string
	var addonNamespace string
	var relativeAddonPath = "../docs/examples/eventrouter.yaml"
	var addonGroupSchema = common.AddonGVR()

	// Setup CRD
	It("should create CRD", func() {
		crdsRoot := "../config/crd/bases"
		files, err := ioutil.ReadDir(crdsRoot)
		if err != nil {
			Fail(fmt.Sprintf("failed to read crd path. %v", err))
		}

		for _, file := range files {
			err = testutil.CreateCRD(extClient, fmt.Sprintf("%s/%s", crdsRoot, file.Name()))
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("should create an Addon object", func() {
		ctx := context.TODO()
		s, e := os.Getenv("LOADTEST_START_NUMBER"), os.Getenv("LOADTEST_END_NUMBER")
		x, _ := strconv.Atoi(s)
		y, _ := strconv.Atoi(e)
		fmt.Printf("start = %d end = %d", x, y)

		var wg sync.WaitGroup
		lock := &sync.Mutex{}
		numberOfRoutines := 4
		wg.Add(numberOfRoutines)
		for i := 1; i <= numberOfRoutines; i++ {
			go func(i int, lock *sync.Mutex) {
				for j := i * 1000; j < i*1000+200; j++ {
					addon, err := testutil.CreateLoadTestsAddon(lock, dynClient, relativeAddonPath, fmt.Sprintf("-%d", i))
					Expect(err).NotTo(HaveOccurred())

					addonName = addon.GetName()
					addonNamespace = addon.GetNamespace()
					Eventually(func() map[string]interface{} {
						a, _ := dynClient.Resource(addonGroupSchema).Namespace(addonNamespace).Get(ctx, addonName, metav1.GetOptions{})
						return a.UnstructuredContent()
					}, 600*600*600).Should(HaveKey("status"))
				}
			}(i, lock)
		}
		wg.Wait()
	})

})
