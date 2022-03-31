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
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	configMap = "addon-manager-controller.yaml"
)

type ControllerConfig struct {
	ManagedNamespaces string `yaml:"addon-namespaces"`
}

func configDir(configEnv string) string {
	return "/etc/manager"
}

func (c *ControllerConfig) LoadYaml(fileName, configEnv string) error {
	f := filepath.Join(configDir(configEnv), fileName)

	if _, err := os.Stat(f); os.IsNotExist(err) {
		panic(fmt.Sprintf("failed getting controller config file %s. err %v", f, err))
	}

	file, err := os.Open(f)
	if err != nil {
		panic(fmt.Sprintf("failed opening file %s. err %v", f, err))
	}

	b, err := ioutil.ReadAll(file)
	if err != nil {
		panic(fmt.Sprintf("failed reading file %s. err %v", f, err))
	}

	if len(b) == 0 {
		panic("reading controller config file size is zero")
	}

	err = yaml.Unmarshal(b, c)
	if err != nil {
		panic(fmt.Sprintf("failed marshal configuration into config %v", err))
	}
	return nil
}
