package controller

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func TestUnstructuredHasFinalizer(t *testing.T) {
	yamltext := []byte(`apiVersion: addonmgr.keikoproj.io/v1alpha1
	kind: Addon
	metadata: null
	creationTimestamp: '2022-02-18T02:44:58Z'
	deletionGracePeriodSeconds: 0
	deletionTimestamp: '2022-02-19T01:21:05Z'
	finalizers:
	  - delete.addonmgr.keikoproj.io
	generation: 3
	name: event-router-2
	namespace: addon-manager-system
	resourceVersion: '95014'
	uid: c0108492-8530-4d09-9a39-1c88bf8bd8d6
	spec: null
	overrides: null
	kustomize: null
	overlay: {}
	params: null
	context: null
	clusterName: minikube
	clusterRegion: us-west-2
	pkgDescription: Event router
	pkgName: event-router-2
	pkgType: composite
	pkgVersion: v0.2
	selector: {}
	status: null
	checksum: ae8fdf75
	lifecycle: null
	installed: Succeeded
	prereqs: Succeeded
	reason: ''
	resources: []
	starttime: 1645152299471	
	`)
	un := &unstructured.Unstructured{}
	err := yaml.Unmarshal(yamltext, un)
	if err != nil {
		panic(err)
	}
	has := UnstructuredHasFinalizer(un)
	if !has {
		t.Fatalf("there is a finalizer.")
	}

}
