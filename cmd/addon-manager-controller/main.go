package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"

	"github.com/keikoproj/addon-manager/addon/controller"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	wfclientset "github.com/argoproj/argo-workflows/v3/pkg/client/clientset/versioned"
	addonclientset "github.com/keikoproj/addon-manager/pkg/client/clientset/versioned"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
)

const (
	CLIName = "addon-manager-controller"
)

// NewRootCommand returns an new instance of the addon-manager-controller main entrypoint
func NewRootCommand() *cobra.Command {
	var (
		//clientConfig     clientcmd.ClientConfig /// fix me
		logLevel              string // --loglevel
		glogLevel             int    // --gloglevel
		logFormat             string // --log-format
		addonWorkers          int    // --addon-workers
		workflowWorkers       int    // --workflow-workers
		managedNamespace      string // --managed-namespace
		addonresourcesWorkers int
	)

	command := cobra.Command{
		Use:   CLIName,
		Short: "workflow-controller is the controller to operate on workflows",
		RunE: func(c *cobra.Command, args []string) error {
			defer runtimeutil.HandleCrash(runtimeutil.PanicHandlers...)

			config, err := clientcmd.BuildConfigFromFlags("", "/Users/jiminh/.kube/config")
			if err != nil {
				panic(err)
			}

			managedNamespace = "addon-manager-system"
			kubeclientset := kubernetes.NewForConfigOrDie(config)
			wfclientset := wfclientset.NewForConfigOrDie(config)
			addonclientset := addonclientset.NewForConfigOrDie(config)

			// start a controller on instances of our custom resource
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			wfController, err := controller.NewAddonController(ctx, config, kubeclientset, wfclientset, addonclientset, managedNamespace)
			if err != nil {
				panic(err)
			}

			log.Info(" workflowWorkers=", workflowWorkers, " addonWorkers=", addonWorkers, " addonresourcesWorkers", addonresourcesWorkers)
			go wfController.Run(ctx, addonWorkers, workflowWorkers, addonresourcesWorkers)

			// Wait forever
			select {}
		},
	}

	command.Flags().StringVar(&logLevel, "loglevel", "info", "Set the logging level. One of: debug|info|warn|error")
	command.Flags().IntVar(&glogLevel, "gloglevel", 0, "Set the glog logging level")
	command.Flags().StringVar(&logFormat, "log-format", "text", "The formatter to use for logs. One of: text|json")
	command.Flags().IntVar(&workflowWorkers, "workflow-workers", 5, "Number of workflow workers")
	command.Flags().IntVar(&addonWorkers, "addon-workers", 5, "Number of addon workers")
	command.Flags().IntVar(&addonresourcesWorkers, "addon-resources-workers", 5, "Number of addon resources workers")

	viper.AutomaticEnv()
	viper.SetEnvPrefix("")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	if err := viper.BindPFlags(command.Flags()); err != nil {
		log.Fatal(err)
	}
	command.Flags().VisitAll(func(f *pflag.Flag) {
		if !f.Changed && viper.IsSet(f.Name) {
			val := viper.Get(f.Name)
			if err := command.Flags().Set(f.Name, fmt.Sprintf("%v", val)); err != nil {
				log.Fatal(err)
			}
		}
	})

	return &command
}

func init() {
	cobra.OnInitialize(initConfig)
}

func initConfig() {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000Z",
		FullTimestamp:   true,
	})
}

func main() {
	if err := NewRootCommand().Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
