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

package version

import "fmt"

// The below variables will be overrriden using ldflags set by goreleaser during the build process
var (
	// Version is the version string
	Version = "v0.9.0"

	// GitCommit is the git commit hash
	GitCommit = "NONE"

	// BuildDate is the date of the build
	BuildDate = "UNKNOWN"
)

const versionStringFmt = `{"version": "%s", "gitCommit": "%s", "buildDate": "%s"}`

// ToString returns the output of Version, GitCommit, BuildDate
func ToString() string {
	return fmt.Sprintf(versionStringFmt, Version, GitCommit, BuildDate)
}
