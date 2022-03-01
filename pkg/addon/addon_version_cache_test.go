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
	"reflect"
	"testing"

	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
)

func TestNewCachedClient(t *testing.T) {
	expected := &cached{
		addons: make(map[string]map[string]Version),
	}
	tests := []struct {
		name string
		want VersionCacheClient
	}{
		{name: "get-new-client", want: expected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewAddonVersionCacheClient(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewAddonVersionCacheClient() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cached_AddVersion(t *testing.T) {
	a := make(map[string]map[string]Version)
	type fields struct {
		addons map[string]map[string]Version
	}
	type args struct {
		v Version
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{name: "add-version", fields: fields{addons: a}, args: args{v: Version{
			Name:      "test",
			Namespace: "default",
			PackageSpec: addonmgrv1alpha1.PackageSpec{
				PkgName:        "test/A",
				PkgVersion:     "1.0.0",
				PkgType:        "composite",
				PkgDescription: "Something",
			},
			PkgPhase: addonmgrv1alpha1.Succeeded,
		},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cached{
				addons: tt.fields.addons,
			}
			c.AddVersion(tt.args.v)
		})
	}
}

func Test_cached_GetVersions(t *testing.T) {
	a := make(map[string]map[string]Version)
	a["test/addon-1"] = map[string]Version{
		"1.0.1": {},
	}
	type fields struct {
		addons map[string]map[string]Version
	}
	type args struct {
		pkgName string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   map[string]Version
	}{
		{name: "get-versions", fields: fields{addons: a}, args: args{pkgName: "test/addon-1"}, want: a["test/addon-1"]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cached{
				addons: tt.fields.addons,
			}
			if got := c.GetVersions(tt.args.pkgName); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("cached.GetVersions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cached_GetVersion(t *testing.T) {
	type fields struct {
		addons map[string]map[string]Version
	}
	type args struct {
		pkgName    string
		pkgVersion string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *Version
	}{
		{name: "get-version", fields: fields{addons: map[string]map[string]Version{
			"test/addon-1": {
				"1.0.0": {Name: "test-1"},
			},
		}}, args: args{pkgName: "test/addon-1", pkgVersion: "1.0.0"}, want: &Version{Name: "test-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cached{
				addons: tt.fields.addons,
			}
			if got := c.GetVersion(tt.args.pkgName, tt.args.pkgVersion); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("cached.GetVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cached_RemoveVersion(t *testing.T) {
	type fields struct {
		addons map[string]map[string]Version
	}
	type args struct {
		pkgName    string
		pkgVersion string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *Version
	}{
		{name: "remove-version", fields: fields{addons: map[string]map[string]Version{
			"test/addon-1": {
				"1.0.1": Version{Name: "test-1"},
			},
		}}, args: args{pkgName: "test/addon-1", pkgVersion: "1.0.1"}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cached{
				addons: tt.fields.addons,
			}
			c.RemoveVersion(tt.args.pkgName, tt.args.pkgVersion)
			if got := c.GetVersion(tt.args.pkgName, tt.args.pkgVersion); got != tt.want {
				t.Errorf("cached.GetVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cached_RemoveVersions(t *testing.T) {
	type fields struct {
		addons map[string]map[string]Version
	}
	type args struct {
		pkgName string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{name: "remove-version", fields: fields{addons: map[string]map[string]Version{
			"test/addon-1": {
				"1.0.1": Version{Name: "test-1"},
			},
		}}, args: args{pkgName: "test/addon-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cached{
				addons: tt.fields.addons,
			}
			c.RemoveVersions(tt.args.pkgName)
			if got := c.GetVersions(tt.args.pkgName); len(got) > 0 {
				t.Errorf("cached.GetVersion() = %v", got)
			}
		})
	}
}

func Test_cached_GetAllVersions(t *testing.T) {
	type fields struct {
		addons map[string]map[string]Version
	}
	tests := []struct {
		name   string
		fields fields
		want   map[string]map[string]Version
	}{
		{name: "get-all-versions", fields: fields{addons: map[string]map[string]Version{
			"test/addon-1": {
				"1.0.0": Version{Name: "test-1"},
			},
		}}, want: map[string]map[string]Version{
			"test/addon-1": {
				"1.0.0": Version{Name: "test-1"},
			},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cached{
				addons: tt.fields.addons,
			}
			if got := c.GetAllVersions(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("cached.GetAllVersions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cached_resolveVersion(t *testing.T) {
	type fields struct {
		addons map[string]map[string]Version
	}
	type args struct {
		m          map[string]Version
		pkgVersion string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *Version
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cached{
				addons: tt.fields.addons,
			}
			if got := c.resolveVersion(tt.args.m, tt.args.pkgVersion); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("cached.resolveVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cached_deepCopy(t *testing.T) {
	type fields struct {
		addons map[string]map[string]Version
	}
	tests := []struct {
		name   string
		fields fields
		want   map[string]map[string]Version
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cached{
				addons: tt.fields.addons,
			}
			if got := c.deepCopy(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("cached.deepCopy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cached_copyVersionMap(t *testing.T) {
	type fields struct {
		addons map[string]map[string]Version
	}
	type args struct {
		m map[string]Version
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   map[string]Version
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cached{
				addons: tt.fields.addons,
			}
			if got := c.copyVersionMap(tt.args.m); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("cached.copyVersionMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cached_HasVersionName(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name  string
		c     *cached
		args  args
		want  bool
		want1 *Version
	}{
		{name: "has-version", c: &cached{
			addons: map[string]map[string]Version{
				"test/addon-1": {
					"1.0.0": Version{Name: "my-ns/addon-1"},
				},
			}}, args: args{name: "my-ns/addon-1"}, want: true, want1: &Version{Name: "my-ns/addon-1"}},
		{name: "not-has-version", c: &cached{
			addons: map[string]map[string]Version{
				"test/addon-1": {
					"1.0.0": Version{Name: "my-ns/addon-1"},
				},
			}}, args: args{name: "my-ns/invalid-addon-2"}, want: false, want1: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := tt.c.HasVersionName(tt.args.name)
			if got != tt.want {
				t.Errorf("cached.HasVersionName() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("cached.HasVersionName() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
