package tests

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"

	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/keikoproj/addon-manager/pkg/common"
	"github.com/keikoproj/addon-manager/test-bdd/testutil"
	"github.com/keikoproj/addon-manager/test-load/pkg/cmd"
	"github.com/keikoproj/addon-manager/test-load/pkg/log"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type flagpole struct {
	managerPID         string // passed in kubeconfig
	wfcontrollerPID    string
	numberOfGoRoutines int
}

const (
	NumberOfWFPerRoutine = 200
)

// NewCommand ...
func NewCommand(logger log.Logger, streams cmd.IOStreams) *cobra.Command {
	flags := &flagpole{}
	cmd := &cobra.Command{
		Args:  cobra.NoArgs,
		Use:   "tests",
		Short: "tests load",
		Long:  "tests addon-manager loads",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunLoads(logger, flags)
		},
	}
	cmd.Flags().StringVar(&flags.managerPID, "managerPID", "", " addon manager pid")
	cmd.Flags().StringVar(&flags.wfcontrollerPID, "wfcontrollerPID", "", " workflow controller pid")
	cmd.Flags().IntVar(&flags.numberOfGoRoutines, "numberOfGoRoutines", 10, " number of goroutines to be created. Each deploy 200 workflows.")
	return cmd
}

func RunLoads(logger log.Logger, flags *flagpole) error {

	if flags.numberOfGoRoutines == 0 {
		fmt.Printf("please specify at least one goroutine.")
		flags.numberOfGoRoutines = 2
	}
	perfsummary, err := os.Create("summary.txt")
	if err != nil {
		return fmt.Errorf("failed creating summary file. %v", err)
	}
	dataWriter := bufio.NewWriter(perfsummary)

	stop := make(chan bool)
	go func(mgrPid, ctrlPid string, writer *bufio.Writer) {
		for {
			select {
			case <-stop:
				return
			default:
				fmt.Printf("\n every 2 minutes collecting mgr %s wfctrl %s %%cpu/%%mem ", mgrPid, ctrlPid)
				Summary(mgrPid, ctrlPid, writer)
				time.Sleep(2 * time.Minute)
			}
		}
	}(flags.managerPID, flags.wfcontrollerPID, dataWriter)

	var wg sync.WaitGroup
	wg.Add(flags.numberOfGoRoutines)
	lock := &sync.Mutex{}

	cfg := config.GetConfigOrDie()
	dynClient := dynamic.NewForConfigOrDie(cfg)
	ctx := context.TODO()
	var addonName string
	var addonNamespace string
	var relativeAddonPath = "docs/examples/eventrouter.yaml"
	var addonGroupSchema = common.AddonGVR()

	for i := 1; i <= flags.numberOfGoRoutines; i++ {
		go func(i int, lock *sync.Mutex) {
			defer wg.Done()
			for j := i * 100; j < i*100+int(NumberOfWFPerRoutine); j++ {
				addon, err := testutil.CreateLoadTestsAddon(lock, dynClient, relativeAddonPath, fmt.Sprintf("-%d", j))
				if err != nil {
					fmt.Printf("\n create addon failure for %v. retry 10 times after every 1 second. \n", err)
					time.Sleep(1 * time.Second)
					for i := 0; i < 10; i++ {
						addon, err = testutil.CreateLoadTestsAddon(lock, dynClient, relativeAddonPath, fmt.Sprintf("-%d", j))
						if err != nil || addon == nil {
							time.Sleep(1 * time.Second)
							continue
						}
						break
					}
				}

				addonName = addon.GetName()
				addonNamespace = addon.GetNamespace()
				for x := 0; x <= 500; x++ {
					a, err := dynClient.Resource(addonGroupSchema).Namespace(addonNamespace).Get(ctx, addonName, metav1.GetOptions{})
					if a == nil || err != nil || a.UnstructuredContent()["status"] == nil {
						fmt.Printf("\n\n addon is not readdy err <%v> retry after 1 second ", err)
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
	perfsummary.Close()
	return nil
}

// capture cpu/memory/addons number periodically
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
	fmt.Printf("\n atm total addons number %s\n", AddonsNum)

	cmd := exec.Command("ps", "-p", managerPID, "-o", "%cpu,%mem")
	fmt.Printf("manager cpu cmd %v", *cmd)
	managerUsage, err := cmd.Output()
	if err != nil {
		fmt.Printf("failed to collect addonmanager cpu/mem usage. %v", err)
		return err
	}

	cmd = exec.Command("ps", "-p", wfctrlPID, "-o", "%cpu,%mem")
	wfControllerUsage, err := cmd.Output()
	if err != nil {
		fmt.Printf("failed to collect addonmanager cpu/mem usage. %v", err)
		return err
	}

	fmt.Printf("addons number %s addonmanager cpu/mem usage %s controller cpu/mem usage %s", AddonsNum, managerUsage, wfControllerUsage)
	oneline := fmt.Sprintf("<addons-num:%s  \n addon-mgr:%s  \nwf-controller:%s>", strings.TrimSpace(AddonsNum), strings.TrimSuffix(string(managerUsage), "\n"), strings.TrimSuffix(string(wfControllerUsage), "\n"))
	datawriter.WriteString(oneline + "\n#############\n")
	datawriter.Flush()
	return nil
}
