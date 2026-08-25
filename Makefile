.PHONY: menu targets help setup doctor install-go shell all build eval fmt-check test vet check bench \
	demo demo-edge docker-build docker-eval docker-demo compose-build compose-eval \
	compose-demo clean

# Image tag used by Docker and Compose targets.
IMAGE_NAME ?= verifoxx:local

# Prefer the repository-local Go toolchain, then fall back to Go on PATH.
GO := ./scripts/go.sh
BINARY_NAME := bin/verifoxx

# Discovery
# Target descriptions live beside their recipes. scripts/menu.sh reads these
# `##` comments for --list, the interactive picker, readiness, and previews.
menu: ## Open the interactive fzf target picker (numbered fallback)
	@bash scripts/menu.sh

targets: ## List every described Make target for scripts and discovery
	@bash scripts/menu.sh --list | awk -F '\t' '{printf "  %-14s %s\n", $$1, $$2}'

help: ## Show every Make target and workflow convention
	@printf 'Verifoxx development targets:\n'
	@bash scripts/menu.sh --list | awk -F '\t' '{printf "  %-14s %s\n", $$1, $$2}'
	@printf '\n  Go: .tools/go first, then PATH. fzf is optional.\n'
	@printf '  Container image: IMAGE_NAME (default %s).\n' "$(IMAGE_NAME)"

# Setup
setup: doctor ## Check prerequisites, then download Go modules
	$(GO) mod download

doctor: ## Report required and optional workflow dependencies
	./scripts/doctor.sh

install-go: ## Reuse installed Go; otherwise install Go 1.27 under .tools/go
	./scripts/install-go.sh

shell: ## Install the global cross-shell mm shortcut
	$(GO) run ./cmd/mm --install

# Build and evaluation
all: build eval ## Build, then regenerate the supplied-pack result

build: ## Compile bin/verifoxx
	@mkdir -p bin
	$(GO) build -o $(BINARY_NAME) ./cmd/verifoxx

eval: build ## Evaluate the supplied pack into results/requests.json
	./$(BINARY_NAME) --policy policies/policy.json --requests fixtures/requests.json --evidence fixtures/evidence.json --output results/requests.json

# Quality
fmt-check: ## Verify gofmt formatting under cmd/ and internal/
	./scripts/gofmt-check.sh

test: ## Run fresh Go tests and workflow regressions
	$(GO) test -count=1 -timeout 60s ./...
	./scripts/doctor-selftest.sh
	./scripts/install-go-idempotence-test.sh
	./scripts/install-go-rollback-test.sh
	./scripts/menu-selftest.sh

vet: ## Run go vet across the module
	$(GO) vet ./...

check: fmt-check test vet build ## Run format checks, tests, vet, and build

# Performance
bench: ## Run representative lifecycle benchmarks with allocation metrics
	./scripts/bench.sh

# Reviewer demos
demo: check ## Verify supplied and edge packs against tracked goldens
	./scripts/demo.sh

demo-edge: build ## Verify only the edge-case pack against its golden
	./scripts/demo.sh --edge-only

# Docker
docker-build: ## Build the multi-stage Docker image
	@docker build -t "$(IMAGE_NAME)" . 1>&2

docker-eval: docker-build ## Evaluate the supplied pack in Docker (JSON stdout)
	@docker run --rm "$(IMAGE_NAME)"

docker-demo: ## Build once and verify both packs through Docker
	IMAGE_NAME="$(IMAGE_NAME)" ./scripts/docker-demo.sh

# Docker Compose
compose-build: ## Build the image through compose.yaml
	@IMAGE_NAME="$(IMAGE_NAME)" docker compose -f compose.yaml build 1>&2

compose-eval: compose-build ## Evaluate the supplied pack through Compose (JSON stdout)
	@IMAGE_NAME="$(IMAGE_NAME)" docker compose -f compose.yaml run --rm -T --no-deps verifoxx

compose-demo: ## Build once and verify both packs through Compose
	@IMAGE_NAME="$(IMAGE_NAME)" ./scripts/compose-demo.sh

# Cleanup
clean: ## Remove bin/ while preserving tracked results
	rm -rf bin
