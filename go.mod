module github.com/keikoproj/addon-manager

go 1.17

require (
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/argoproj/argo-workflows/v3 v3.2.6
	github.com/go-logr/logr v0.4.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.2.1
	golang.org/x/net v0.0.0-20210614182718-04defd469f4e
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	k8s.io/api v0.21.5
	k8s.io/apiextensions-apiserver v0.21.2
	k8s.io/apimachinery v0.21.5
	k8s.io/client-go v0.21.5
	sigs.k8s.io/controller-runtime v0.9.2
)
