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
	"encoding/json"
	"errors"
)

func DeepCopyJSON(src map[string]interface{}, dest map[string]interface{}) error {
	if src == nil {
		return errors.New("src is nil. You cannot read from a nil map")
	}
	if dest == nil {
		return errors.New("dest is nil. You cannot insert to a nil map")
	}
	jsonStr, err := json.Marshal(src)
	if err != nil {
		return err
	}
	err = json.Unmarshal(jsonStr, &dest)
	if err != nil {
		return err
	}
	return nil
}

// Helper functions to check and remove string from a slice of strings.
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func RemoveString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}
