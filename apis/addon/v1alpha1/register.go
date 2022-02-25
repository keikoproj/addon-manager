package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SchemeGroupVersion is group version used to register these objects
var (
	SchemeGroupVersion          = schema.GroupVersion{Group: "addonmgr.keikoproj.io", Version: "v1alpha1"}
	AddonSchemaGroupVersionKind = schema.GroupVersionKind{Group: "addonmgr.keikoproj.io", Version: "v1alpha1", Kind: "Addon"}
)

// Kind takes an unqualified kind and returns back a Group qualified GroupKind
func Kind(kind string) schema.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns a Group-qualified GroupResource.
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

// Compitability, temp revert
