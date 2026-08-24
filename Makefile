SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help
.PHONY: help build build-all build-single build-archive validate-archives \
        validate-build-atomicity check-reproducible-archives check-toolchain shell-test lint-shell lint-workflows docs-contract format fmt vet test test-race integration cover clean docker-build \
        docker-run run validate

# --- Project --------------------------------------------------------------

BINARY      := aiproxy
MAIN_PKG    := ./cmd/aiproxy
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LD_FLAGS    := -buildid= -s -w -X main.version=$(VERSION)
GO_BUILD_FLAGS := -trimpath -buildvcs=false
SOURCE_DATE_EPOCH ?= 0
DIST_DIR    := dist
PREFIX      := aiproxy
BUILD_GO    ?= go

# --- Cross-compile matrix -------------------------------------------------

OS_ARCH_PAIRS := \
    linux:amd64 \
    linux:arm64 \
    linux:386 \
    linux:arm \
    windows:amd64 \
    windows:386 \
    darwin:amd64 \
    darwin:arm64 \
    freebsd:amd64 \
    freebsd:arm64 \
    openbsd:amd64 \
    openbsd:arm64 \
    netbsd:amd64

# --- Help -----------------------------------------------------------------

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) \
	  | awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# --- Local Go build -------------------------------------------------------

build: ## Build the aiproxy binary into dist/
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 $(BUILD_GO) build $(GO_BUILD_FLAGS) -ldflags "$(LD_FLAGS)" -o $(DIST_DIR)/$(BINARY) $(MAIN_PKG)
	@echo "built $(DIST_DIR)/$(BINARY) (version $(VERSION))"

build-single: ## Build for a single OS:ARCH pair (OS_ARCH=linux:amd64)
	@set -e; \
	if [ -z "$(OS_ARCH)" ]; then echo "OS_ARCH is required" >&2; exit 1; fi; \
	OS_ARCH=$(OS_ARCH); \
	OS=$${OS_ARCH%%:*}; \
	ARCH=$${OS_ARCH#*:}; \
	if [ -z "$$OS" ] || [ -z "$$ARCH" ] || [ "$$OS" = "$$ARCH" ]; then echo "invalid OS_ARCH=$$OS_ARCH" >&2; exit 1; fi; \
	echo "Building for OS=$$OS and ARCH=$$ARCH"; \
	DIR="$(DIST_DIR)/$$OS-$$ARCH"; \
	EXT=$$(if [ "$$OS" = "windows" ]; then echo ".exe"; else echo ""; fi); \
	TMP="$(DIST_DIR)/.tmp-$$OS-$$ARCH-$$$$"; \
	rm -rf "$$TMP" "$$DIR"; \
	mkdir -p "$(DIST_DIR)" "$$TMP"; \
	trap 'rm -rf "'"$$TMP"'"' EXIT; \
	CGO_ENABLED=0 GOOS=$$OS GOARCH=$$ARCH \
	  $(BUILD_GO) build $(GO_BUILD_FLAGS) -ldflags "$(LD_FLAGS) -X main.version=$(VERSION)/$$OS-$$ARCH" \
	  -o "$$TMP/$(BINARY)$$EXT" $(MAIN_PKG); \
	if [ ! -s "$$TMP/$(BINARY)$$EXT" ]; then echo "missing or empty executable for $$OS_ARCH" >&2; exit 1; fi; \
	mv "$$TMP" "$$DIR"; \
	trap - EXIT

build-all: ## Cross-compile for all OS/arch pairs in OS_ARCH_PAIRS
	@set -e; \
	for pair in $(OS_ARCH_PAIRS); do \
	  $(MAKE) build-single OS_ARCH=$$pair; \
	done

build-archive: ## Tar each cross-compiled dist/<os>-<arch>/ dir into a release archive
	@set -e; \
	for pair in $(OS_ARCH_PAIRS); do \
	  OS=$${pair%%:*}; \
	  ARCH=$${pair#*:}; \
	  EXT=$$(if [ "$$OS" = "windows" ]; then echo ".exe"; else echo ""; fi); \
	  name="$$OS-$$ARCH"; \
	  d="$(DIST_DIR)/$$name"; \
	  exe="$$d/$(BINARY)$$EXT"; \
	  if [ ! -d "$$d" ]; then echo "missing target directory $$d" >&2; exit 1; fi; \
	  if [ ! -f "$$exe" ] || [ ! -s "$$exe" ]; then echo "missing or empty expected executable $$exe" >&2; exit 1; fi; \
	  shopt -s nullglob dotglob; entries=("$$d"/*); shopt -u nullglob dotglob; \
	  if [ "$${#entries[@]}" -ne 1 ] || [ "$${entries[0]}" != "$$exe" ]; then echo "target $$d must contain only $(BINARY)$$EXT" >&2; exit 1; fi; \
	  archive="$(DIST_DIR)/$(PREFIX)-$$name.tar.gz"; \
	  tar --sort=name --mtime="@$(SOURCE_DATE_EPOCH)" --owner=0 --group=0 --numeric-owner \
	    -cf - -C "$$d" "$(BINARY)$$EXT" | gzip -n > "$$archive"; \
	  echo "archived $$archive"; \
	done

validate-archives: ## Validate each release archive contains exactly one expected executable
	@set -e; \
	for pair in $(OS_ARCH_PAIRS); do \
	  OS=$${pair%%:*}; \
	  ARCH=$${pair#*:}; \
	  EXT=$$(if [ "$$OS" = "windows" ]; then echo ".exe"; else echo ""; fi); \
	  name="$$OS-$$ARCH"; \
	  archive="$(DIST_DIR)/$(PREFIX)-$$name.tar.gz"; \
	  if [ ! -f "$$archive" ] || [ ! -s "$$archive" ]; then echo "missing or empty archive $$archive" >&2; exit 1; fi; \
	  TMP=$$(mktemp -d); \
	  trap 'rm -rf "'"$$TMP"'"' EXIT; \
	  members=$$(tar -tzf "$$archive"); \
	  if [ "$$members" != "$(BINARY)$$EXT" ]; then echo "archive $$archive must contain only $(BINARY)$$EXT" >&2; exit 1; fi; \
	  details=$$(tar -tvzf "$$archive"); \
	  case "$$details" in -*) ;; *) echo "archive $$archive member must be a regular file" >&2; exit 1 ;; esac; \
	  tar --no-same-owner --no-same-permissions -xzf "$$archive" -C "$$TMP"; \
	  shopt -s nullglob dotglob; entries=("$$TMP"/*); shopt -u nullglob dotglob; \
	  if [ "$${#entries[@]}" -ne 1 ] || [ "$${entries[0]}" != "$$TMP/$(BINARY)$$EXT" ] || [ ! -f "$${entries[0]}" ] || [ ! -s "$${entries[0]}" ]; then echo "archive $$archive must contain exactly one non-empty $(BINARY)$$EXT" >&2; exit 1; fi; \
	  rm -rf "$$TMP"; \
	  trap - EXIT; \
	  echo "validated $$archive"; \
	done

validate-build-atomicity: ## Validate cross-build failures and artifact checks fail closed
	@./scripts/validate-build-atomicity.sh

check-reproducible-archives: ## Verify two clean builds produce identical archives
	@./scripts/check-reproducible-archives.sh

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
