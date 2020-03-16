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

import (
	"github.com/Masterminds/semver/v3"
	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/v1alpha1"
	"sync"
)

// VersionCacheClient interface clients must implement for addon version cache.
type VersionCacheClient interface {
	AddVersion(Version)
	GetVersions(pkgName string) map[string]Version
	GetVersion(pkgName, pkgVersion string) *Version
	HasVersionName(name string) (bool, *Version)
	RemoveVersion(pkgName, pkgVersion string)
	RemoveVersions(pkgName string)
	GetAllVersions() map[string]map[string]Version
}

// Version data that will be cached
type Version struct {
	Name      string
	Namespace string
	addonmgrv1alpha1.PackageSpec
	PkgPhase addonmgrv1alpha1.ApplicationAssemblyPhase
}

type cached struct {
	sync.RWMutex
	addons map[string]map[string]Version
}

// NewAddonVersionCacheClient returns a new instance of VersionCacheClient
func NewAddonVersionCacheClient() VersionCacheClient {
	return &cached{
		addons: make(map[string]map[string]Version),
	}
}

func (c *cached) AddVersion(v Version) {
	c.Lock()
	defer c.Unlock()

	_, ok := c.addons[v.PkgName]
	if !ok {
		mm := make(map[string]Version)
		c.addons[v.PkgName] = mm
	}
	c.addons[v.PkgName][v.PkgVersion] = v
}

func (c *cached) GetVersions(pkgName string) map[string]Version {
	c.RLock()
	defer c.RUnlock()

	m, ok := c.addons[pkgName]
	if !ok {
		return nil
	}

	return c.copyVersionMap(m)
}

func (c *cached) GetVersion(pkgName, pkgVersion string) *Version {
	var vmap = c.GetVersions(pkgName)

	if vmap == nil {
		return nil
	}

	v, ok := vmap[pkgVersion]
	if !ok {
		return c.resolveVersion(vmap, pkgVersion)
	}

	return &v
}

func (c *cached) RemoveVersion(pkgName, pkgVersion string) {
	c.Lock()
	defer c.Unlock()

	if _, ok := c.addons[pkgName][pkgVersion]; ok {
		// Remove version
		delete(c.addons[pkgName], pkgVersion)
	}
}

func (c *cached) RemoveVersions(pkgName string) {
	c.Lock()
	defer c.Unlock()

	if _, ok := c.addons[pkgName]; ok {
		// Remove all versions
		delete(c.addons, pkgName)
	}
}

func (c *cached) GetAllVersions() map[string]map[string]Version {
	return c.deepCopy()
}

func (c *cached) HasVersionName(name string) (bool, *Version) {
	vvmap := c.GetAllVersions()

	for _, vmap := range vvmap {
		for _, version := range vmap {
			if version.Name == name {
				return true, &version
			}
		}
	}

	return false, nil
}

func (c *cached) resolveVersion(m map[string]Version, pkgVersion string) *Version {
	// Assume pkgVersion may be a semantic package description
	ct, err := semver.NewConstraint(pkgVersion)
	if err != nil {
		// Package version is not a constraint
		return nil
	}

	for key := range m {
		sv, err := semver.NewVersion(key)
		if err != nil {
			// Cannot parse cached map version
			continue
		}

		if ct.Check(sv) {
			v := m[key]
			return &v
		}
	}

	return nil
}

func (c *cached) deepCopy() map[string]map[string]Version {
	c.RLock()
	defer c.RUnlock()
	cacheCopy := make(map[string]map[string]Version)

	for key, val := range c.addons {
		cacheCopy[key] = c.copyVersionMap(val)
	}

	return cacheCopy
}

func (c *cached) copyVersionMap(m map[string]Version) map[string]Version {
	var vmap = make(map[string]Version)

	// copy map by assigning elements to new map
	for key, value := range m {
		vmap[key] = value
	}

	return vmap
}
