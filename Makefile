# Image URL to use all building/pushing image targets
IMG ?= keikoproj/addon-manager:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.32.0

KUBERNETES_LOCAL_CLUSTER_VERSION ?= --image=kindest/node:v1.32.0
GIT_COMMIT := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKGS := $(shell go list ./...|grep -v test-)
MODULE := $(shell go list -m)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

LOADTEST_TIMEOUT ?= "60m"
LOADTEST_START_NUMBER ?= 1
LOADTEST_END_NUMBER ?= 2000

.EXPORT_ALL_VARIABLES:
GO111MODULE=on

all: test manager addonctl

# Run tests
.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test -race -v $(PKGS) -coverprofile cover.out

.PHONY: cover
cover: test
	go tool cover -func=cover.out -o coverage.txt
	go tool cover -html=cover.out -o coverage.html
	@cat coverage.txt
	@echo "Run 'open coverage.html' to view coverage report."

# Run E2E tests
bdd: clean fmt vet deploy
	go test -timeout 5m -v ./test-bdd/...

loadtest: fmt vet deploy
	go test -timeout $(LOADTEST_TIMEOUT) -startnumber $(LOADTEST_START_NUMBER) -endnumber $(LOADTEST_END_NUMBER) -v ./test-load/...

# Build manager binary
manager: generate fmt vet
	go build -race -o bin/manager main.go

# Build addonctl binary
addonctl: generate fmt vet
	go build -race -o bin/addonctl cmd/addonctl/main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crd/bases

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: install
	kubectl kustomize config/default | kubectl apply -f -

clean:
	@echo "Cleaning up addons and deployments..."
	kubectl delete addons -n addon-manager-system --all --wait=true --timeout=60s || true
	@for addon in $(kubectl get addons -n addon-manager-system -o jsonpath='{.items[*].metadata.name}'); do kubectl patch addon ${addon} -n addon-manager-system -p '{"metadata":{"finalizers":null}}' --type=merge; done
	@kubectl kustomize config/default | kubectl delete -f - 2> /dev/null || true

kind-cluster-config:
	export KUBECONFIG=$$(kind export kubeconfig --name="kind")

kind-cluster:
	kind create cluster --config hack/kind.cluster.yaml --name="kind" $(KUBERNETES_LOCAL_CLUSTER_VERSION)
	kind load docker-image ${IMG}

kind-cluster-delete: kind-cluster-config
	kind delete cluster

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
.PHONY: generate
generate: controller-gen types ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="$(MODULE)"

# generates many other files (listers, informers, client etc).
api/addon/v1alpha1/zz_generated.deepcopy.go: $(TYPES)
	ln -s . v1
	$(CODE_GENERATOR_GEN)/generate-groups.sh \
			"deepcopy,client,informer,lister" \
			github.com/keikoproj/addon-manager/pkg/client github.com/keikoproj/addon-manager/api\
            addon:v1alpha1 \
            --go-header-file ./hack/custom-boilerplate.go.txt
	rm -rf v1

.PHONY: types
types: api/addon/v1alpha1/zz_generated.deepcopy.go

# Build the docker image
docker-build: manager
	docker build --build-arg COMMIT=${GIT_COMMIT} --build-arg DATE=${BUILD_DATE} -t ${IMG} .
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
docker-push:
	docker push ${IMG}

release:
	goreleaser release --clean

snapshot:
	goreleaser release --clean --snapshot

code-generator:
ifeq (, $(shell which code-generator))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	curl -L -o code-generator.zip https://github.com/kubernetes/code-generator/archive/refs/tags/v0.28.2.zip ;\
	unzip code-generator.zip ;\
	mv code-generator-0.28.2 $(GOPATH)/bin/ ;\
	rm -rf code-generator.zip ;\
	}
CODE_GENERATOR_GEN=$(GOBIN)/code-generator-0.28.2
else
CODE_GENERATOR_GEN=$(shell which code-generator)
endif

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)

## Tool Versions
KUSTOMIZE_VERSION ?= v5.3.0
CONTROLLER_TOOLS_VERSION ?= v0.17.2
ENVTEST_VERSION ?= release-0.20

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary (ideally with version)
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv "$$(echo "$(1)" | sed "s/-$(3)$$//")" $(1) ;\
}
endef
