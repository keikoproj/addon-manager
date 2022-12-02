
# Image URL to use all building/pushing image targets
IMG ?= keikoproj/addon-manager:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.24

KUBERNETES_LOCAL_CLUSTER_VERSION ?= --image=kindest/node:v1.24.7
KOPS_STATE_STORE=s3://kops-state-store-233444812205-us-west-2
KOPS_CLUSTER_NAME=kops-aws-usw2.cluster.k8s.local
GIT_COMMIT := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKGS := $(shell go list ./...|grep -v test-)

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
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test -race $(PKGS) -coverprofile cover.out

# Run E2E tests
bdd: fmt vet deploy
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
	kubectl delete addons -n addon-manager-system --all
	kubectl kustomize config/deploy | kubectl delete -f - || true

kops-cluster-setup:
	kops replace --force --state=${KOPS_STATE_STORE} -f hack/kops-aws-usw2.cluster.yaml
	kops create secret --state=${KOPS_STATE_STORE} --name=${KOPS_CLUSTER_NAME} sshpublickey admin -i ~/.ssh/id_rsa.pub

kops-cluster:
	kops update cluster --state=${KOPS_STATE_STORE} --name=${KOPS_CLUSTER_NAME} --yes
	kops rolling-update cluster --state=${KOPS_STATE_STORE} --name=${KOPS_CLUSTER_NAME} --yes --cloudonly
	kops validate cluster --state=${KOPS_STATE_STORE} --name=${KOPS_CLUSTER_NAME};

kops-cluster-delete:
	kops delete --state=${KOPS_STATE_STORE} -f hack/kops-aws-usw2.cluster.yaml --yes

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
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

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
	goreleaser release --rm-dist

snapshot:
	goreleaser release --rm-dist --snapshot

code-generator:
ifeq (, $(shell which code-generator))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	curl -L -o code-generator.zip https://github.com/kubernetes/code-generator/archive/refs/tags/v0.21.5.zip ;\
	unzip code-generator.zip ;\
	mv code-generator-0.21.5 $(GOPATH)/bin/ ;\
	rm -rf code-generator.zip ;\
	}
CODE_GENERATOR_GEN=$(GOBIN)/code-generator-0.21.5
else
CODE_GENERATOR_GEN=$(shell which code-generator)
endif

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
KUSTOMIZE_VERSION ?= v4.5.5
CONTROLLER_TOOLS_VERSION ?= v0.9.2

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	test -s $(LOCALBIN)/kustomize || { curl -Ss $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN); }

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

