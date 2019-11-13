
# Image URL to use all building/pushing image targets
IMG ?= keikoproj/addon-manager:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"
KUBERNETES_LOCAL_CLUSTER_VERSION ?= --image=kindest/node:v1.14.3

.EXPORT_ALL_VARIABLES:
GO111MODULE=on

all: test manager addonctl

# Run tests
test: generate fmt vet manifests
	go test ./api/... ./controllers/... ./pkg/... ./cmd/... -coverprofile cover.out

# Run E2E tests
bdd: fmt vet deploy
	go test -timeout 5m -v ./test-bdd/...

# Build manager binary
manager: generate fmt vet
	go build -race -o bin/manager cmd/manager/main.go

# Build addonctl binary
addonctl: generate fmt vet
	go build -race -o bin/addonctl cmd/addonctl/main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./cmd/manager/main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crd/bases

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: install
	kubectl kustomize config/default | kubectl apply -f -

clean:
	kubectl delete addons -n addon-manager-system --all
	kubectl kustomize config/deploy | kubectl delete -f - || true

kops-cluster:
	kops create cluster

kind-cluster-config:
	export KUBECONFIG=$$(kind get kubeconfig-path --name="kind")

kind-cluster: kind-cluster-config
	kind create cluster --config hack/kind.cluster.yaml $(KUBERNETES_LOCAL_CLUSTER_VERSION)
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
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./api/...

# Build the docker image
docker-build: test
	docker build -t ${IMG} .
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.0-beta.5
CONTROLLER_GEN=$(shell go env GOPATH)/bin/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif
