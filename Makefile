
# Image URL to use all building/pushing image targets
IMG ?= keikoproj/addon-manager:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"
KUBERNETES_LOCAL_CLUSTER_VERSION ?= --image=kindest/node:v1.21.2
KOPS_STATE_STORE=s3://kops-state-store-233444812205-us-west-2
KOPS_CLUSTER_NAME=kops-aws-usw2.cluster.k8s.local
GIT_COMMIT := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date +%Y-%m-%dT%H:%M:%SZ)

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
test:
	go test -v -race ./controllers/... ./apis/addon/... ./pkg/... ./cmd/... -coverprofile cover.out

.PHONY: test.controllers
test.controllers:
	go test -v -race ./controllers/... -coverprofile cover.out

.PHONY: test.api
test.api:
	go test -v -race ./apis/addon/... -coverprofile cover.out

.PHONY: test.pkg
test.pkg:
	go test -v -race ./pkg/... -coverprofile cover.out

.PHONY: test.cmd
test.pkg:
	go test -v -race ./cmd/... -coverprofile cover.out

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
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./apis/addon/...

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

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.5 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

code-generator:
ifeq (, $(shell which code-generator))
	@{ \
	set -e ;\
	CODE_GENERATOR_DIR=$$(mkdir -p $(GOPATH)/src/k8s.io/) ;\
	cd $$CODE_GENERATOR_DIR ;\
	go get github.com/kubernetes/code-generator@v0.21.5 ;\
	}
endif

code-generator:
	bash $(GOPATH)/src/k8s.io/code-generator@v0.21.5/generate-groups.sh \
	"deepcopy,client,informer,lister" \
	github.com/keikoproj/addon-manager/pkg/client github.com/keikoproj/addon-manager/apis\
	addon:v1alpha1 \
	--go-header-file ./hack/custom.go.txt
