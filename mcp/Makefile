# Makefile for building and publishing the Diffusion MCP Server container

# Variables
REGISTRY := ghcr.io
ORG := polar-team
IMAGE_NAME := diffusion-mcp-server
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed -E 's/^v?([0-9]+\.[0-9]+\.[0-9]+).*/\1/' || echo "0.0.0")
CACHE_PATH ?= ./cache
TEST_CONTAINER := false

# Path to the diffusion Go source (parent repo by default)
DIFFUSION_SRC ?= ../..

ifeq ($(TEST_CONTAINER), true)
	TEST_SUFFIX := -test
endif

# Full image reference
IMAGE := $(REGISTRY)/$(ORG)/$(IMAGE_NAME)

# Platforms
PLATFORMS := linux/amd64,linux/arm64

# Build args passed to docker build
BUILD_ARGS := --build-arg DIFFUSION_VERSION=$(VERSION)

.PHONY: help
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-25s %s\n", $$1, $$2}'

# ---------------------------------------------------------------------------
# Prepare build context: copy diffusion Go source into diffusion_src/
# ---------------------------------------------------------------------------
.PHONY: prepare-context
prepare-context:
	@echo "Preparing build context — copying diffusion Go source..."
	@rm -rf diffusion_src
	@mkdir -p diffusion_src
	@cp -r $(DIFFUSION_SRC)/cmd diffusion_src/cmd
	@cp -r $(DIFFUSION_SRC)/internal diffusion_src/internal
	@cp $(DIFFUSION_SRC)/go.mod diffusion_src/go.mod
	@cp $(DIFFUSION_SRC)/go.sum diffusion_src/go.sum
	@echo "Build context ready."

.PHONY: clean-context
clean-context:
	@rm -rf diffusion_src

# ---------------------------------------------------------------------------
# Cache / registry helpers
# ---------------------------------------------------------------------------
.PHONY: check_cache
check_cache:
	@echo "Checking cache at $(CACHE_PATH)..."
ifneq ($(wildcard ${CACHE_PATH}/index.json),)
	@echo "Cache found."
else
	@echo "No cache found. Creating firstinit file."
	@[ -d ./cache ] || mkdir -p ./cache
	@touch ${CACHE_PATH}/firstinit
endif

.PHONY: login
login: ## Login to GitHub Container Registry
	@echo "Logging in to $(REGISTRY)..."
ifeq ($(OS),Windows_NT)
	@powershell -Command 'echo "$$env:GITHUB_TOKEN" | docker login $(REGISTRY) -u "$$env:GITHUB_ACTOR" --password-stdin'
else
	@echo "$$GITHUB_TOKEN" | docker login $(REGISTRY) -u "$$GITHUB_ACTOR" --password-stdin
endif

.PHONY: setup-buildx
setup-buildx: ## Setup Docker Buildx for multi-platform builds
	@echo "Setting up Docker Buildx..."
	@docker buildx inspect multiplatform-builder > /dev/null 2>&1 || \
		docker buildx create --name multiplatform-builder --driver docker-container --use
	@docker buildx inspect --bootstrap

# ---------------------------------------------------------------------------
# Build targets
# ---------------------------------------------------------------------------
.PHONY: build-and-push-separate
build-and-push-separate: prepare-context ## Build and push with separate tags per architecture
ifneq ($(wildcard ${CACHE_PATH}/firstinit),)
	@echo "Building and pushing $(IMAGE) with architecture-specific tags..."
	docker buildx build \
		--platform linux/amd64 \
		--cache-to=type=local,dest=${CACHE_PATH} \
		$(BUILD_ARGS) \
		-t $(IMAGE):$(VERSION)-amd64$(TEST_SUFFIX) \
		-t $(IMAGE):latest-amd64$(TEST_SUFFIX) \
		--push \
		.
	docker buildx build \
		--platform linux/arm64 \
		--cache-to=type=local,dest=${CACHE_PATH} \
		$(BUILD_ARGS) \
		-t $(IMAGE):$(VERSION)-arm64$(TEST_SUFFIX) \
		-t $(IMAGE):latest-arm64$(TEST_SUFFIX) \
		--push \
		.
	@rm -f ${CACHE_PATH}/firstinit
else
	@echo "Building and pushing $(IMAGE) with architecture-specific tags using cache..."
	docker buildx build \
		--platform linux/amd64 \
		--cache-from=type=local,src=${CACHE_PATH} \
		--cache-to=type=local,dest=${CACHE_PATH} \
		$(BUILD_ARGS) \
		-t $(IMAGE):$(VERSION)-amd64$(TEST_SUFFIX) \
		-t $(IMAGE):latest-amd64$(TEST_SUFFIX) \
		--push \
		.
	docker buildx build \
		--platform linux/arm64 \
		--cache-from=type=local,src=${CACHE_PATH} \
		--cache-to=type=local,dest=${CACHE_PATH} \
		$(BUILD_ARGS) \
		-t $(IMAGE):$(VERSION)-arm64$(TEST_SUFFIX) \
		-t $(IMAGE):latest-arm64$(TEST_SUFFIX) \
		--push \
		.
endif
	@$(MAKE) clean-context

.PHONY: publish
publish: check_cache setup-buildx login build-and-push-separate ## Full pipeline: cache → buildx → login → build+push

.PHONY: build-local
build-local: prepare-context ## Build image locally for current platform
	@echo "Building local image for testing..."
	docker build $(BUILD_ARGS) -t $(IMAGE_NAME):local .
	@$(MAKE) clean-context
	@echo "Image built: $(IMAGE_NAME):local"
	@echo "Test with: docker run --rm -v \$$(pwd):/project $(IMAGE_NAME):local diffusion --version"

.PHONY: test-local
test-local: build-local ## Build and test image locally
	@echo "Testing MCP server module loads..."
	@docker run --rm $(IMAGE_NAME):local python -c "from diffusion_mcp.server import mcp; print('MCP server OK')"
	@echo "Testing Docker CLI is available..."
	@docker run --rm $(IMAGE_NAME):local docker --version
	@echo "Testing diffusion binary is available..."
	@docker run --rm $(IMAGE_NAME):local diffusion --version
	@echo "✅ All local tests passed"

.PHONY: build-and-save
build-and-save: prepare-context ## Build and save image to tar file
	@echo "Building and saving image..."
	docker build $(BUILD_ARGS) -t $(IMAGE_NAME):local .
	@$(MAKE) clean-context
	@mkdir -p ./images
	docker save $(IMAGE_NAME):local -o ./images/$(IMAGE_NAME)-local.tar
	@echo "Image saved to: ./images/$(IMAGE_NAME)-local.tar"
	@echo "Load with: docker load -i ./images/$(IMAGE_NAME)-local.tar"

.PHONY: load-local
load-local: ## Load saved image from tar file
	@echo "Loading image from local repository..."
	@if [ -f ./images/$(IMAGE_NAME)-local.tar ]; then \
		docker load -i ./images/$(IMAGE_NAME)-local.tar; \
		echo "Image loaded: $(IMAGE_NAME):local"; \
	else \
		echo "Error: ./images/$(IMAGE_NAME)-local.tar not found"; \
		echo "Run 'make build-and-save' first"; \
		exit 1; \
	fi

.PHONY: clean
clean: clean-context ## Remove buildx builder and build context
	@echo "Cleaning up buildx builder..."
	-docker buildx rm multiplatform-builder

.PHONY: clean-local
clean-local: ## Remove local images and saved tar files
	@echo "Cleaning up local images..."
	-docker rmi $(IMAGE_NAME):local
	-docker rmi $(IMAGE):test
	@echo "Cleaning up saved images..."
	-rm -rf ./images
	@echo "Local cleanup complete"

.PHONY: show-platforms
show-platforms: ## Show configured platforms and tags
	@echo "Configured platforms: $(PLATFORMS)"
	@echo "Image: $(IMAGE):$(VERSION)"
	@echo ""
	@echo "Architecture-specific tags:"
	@echo "  - $(IMAGE):$(VERSION)-amd64"
	@echo "  - $(IMAGE):latest-amd64"
	@echo "  - $(IMAGE):$(VERSION)-arm64"
	@echo "  - $(IMAGE):latest-arm64"
	@echo ""
	@echo "Multi-arch manifest tags (created by CI):"
	@echo "  - $(IMAGE):$(VERSION)"
	@echo "  - $(IMAGE):latest"

# Default target
.DEFAULT_GOAL := help
