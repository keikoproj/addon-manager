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

package addonctl

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/pkg/apis/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/common"
	"github.com/keikoproj/addon-manager/pkg/version"
	"github.com/keikoproj/addon-manager/pkg/workflows"
)

var addonName string
var clusterName string
var clusterRegion string
var debug bool
var dryRun bool
var description string
var dependencies string
var install string
var namespace string
var pkgChannel string
var pkgType string
var pkgVersion string
var paramsRaw string
var prereqs string
var secretsRaw string
var selector string

// certain variables parsed into these below
var dependenciesMap = make(map[string]string)
var params = make(map[string]string)
var prereqResources = make([]string, 0)
var prereqScripts = make(map[string]string)
var installResources = make([]string, 0)
var installScripts = make(map[string]string)
var secretsList = make([]string, 0)
var selectorMap = make(map[string]string)

var addonMgrSystemNamespace = "addon-manager-system"
var log logr.Logger

// Execute the command
func Execute() {
	root := newRootCommand()
	log = zap.New(zap.UseDevMode(true))

	if err := root.Execute(); err != nil {
		log.Error(err, "Failed to execute command")
		os.Exit(1)
	}
}

func parseAllArgs(md *cobra.Command, args []string) error {
	fmt.Println("Parsing all args")
	err := extractResources(prereqs, install)
	if err != nil {
		return err
	}
	err = parseAddonParams(paramsRaw)
	if err != nil {
		return err
	}
	err = parseDependencies(dependencies)
	if err != nil {
		return err
	}
	err = parseSecrets(secretsRaw)
	if err != nil {
		return err
	}
	err = parseSelector(selector)
	if err != nil {
		return err
	}
	err = validatePkgType(pkgType)
	if err != nil {
		return err
	}
	return nil
}

func newRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "addonctl",
		Short:   "A control plane for managing addons",
		Version: version.ToString(),
	}

	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Println(err)
		return rootCmd
	}

	rootCmd.PersistentFlags().StringVar(&clusterName, "cluster-name", "", "Name of the cluster context being used")
	rootCmd.MarkFlagRequired("cluster-name")
	rootCmd.PersistentFlags().StringVar(&clusterRegion, "cluster-region", "", "Cluster region")
	rootCmd.MarkFlagRequired("cluster-region")
	rootCmd.PersistentFlags().StringVar(&description, "desc", "", "Description of the addon")
	rootCmd.PersistentFlags().StringVar(&dependencies, "deps", "", "Comma separated dependencies list in the format 'pkgName:pkgVersion'")
	rootCmd.PersistentFlags().StringVarP(&pkgChannel, "channel", "c", "", "Channel for the addon package")
	rootCmd.MarkFlagRequired("channel")
	rootCmd.PersistentFlags().StringVarP(&pkgType, "type", "t", "", "Addon package type")
	rootCmd.MarkFlagRequired("type")
	rootCmd.PersistentFlags().StringVarP(&pkgVersion, "version", "v", "", "Addon package version")
	rootCmd.MarkFlagRequired("version")
	rootCmd.PersistentFlags().StringVar(&secretsRaw, "secrets", "", "Comma separated list of secret names which are validated as part ofthe addon-manager-system namespace")
	rootCmd.PersistentFlags().StringVar(&selector, "selector", "", "Selector applied to all resources?")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Namespace where the addon will be deployed")
	rootCmd.MarkFlagRequired("namespace")

	// TODO P3 --v verbose

	rootCmd.PersistentFlags().StringVarP(&paramsRaw, "params", "p", "", "Params to supply to the resource yaml")
	rootCmd.PersistentFlags().StringVar(&prereqs, "prereqs", "", "File or directory of resource yaml to submit as prereqs step")
	rootCmd.PersistentFlags().StringVar(&install, "install", "", "File or directory of resource yaml to submit as install step")

	rootCmd.PersistentFlags().BoolVar(&dryRun, "dryrun", false, "Outputs the addon spec but doesn't submit")

	// add commands
	rootCmd.AddCommand(&cobra.Command{
		Use:     "create",
		Short:   "Create the addon resource with the supplied arguments",
		PreRunE: parseAllArgs,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.New("requires more arguments")
			}
			// Ensure Addon name is first positional argument
			addonName = args[0]
			re := regexp.MustCompile("^[a-zA-Z][a-zA-Z0-9-.]*$")
			if !re.MatchString(addonName) {
				return errors.New("Invalid addon name")
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			instance := &addonmgrv1alpha1.Addon{}
			instance.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "addonmgr.keikoproj.io",
				Version: "v1alpha1",
				Kind:    "Addon",
			})
			instance.SetName(addonName)
			instance.SetNamespace(addonMgrSystemNamespace)

			//assume all args are validated
			instance.Spec.PkgChannel = pkgChannel
			instance.Spec.PkgDeps = dependenciesMap
			instance.Spec.PkgDescription = description
			instance.Spec.PkgName = addonName
			instance.Spec.PkgType = addonmgrv1alpha1.PackageType(pkgType)
			instance.Spec.PkgVersion = pkgVersion
			instance.Spec.Selector = metav1.LabelSelector{MatchLabels: selectorMap}
			instance.Spec.Secrets = []addonmgrv1alpha1.SecretCmdSpec{}
			for _, secret := range secretsList {
				scs := addonmgrv1alpha1.SecretCmdSpec{Name: secret}
				instance.Spec.Secrets = append(instance.Spec.Secrets, scs)
			}
			instance.Spec.Params.Namespace = namespace
			instance.Spec.Params.Context.ClusterName = clusterName
			instance.Spec.Params.Context.ClusterRegion = clusterRegion
			// instance.Spec.Params.Context.AdditionalConfigs

			// set params as string params, will be coppied over in workflow.go
			for name, val := range params {
				instance.Spec.Params.Data[name] = addonmgrv1alpha1.FlexString(val)
			}

			prereqWorkflowBuilder := workflows.New()
			prereqWf := prereqWorkflowBuilder.Scripts(prereqScripts).Resources(prereqResources).Build() // Removed SetName(n) because it depends on checksum, addon_controller must set it
			instance.Spec.Lifecycle.Prereqs.Template = workflows.ConvertUnstructuredWorkflowToString(prereqWf)
			// instance.Spec.Lifecycle.Prereqs.NamePrefix
			// instance.Spec.Lifecycle.Prereqs.Role

			installWorkflowBuilder := workflows.New()
			installWf := installWorkflowBuilder.Scripts(installScripts).Resources(installResources).Build() // Removed SetName(n) because it depends on checksum, addon_controller must set it
			instance.Spec.Lifecycle.Install.Template = workflows.ConvertUnstructuredWorkflowToString(installWf)
			// instance.Spec.Lifecycle.Install.NamePrefix
			// instance.Spec.Lifecycle.Install.Role

			deleteWorkflowBuilder := workflows.New()
			deleteWf := deleteWorkflowBuilder.Delete().Build()
			instance.Spec.Lifecycle.Delete.Template = workflows.ConvertUnstructuredWorkflowToString(deleteWf)

			fmt.Println(dryRun)
			if dryRun {
				fmt.Println("Printing workflow to stdout without submitting:")
				prettyPrint(instance)
				// TODO output to file
				return
			}

			kubeClient := dynamic.NewForConfigOrDie(cfg)

			addonMap := make(map[string]interface{})
			jsonInstance, _ := json.Marshal(instance)

			err = json.Unmarshal(jsonInstance, &addonMap)
			if err != nil {
				fmt.Println(err)
				return
			}

			ctx := context.TODO()
			addon := unstructured.Unstructured{}
			addon.SetUnstructuredContent(addonMap)
			addonObject, err := kubeClient.Resource(common.AddonGVR()).Namespace(addonMgrSystemNamespace).Get(ctx, addonName, metav1.GetOptions{})

			if err == nil {
				fmt.Printf("Updating addon %s...\n", addonName)
				resourceVersion := addonObject.GetResourceVersion()
				addon.SetResourceVersion(resourceVersion)
				_, err = kubeClient.Resource(common.AddonGVR()).Namespace(addonMgrSystemNamespace).Update(ctx, &addon, metav1.UpdateOptions{})
				if err != nil {
					fmt.Println(err)
					return
				}

			} else {
				fmt.Printf("Creating addon %s...\n", addonName)
				_, err = kubeClient.Resource(common.AddonGVR()).Namespace(addonMgrSystemNamespace).Create(ctx, &addon, metav1.CreateOptions{})
				if err != nil {
					fmt.Println(err)
					return
				}
			}
		},
	})

	return rootCmd
}

func prettyPrint(v interface{}) (err error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		fmt.Println(string(b))
	}
	return
}

func parseSelector(sel string) error {
	fmt.Println("Parsing selector...")
	if sel == "" {
		return nil
	}
	s := strings.Split(sel, ":")
	if len(s) == 1 && s[0] == ":" {
		return errors.New("Missing ':' separator in selector")
	} else if len(s) != 2 {
		return errors.New("Dependency had multiple separators")
	}
	selectorMap[s[0]] = s[1]
	return nil
}

func parseAddonParams(raw string) error {
	fmt.Println("Parsing addon params...")
	if raw == "" {
		params = nil
		return nil
	}

	rawList := strings.Split(raw, ",")
	for _, item := range rawList {
		kv := strings.Split(item, "=")
		if len(kv) == 1 && kv[0] == "=" {
			return fmt.Errorf("Unable to parse addon params: '%s'. Key-value pair %s does not have separator '='", raw, item)
		}
		params[kv[0]] = kv[1]
	}
	return nil
}

func validatePkgType(pt string) error {
	fmt.Println("Validating pkgType...")
	t := addonmgrv1alpha1.PackageType(pt)
	if t != addonmgrv1alpha1.HelmPkg && t != addonmgrv1alpha1.ShipPkg && t != addonmgrv1alpha1.KustomizePkg && t != addonmgrv1alpha1.CnabPkg && t != addonmgrv1alpha1.CompositePkg {
		return errors.New("unsupported package type")
	}
	return nil
}

func parseDependencies(deps string) error {
	fmt.Println("Parsing dependencies...")
	if deps == "" {
		return nil
	}
	for _, dep := range strings.Split(deps, ",") {
		d := strings.Split(dep, ":")
		if len(d) == 1 && d[0] == ":" {
			return fmt.Errorf("missing ':' separator in dependency")
		} else if len(d) != 2 {
			return fmt.Errorf("dependency had multiple separators")
		}
		dependenciesMap[d[0]] = d[1]
	}
	return nil
}

//taking in arguments but parsing into global arguments is fine? the best approach?
func extractResources(prereqsPath, installPath string) error {
	lifecycleSteps := map[string]string{"prereqs": prereqsPath, "install": installPath}
	for stepName, path := range lifecycleSteps {
		if path != "" {
			path = filepath.Join(os.Getenv("PWD"), path)
			fi, err := os.Stat(path)

			switch {
			case err != nil:
				return err
			case fi.IsDir():
				files, err := ioutil.ReadDir(path)
				if err != nil {
					return errors.Wrapf(err, "failed to read dir path %s", path)
				}
				for _, f := range files {
					switch {
					case strings.HasSuffix(f.Name(), ".py"):
						data, err := ioutil.ReadFile(f.Name())
						if err != nil {
							return fmt.Errorf("unable to read file %s", f.Name())
						}

						if stepName == "prereqs" {
							prereqScripts[f.Name()] = string(data)
						} else if stepName == "install" {
							installScripts[f.Name()] = string(data)
						}

					case strings.HasSuffix(f.Name(), ".yaml") || strings.HasSuffix(f.Name(), ".yml"):
						if err := parseResources(path, stepName); err != nil {
							return err
						}
					default:
						// simply ignore the file
						continue
					}
				}
			default:
				// it's a file
				switch {
				case strings.HasSuffix(fi.Name(), ".py"):
					data, err := ioutil.ReadFile(path)
					if err != nil {
						return fmt.Errorf("unable to read file %s", fi.Name())
					}

					if stepName == "prereqs" {
						prereqScripts[fi.Name()] = string(data)
					} else if stepName == "install" {
						installScripts[fi.Name()] = string(data)
					}

				case strings.HasSuffix(fi.Name(), ".yaml") || strings.HasSuffix(fi.Name(), ".yml"):
					if err := parseResources(path, stepName); err != nil {
						return err
					}
				default:
					// simply ignore the file
					continue
				}
			}
		}
	}
	return nil
}

func parseSecrets(raw string) error {
	if raw == "" {
		return nil
	}
	secrets := strings.Split(raw, ",")
	if len(secrets) == 1 && secrets[0] == "," {
		return fmt.Errorf("Error parsing secrets %s", raw)
	}
	for _, item := range secrets {
		secretsList = append(secretsList, item)
	}
	return nil
}

// best way to write parsing functions? take no params and work on global variables, or take and modify the global params (need to pass in pointers in that case)

func parseResources(filename, stepName string) error {
	rawBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return errors.Wrapf(err, "failed to read file %s", filename)
	}
	resources := strings.Split(string(rawBytes), "---\n")
	for _, resource := range resources {
		if strings.TrimSpace(resource) == "" {
			continue
		}

		if stepName == "prereqs" {
			prereqResources = append(prereqResources, resource)
		} else if stepName == "install" {
			installResources = append(installResources, resource)
		}
	}
	return nil
}
