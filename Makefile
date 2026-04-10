# chaosplane Makefile
# ──────────────────────────────────────────────────────────────────────────────

# Module
MODULE := github.com/chaosplane-hq/chaosplane

# Binary output
BIN_DIR := $(shell pwd)/bin
GOBIN   := $(BIN_DIR)

# Go
GOOS   ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Container registry
REGISTRY ?= ghcr.io/chaosplane-hq
TAG      ?= dev

# Tool versions
CONTROLLER_GEN_VERSION ?= v0.16.5
GOLANGCI_LINT_VERSION  ?= v1.62.2
GOFUMPT_VERSION        ?= v0.7.0
BUF_VERSION            ?= v1.47.2
KIND_VERSION           ?= v0.25.0

# kind cluster
KIND_CLUSTER_NAME ?= chaosplane-dev

# Binaries
COMPONENTS := operator agent daemon chaosctl

# ──────────────────────────────────────────────────────────────────────────────
# Help
# ──────────────────────────────────────────────────────────────────────────────

.DEFAULT_GOAL := help

##@ General

.PHONY: help
help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
		/^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Development

.PHONY: setup
setup: ## Install tools (controller-gen, golangci-lint, gofumpt, buf) + kind cluster
	@echo "Installing tools to $(BIN_DIR)..."
	@mkdir -p $(BIN_DIR)
	GOBIN=$(GOBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
	GOBIN=$(GOBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	GOBIN=$(GOBIN) go install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)
	GOBIN=$(GOBIN) go install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)
	@echo "Setting up kind cluster..."
	@bash hack/setup-kind.sh

.PHONY: dev
dev: ## Start Tilt dev server
	tilt up

##@ Build

.PHONY: build
build: ## Build all binaries (operator, agent, daemon, chaosctl) to bin/
	@mkdir -p $(BIN_DIR)
	$(foreach comp,$(COMPONENTS),\
		CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BIN_DIR)/$(comp) ./cmd/$(comp)/...;)

.PHONY: build-%
build-%: ## Build a specific binary (e.g. make build-operator)
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BIN_DIR)/$* ./cmd/$*/...

##@ Test

.PHONY: test
test: ## Unit tests with coverage
	go test -race -coverprofile=cover.out -covermode=atomic ./...

.PHONY: test-integration
test-integration: ## Integration tests
	go test -race -tags=integration -count=1 ./test/integration/...

.PHONY: e2e
e2e: ## E2E tests (kind)
	go test -race -tags=e2e -count=1 -timeout=30m ./test/e2e/...

##@ Code Quality

.PHONY: lint
lint: ## golangci-lint run
	golangci-lint run ./...

.PHONY: fmt
fmt: ## gofumpt -w .
	gofumpt -w .

##@ Code Generation

.PHONY: generate
generate: ## controller-gen deepcopy
	controller-gen object paths="./..."

.PHONY: manifests
manifests: ## controller-gen CRD + RBAC + Webhook YAML
	controller-gen crd rbac:roleName=chaosplane-operator webhook paths="./..." \
		output:crd:artifacts:config=config/crd/bases \
		output:rbac:artifacts:config=config/rbac \
		output:webhook:artifacts:config=config/webhook

##@ Container Images

.PHONY: docker-build
docker-build: ## Build all Docker images
	$(foreach comp,$(COMPONENTS),\
		docker build -t $(REGISTRY)/$(comp):$(TAG) -f Dockerfile.$(comp) .;)

.PHONY: docker-build-%
docker-build-%: ## Build a specific Docker image (e.g. make docker-build-operator)
	docker build -t $(REGISTRY)/$*:$(TAG) -f Dockerfile.$* .

.PHONY: docker-push
docker-push: ## Push to ghcr.io/chaosplane-hq
	$(foreach comp,$(COMPONENTS),\
		docker push $(REGISTRY)/$(comp):$(TAG);)

.PHONY: docker-push-%
docker-push-%: ## Push a specific Docker image (e.g. make docker-push-operator)
	docker push $(REGISTRY)/$*:$(TAG)

##@ Cleanup

.PHONY: clean
clean: ## Remove bin/, cover.out
	rm -rf $(BIN_DIR) cover.out
