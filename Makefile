SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help
.PHONY: help build web-build check-toolchain shell-test lint-shell lint-workflows docs-contract format fmt vet test test-race integration cover clean docker-build \
        docker-run run validate sandbox sandbox-up sandbox-provision sandbox-down sandbox-logs sandbox-config sandbox-destroy

COMPOSE := docker compose -f sandbox/docker-compose.yml

# --- Project --------------------------------------------------------------

BINARY      := aiproxy
MAIN_PKG    := ./cmd/aiproxy
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LD_FLAGS    := -buildid= -s -w -X main.version=$(VERSION)
GO_BUILD_FLAGS := -trimpath -buildvcs=false
DIST_DIR    := dist
PREFIX      := aiproxy
BUILD_GO    ?= go

# --- Help -----------------------------------------------------------------

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) \
	  | awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# --- Local Go build -------------------------------------------------------

web-build: ## Build the dashboard web UI into internal/webui/dist
ifndef WEB_SKIP
	@pnpm --filter @aiproxy/web-ui build
	@touch internal/webui/dist/.gitkeep
endif

build: web-build ## Build the aiproxy binary into dist/ (WEB_SKIP=1 skips the web UI)
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 $(BUILD_GO) build $(GO_BUILD_FLAGS) -ldflags "$(LD_FLAGS)" -o $(DIST_DIR)/$(BINARY) $(MAIN_PKG)
	@echo "built $(DIST_DIR)/$(BINARY) (version $(VERSION))"

check-toolchain: ## Validate declared Go and pnpm tool versions stay aligned
	@./scripts/check-toolchain.sh --self-test

shell-test: ## Test the public asdf plugin scripts
	@bats test/asdf-plugin.bats

lint-shell: ## Validate shell scripts
	@shellcheck bin/* scripts/*.sh test/*.bats

lint-workflows: ## Validate GitHub Actions workflows
	@actionlint

docs-contract: ## Validate high-drift public docs contract tables stay aligned
	@./scripts/check-doc-contracts.sh

# --- Quality --------------------------------------------------------------

format fmt: ## Run gofmt -s on all Go sources
	@gofmt -w -s .

vet: ## Run go vet on all packages
	@go vet ./...

test: ## Run all unit tests
	@go test ./...

test-race: ## Run tests with the race detector
	@go test -race ./...

integration: build ## Run hermetic binary-level integration tests
	@AIPROXY_BINARY="$(CURDIR)/$(DIST_DIR)/$(BINARY)" go test -tags=integration ./internal/integration

cover: ## Run tests with coverage report
	@mkdir -p "$(DIST_DIR)"
	@go test -coverprofile=$(DIST_DIR)/coverage.out ./...
	@go tool cover -func=$(DIST_DIR)/coverage.out | tail -1
	@echo "coverage profile: $(DIST_DIR)/coverage.out"

clean: ## Remove dist/ and coverage artifacts
	@rm -rf $(DIST_DIR)
	@echo "cleaned $(DIST_DIR)"

# --- Run / validate -------------------------------------------------------

run: ## Run the server locally (CONFIG=path/to/config.hcl)
	@if [ -z "$(CONFIG)" ]; then echo "usage: make run CONFIG=path/to/config.hcl"; exit 1; fi
	@config="$(CONFIG)"; config=$$(cd -- "$$(dirname -- "$$config")" && pwd -P)/$$(basename -- "$$config"); \
	  go run $(MAIN_PKG) serve --config "$$config"

validate: ## Validate config without starting the server (CONFIG=path/to/config.hcl)
	@if [ -z "$(CONFIG)" ]; then echo "usage: make validate CONFIG=path/to/config.hcl"; exit 1; fi
	@config="$(CONFIG)"; config=$$(cd -- "$$(dirname -- "$$config")" && pwd -P)/$$(basename -- "$$config"); \
	  go run $(MAIN_PKG) validate --config "$$config"

# --- Sandbox --------------------------------------------------------------

sandbox: sandbox-up ## Start postgres and print connection info

sandbox-up: ## Start sandbox postgres (add PROFILES=keycloak for Keycloak)
	$(COMPOSE) up -d --wait postgres

sandbox-provision: ## Provision the local Keycloak realm and client (future OIDC)
	$(COMPOSE) --profile keycloak up --build keycloak-provision

sandbox-down: ## Stop sandbox containers without removing data
	$(COMPOSE) down

sandbox-logs: ## Follow sandbox logs
	$(COMPOSE) logs --tail=100 --follow

sandbox-config: ## Validate the Compose configuration
	$(COMPOSE) config --quiet

sandbox-destroy: ## Remove sandbox data (requires CONFIRM_DESTROY=1)
	@test "$(CONFIRM_DESTROY)" = "1" || (printf '%s\n' 'Refusing to remove sandbox data. Re-run with CONFIRM_DESTROY=1.' >&2; exit 1)
	$(COMPOSE) down --volumes --remove-orphans

# --- Docker ---------------------------------------------------------------

docker-build: ## Build the container image as $(PREFIX):$(VERSION)
	@docker build \
	  --build-arg VERSION=$(VERSION) \
	  -t $(PREFIX):$(VERSION) \
	  -t $(PREFIX):latest \
	  -f Dockerfile .

docker-run: ## Run the container image with a mounted config (CONFIG=path/to/config.hcl)
	@if [ -z "$(CONFIG)" ]; then echo "usage: make docker-run CONFIG=path/to/config.hcl"; exit 1; fi
	@set -e; env_args=(); if [ -f .env ]; then env_args=(--env-file .env); fi; \
	  config="$(CONFIG)"; config=$$(cd -- "$$(dirname -- "$$config")" && pwd -P)/$$(basename -- "$$config"); \
	  docker run --rm -p 8080:8080 -v "$$config:/etc/aiproxy/config.hcl:ro" \
	  "$${env_args[@]}" "$(PREFIX):latest"
