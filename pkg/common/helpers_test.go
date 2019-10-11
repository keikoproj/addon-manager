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
	"reflect"
	"testing"
)

func TestContainsString(t *testing.T) {
	a := []string{"this", "is", "a", "test", "slice"}
	got := ContainsString(a, "test")
	if !got {
		t.Errorf("common.ContainsString = %v, want %v", got, true)
	}
}

func TestNotContainsString(t *testing.T) {
	a := []string{"this", "is", "a", "test", "slice"}
	got := ContainsString(a, "toast")
	if got {
		t.Errorf("common.ContainsString = %v, want %v", got, true)
	}
}

func TestEmptyContainsString(t *testing.T) {
	a := []string{}
	got := ContainsString(a, "")
	if got {
		t.Errorf("common.ContainsString = %v, want %v", got, true)
	}
}

func TestRemoveStringPresent(t *testing.T) {
	a := []string{"this", "is", "a", "test", "slice"}
	expected := []string{"this", "is", "a", "slice"}
	got := RemoveString(a, "test")
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("common.RemoveString = %v, want %v", got, expected)
	}
}

func TestRemoveStringNotPresent(t *testing.T) {
	a := []string{"this", "is", "a", "test", "slice"}
	expected := []string{"this", "is", "a", "test", "slice"}
	got := RemoveString(a, "toast")
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("common.RemoveString = %v, want %v", got, expected)
	}
}
