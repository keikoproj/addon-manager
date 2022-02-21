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
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/keikoproj/addon-manager/pkg/common"
	"github.com/keikoproj/addon-manager/test-bdd/testutil"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	numberOfRoutines = 10
)

func main() {
	s, e := os.Getenv("LOADTEST_START_NUMBER"), os.Getenv("LOADTEST_END_NUMBER")
	x, _ := strconv.Atoi(s)
	y, _ := strconv.Atoi(e)
	fmt.Printf("start = %d end = %d", x, y)

	costsummary, err := os.Create("summary.txt")
	if err != nil {
		return
	}
	dataWriter := bufio.NewWriter(costsummary)

	stop := make(chan bool)
	mgrPid := os.Getenv("MANAGER_PID")
	ctrlPid := os.Getenv("WFCTRL_PID")
	go func(mgrPid, ctrlPid string, writer *bufio.Writer) {
		for {
			select {
			case <-stop:
				return
			default:
				fmt.Printf("\n every 2 minutes collecting data for mgr %s wfctrl %s", mgrPid, ctrlPid)
				Summary(mgrPid, ctrlPid, writer)
				time.Sleep(2 * time.Minute)
			}
		}
	}(mgrPid, ctrlPid, dataWriter)

	var wg sync.WaitGroup
	wg.Add(numberOfRoutines)
	lock := &sync.Mutex{}

	cfg := config.GetConfigOrDie()
	dynClient := dynamic.NewForConfigOrDie(cfg)
	ctx := context.TODO()
	var addonName string
	var addonNamespace string
	var relativeAddonPath = "docs/examples/eventrouter.yaml"
	var addonGroupSchema = common.AddonGVR()

	for i := 1; i <= numberOfRoutines; i++ {
		go func(i int, lock *sync.Mutex) {
			defer wg.Done()
			for j := i * 100; j < i*100+200; j++ {
				addon, err := testutil.CreateLoadTestsAddon(lock, dynClient, relativeAddonPath, fmt.Sprintf("-%d", j))
				if err != nil {
					fmt.Printf("\n\n create addon failure err %v", err)
					time.Sleep(1 * time.Second)
					continue
				}

				addonName = addon.GetName()
				addonNamespace = addon.GetNamespace()
				for x := 0; x <= 500; x++ {
					a, err := dynClient.Resource(addonGroupSchema).Namespace(addonNamespace).Get(ctx, addonName, metav1.GetOptions{})
					if a == nil || err != nil || a.UnstructuredContent()["status"] == nil {
						fmt.Printf("\n\n retry get addon status %v get ", err)
						time.Sleep(1 * time.Second)
						continue
					}
					break
				}
			}
		}(i, lock)
	}
	wg.Wait()
	stop <- true
	costsummary.Close()

}

// capture cpu/memory/addons number every 3 mintues
func Summary(managerPID, wfctrlPID string, datawriter *bufio.Writer) error {

	kubectlCmd := exec.Command("kubectl", "-n", "addon-manager-system", "get", "addons")
	wcCmd := exec.Command("wc", "-l")

	//make a pipe
	reader, writer := io.Pipe()
	var buf bytes.Buffer

	//set the output of "cat" command to pipe writer
	kubectlCmd.Stdout = writer
	//set the input of the "wc" command pipe reader

	wcCmd.Stdin = reader

	//cache the output of "wc" to memory
	wcCmd.Stdout = &buf

	//start to execute "cat" command
	kubectlCmd.Start()

	//start to execute "wc" command
	wcCmd.Start()

	//waiting for "cat" command complete and close the writer
	kubectlCmd.Wait()
	writer.Close()

	//waiting for the "wc" command complete and close the reader
	wcCmd.Wait()
	reader.Close()

	AddonsNum := buf.String()
	fmt.Printf("\n addons number %s\n", AddonsNum)

	//cmd = fmt.Sprintf("ps -p %s -o %%cpu,%%mem", managerPID)
	cmd := exec.Command("ps", "-p", managerPID, "-o", "%cpu,%mem")
	fmt.Printf("manager cpu cmd %v", *cmd)
	managerUsage, err := cmd.Output()
	if err != nil {
		fmt.Printf("failed to collect addonmanager cpu/mem usage. %v", err)
		return err
	}
	//fmt.Printf("\n addonmanager cpu/mem usage %s ", managerUsage)

	cmd = exec.Command("ps", "-p", wfctrlPID, "-o", "%cpu,%mem")
	wfControllerUsage, err := cmd.Output()
	if err != nil {
		fmt.Printf("failed to collect addonmanager cpu/mem usage. %v", err)
		return err
	}
	//fmt.Printf("workflow controller cpu/mem usage %s ", wfControllerUsage)
	fmt.Printf("addons number %s addonmanager cpu/mem usage %s controller cpu/mem usage %s", AddonsNum, managerUsage, wfControllerUsage)
	oneline := fmt.Sprintf("<addons-num:%s  \n addon-mgr:%s  \nwf-controller:%s>", strings.TrimSpace(AddonsNum), strings.TrimSuffix(string(managerUsage), "\n"), strings.TrimSuffix(string(wfControllerUsage), "\n"))
	datawriter.WriteString(oneline + "\n#############\n")
	datawriter.Flush()
	return nil
}
