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

package addon

import "time"

// Addon constants
const (
	Group          string = "addonmgr.keikoproj.io"
	Version        string = "v1alpha1"
	APIVersion     string = Group + "/" + Version
	AddonKind      string = "Addon"
	AddonSingular  string = "addon"
	AddonPlural    string = "addons"
	AddonShortName string = "addon"
	AddonFullName  string = AddonPlural + "." + Group

	ManagedNameSpace string = "addon-manager-system"

	AddonResyncPeriod = 20 * time.Minute

	FinalizerName = "delete.addonmgr.keikoproj.io"

	ResourceDefaultManageByLabel = "app.kubernetes.io/managed-by"
	ResourceDefaultManageByValue = "addonmgr.keikoproj.io"
	ResourceDefaultOwnLabel      = "app.kubernetes.io/name"
	ResourceDefaultPartLabel     = "app.kubernetes.io/part-of"

	TTL = time.Duration(1) * time.Hour // 1 hour
)
