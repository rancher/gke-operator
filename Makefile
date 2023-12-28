TARGETS := $(shell ls scripts)
GIT_BRANCH?=$(shell git branch --show-current)
GIT_COMMIT?=$(shell git rev-parse HEAD)
GIT_COMMIT_SHORT?=$(shell git rev-parse --short HEAD)
GIT_TAG?=v9.0.0
ifneq ($(GIT_BRANCH), main)
GIT_TAG?=$(shell git describe --abbrev=0 --tags 2>/dev/null || echo "v0.0.0" )
endif
TAG?=${GIT_TAG}-${GIT_COMMIT_SHORT}
OPERATOR_CHART?=$(shell find $(ROOT_DIR) -type f -name "rancher-gke-operator-[0-9]*.tgz" -print)
CRD_CHART?=$(shell find $(ROOT_DIR) -type f -name "rancher-gke-operator-crd*.tgz" -print)
CHART_VERSION?=900 # Only used in e2e to avoid downgrades from rancher
REPO?=docker.io/rancher/gke-operator
CLUSTER_NAME?="gke-operator-e2e"
E2E_CONF_FILE ?= $(ROOT_DIR)/test/e2e/config/config.yaml

ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
BIN_DIR := $(abspath $(ROOT_DIR)/bin)
GO_INSTALL = ./scripts/go_install.sh

MOCKGEN_VER := v1.6.0
MOCKGEN_BIN := mockgen
MOCKGEN := $(BIN_DIR)/$(MOCKGEN_BIN)-$(MOCKGEN_VER)
MOCKGEN_PKG := github.com/golang/mock/mockgen

GINKGO_VER := v2.15.0
GINKGO_BIN := ginkgo
GINKGO := $(BIN_DIR)/$(GINKGO_BIN)-$(GINKGO_VER)
GINKGO_PKG := github.com/onsi/ginkgo/v2/ginkgo

SETUP_ENVTEST_VER := v0.0.0-20211110210527-619e6b92dab9
SETUP_ENVTEST_BIN := setup-envtest
SETUP_ENVTEST := $(BIN_DIR)/$(SETUP_ENVTEST_BIN)-$(SETUP_ENVTEST_VER)

ifeq ($(shell go env GOOS),darwin) # Use the darwin/amd64 binary until an arm64 version is available
	KUBEBUILDER_ASSETS ?= $(shell $(SETUP_ENVTEST) use --use-env -p path --arch amd64 $(KUBEBUILDER_ENVTEST_KUBERNETES_VERSION))
else
	KUBEBUILDER_ASSETS ?= $(shell $(SETUP_ENVTEST) use --use-env -p path $(KUBEBUILDER_ENVTEST_KUBERNETES_VERSION))
endif

GO_APIDIFF_VER := v0.7.0
GO_APIDIFF_BIN := go-apidiff
GO_APIDIFF := $(BIN_DIR)/$(GO_APIDIFF_BIN)-$(GO_APIDIFF_VER)
GO_APIDIFF_PKG := github.com/joelanford/go-apidiff

default: operator

$(MOCKGEN):
	GOBIN=$(BIN_DIR) $(GO_INSTALL) $(MOCKGEN_PKG) $(MOCKGEN_BIN) $(MOCKGEN_VER)

$(GINKGO):
	GOBIN=$(BIN_DIR) $(GO_INSTALL) github.com/onsi/ginkgo/v2/ginkgo $(GINKGO_BIN) $(GINKGO_VER)

$(GO_APIDIFF):
	GOBIN=$(BIN_DIR) $(GO_INSTALL) $(GO_APIDIFF_PKG) $(GO_APIDIFF_BIN) $(GO_APIDIFF_VER)

$(SETUP_ENVTEST):
	GOBIN=$(BIN_DIR) $(GO_INSTALL) sigs.k8s.io/controller-runtime/tools/setup-envtest $(SETUP_ENVTEST_BIN) $(SETUP_ENVTEST_VER)

.dapper:
	@echo Downloading dapper
	@curl -sL https://releases.rancher.com/dapper/latest/dapper-`uname -s`-`uname -m` > .dapper.tmp
	@@chmod +x .dapper.tmp
	@./.dapper.tmp -v
	@mv .dapper.tmp .dapper

$(MOCKGEN):
	GOBIN=$(BIN_DIR) $(GO_INSTALL) $(MOCKGEN_PKG) $(MOCKGEN_BIN) $(MOCKGEN_VER)

$(GINKGO):
	GOBIN=$(BIN_DIR) $(GO_INSTALL) $(GINKGO_PKG) $(GINKGO_BIN) $(GINKGO_VER)

$(GO_APIDIFF):
	GOBIN=$(BIN_DIR) $(GO_INSTALL) $(GO_APIDIFF_PKG) $(GO_APIDIFF_BIN) $(GO_APIDIFF_VER)

.PHONY: $(TARGETS)
$(TARGETS): .dapper
	./.dapper $@

.PHONY: generate-go
generate-go: $(MOCKGEN)
	go generate ./pkg/gke/...

.PHONY: test
test: $(SETUP_ENVTEST) $(GINKGO)
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" $(GINKGO) -v -r -p --trace --race ./pkg/... ./controller/...

.PHONY: generate-crd
generate-crd: $(MOCKGEN)
	go generate main.go

.PHONY: generate
generate:
	$(MAKE) generate-go
	$(MAKE) generate-crd

.PHONY: operator
operator:
	go build -o bin/gke-operator main.go

ALL_VERIFY_CHECKS = generate

.PHONY: verify
verify: $(addprefix verify-,$(ALL_VERIFY_CHECKS))

.PHONY: verify-generate
verify-generate: generate
	@if !(git diff --quiet HEAD); then \
		git diff; \
		echo "generated files are out of date, run make generate"; exit 1; \
	fi

.PHONY: operator-chart
operator-chart:
	mkdir -p $(BIN_DIR)
	cp -rf $(ROOT_DIR)/charts/gke-operator $(BIN_DIR)/chart
	sed -i -e 's/tag:.*/tag: '${TAG}'/' $(BIN_DIR)/chart/values.yaml
	sed -i -e 's|repository:.*|repository: '${REPO}'|' $(BIN_DIR)/chart/values.yaml
	helm package --version ${CHART_VERSION} --app-version ${GIT_TAG} -d $(BIN_DIR)/ $(BIN_DIR)/chart
	rm -Rf $(BIN_DIR)/chart
	
.PHONY: crd-chart
crd-chart:
	mkdir -p $(BIN_DIR)
	helm package --version ${CHART_VERSION} --app-version ${GIT_TAG} -d $(BIN_DIR)/ $(ROOT_DIR)/charts/gke-operator-crd
	rm -Rf $(BIN_DIR)/chart

.PHONY: charts
charts:
	$(MAKE) operator-chart
	$(MAKE) crd-chart

.PHONY: setup-kind
setup-kind:
	CLUSTER_NAME=$(CLUSTER_NAME) $(ROOT_DIR)/scripts/setup-kind-cluster.sh

.PHONY: e2e-tests
e2e-tests: $(GINKGO) charts
	export EXTERNAL_IP=`kubectl get nodes -o jsonpath='{.items[].status.addresses[?(@.type == "InternalIP")].address}'` && \
	export BRIDGE_IP="172.18.0.1" && \
	export CONFIG_PATH=$(E2E_CONF_FILE) && \
	export OPERATOR_CHART=$(OPERATOR_CHART) && \
	export CRD_CHART=$(CRD_CHART) && \
	cd $(ROOT_DIR)/test && $(GINKGO) $(ONLY_DEPLOY) -r -v ./e2e

.PHONY: kind-e2e-tests
kind-e2e-tests: docker-build-e2e setup-kind
	kind load docker-image --name $(CLUSTER_NAME) ${REPO}:${TAG}
	$(MAKE) e2e-tests

kind-deploy-operator:
	ONLY_DEPLOY="--label-filter=\"do-nothing\"" $(MAKE) kind-e2e-tests

.PHONY: docker-build
docker-build-e2e:
	DOCKER_BUILDKIT=1 docker build \
		-f test/e2e/Dockerfile.e2e \
		--build-arg "TAG=${GIT_TAG}" \
		--build-arg "COMMIT=${GIT_COMMIT}" \
		--build-arg "COMMITDATE=${COMMITDATE}" \
		-t ${REPO}:${TAG} .

.PHOHY: delete-local-kind-cluster
delete-local-kind-cluster: ## Delete the local kind cluster
	kind delete cluster --name=$(CLUSTER_NAME)

.PHONY: clean
clean:
	rm -rf build bin dist
