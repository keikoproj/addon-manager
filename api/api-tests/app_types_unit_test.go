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

package apitests

import (
	"testing"

	"github.com/onsi/gomega"

	addonv1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
)

func TestAPIFunctions(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tests := []struct {
		phase    addonv1.ApplicationAssemblyPhase
		expected bool
	}{
		{
			phase:    addonv1.Succeeded,
			expected: true,
		},
		{
			phase:    addonv1.Pending,
			expected: false,
		},
	}

	for _, tc := range tests {
		res := tc.phase.Completed()
		g.Expect(res).To(gomega.Equal(tc.expected))
	}

	tests = []struct {
		phase    addonv1.ApplicationAssemblyPhase
		expected bool
	}{
		{
			phase:    addonv1.Succeeded,
			expected: true,
		},
		{
			phase:    addonv1.Failed,
			expected: false,
		},
	}

	for _, tc := range tests {
		res := tc.phase.Succeeded()
		g.Expect(res).To(gomega.Equal(tc.expected))
	}

	tests = []struct {
		phase    addonv1.ApplicationAssemblyPhase
		expected bool
	}{
		{
			phase:    addonv1.Deleting,
			expected: true,
		},
		{
			phase:    addonv1.Failed,
			expected: false,
		},
	}

	for _, tc := range tests {
		res := tc.phase.Deleting()
		g.Expect(res).To(gomega.Equal(tc.expected))
	}
}
