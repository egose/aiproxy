SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help
.PHONY: help build build-all build-single build-archive format fmt vet test test-race cover clean \
        docker-build docker-run run validate

# --- Project --------------------------------------------------------------

BINARY      := aiproxy
MAIN_PKG    := ./cmd/aiproxy
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LD_FLAGS    := -s -w -X main.version=$(VERSION)
DIST_DIR    := dist
PREFIX      := aiproxy

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
	CGO_ENABLED=0 go build -ldflags "$(LD_FLAGS)" -o $(DIST_DIR)/$(BINARY) $(MAIN_PKG)
	@echo "built $(DIST_DIR)/$(BINARY) (version $(VERSION))"

build-single: ## Build for a single OS:ARCH pair (OS_ARCH=linux:amd64)
	@set -e; \
	OS_ARCH=$(OS_ARCH); \
	OS=$$(echo $$OS_ARCH | cut -d: -f1); \
	ARCH=$$(echo $$OS_ARCH | cut -d: -f2); \
	echo "Building for OS=$$OS and ARCH=$$ARCH"; \
	DIR="$(DIST_DIR)/$$OS-$$ARCH"; \
	mkdir -p $$DIR; \
	EXT=$$(if [ "$$OS" = "windows" ]; then echo ".exe"; else echo ""; fi); \
	CGO_ENABLED=0 GOOS=$$OS GOARCH=$$ARCH \
	  go build -ldflags "$(LD_FLAGS) -X main.version=$(VERSION)/$$OS-$$ARCH" \
	  -o $$DIR/$(BINARY)$$EXT $(MAIN_PKG)

build-all: ## Cross-compile for all OS/arch pairs in OS_ARCH_PAIRS
	@$(foreach pair,$(OS_ARCH_PAIRS),$(MAKE) build-single OS_ARCH=$(pair);)

build-archive: ## Tar each cross-compiled dist/<os>-<arch>/ dir into a release archive
	@set -e; \
	for d in $(DIST_DIR)/*-*/; do \
	  [ -d "$$d" ] || continue; \
	  name=$$(basename "$$d"); \
	  archive="$(DIST_DIR)/$(PREFIX)-$$name.tar.gz"; \
	  tar -czf "$$archive" -C "$$d" .; \
	  echo "archived $$archive"; \
	done

# --- Quality --------------------------------------------------------------

format fmt: ## Run gofmt -s on all Go sources
	@gofmt -w -s .

vet: ## Run go vet on all packages
	@go vet ./...

test: ## Run all unit tests
	@go test ./...

test-race: ## Run tests with the race detector
	@go test -race ./...

cover: ## Run tests with coverage report
	@go test -coverprofile=$(DIST_DIR)/coverage.out ./...
	@go tool cover -func=$(DIST_DIR)/coverage.out | tail -1
	@echo "coverage profile: $(DIST_DIR)/coverage.out"

clean: ## Remove dist/ and coverage artifacts
	@rm -rf $(DIST_DIR)
	@echo "cleaned $(DIST_DIR)"

# --- Run / validate -------------------------------------------------------

run: ## Run the server locally (CONFIG=path/to/config.hcl)
	@if [ -z "$(CONFIG)" ]; then echo "usage: make run CONFIG=path/to/config.hcl"; exit 1; fi
	@go run $(MAIN_PKG) serve --config $(CONFIG)

validate: ## Validate config without starting the server (CONFIG=path/to/config.hcl)
	@if [ -z "$(CONFIG)" ]; then echo "usage: make validate CONFIG=path/to/config.hcl"; exit 1; fi
	@go run $(MAIN_PKG) validate --config $(CONFIG)

# --- Docker ---------------------------------------------------------------

docker-build: ## Build the container image as $(PREFIX):$(VERSION)
	@docker build \
	  --build-arg VERSION=$(VERSION) \
	  -t $(PREFIX):$(VERSION) \
	  -t $(PREFIX):latest \
	  -f Dockerfile .

docker-run: ## Run the container image with a mounted config (CONFIG=path/to/config.hcl)
	@if [ -z "$(CONFIG)" ]; then echo "usage: make docker-run CONFIG=path/to/config.hcl"; exit 1; fi
	@docker run --rm -p 8080:8080 -v $(PWD)/$(CONFIG):/etc/aiproxy/config.hcl:ro \
	  --env-file .env \
	  $(PREFIX):latest
