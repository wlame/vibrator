# Makefile for vibrator (Go rewrite).
#
# Conventions:
#   - All commands run from the repo root.
#   - VERSION defaults to "dev"; pass VERSION=x.y.z for release builds.
#   - The integration target requires a real docker daemon; skipped in CI by
#     default unless you set INTEGRATION=1.

SHELL    := /bin/bash
.DEFAULT_GOAL := help

GO       ?= go
BIN_DIR  := build
BINARY   := $(BIN_DIR)/vibrate
PKG      := github.com/wlame/vibrator
MOD      := $(PKG)
CMD      := ./cmd/vibrate

VERSION  ?= dev
LDFLAGS  := -s -w -X $(MOD)/internal/cli.Version=$(VERSION)

# Used by `make integration`. Skipped silently when not set.
INTEGRATION ?=

.PHONY: build test lint integration clean tidy help fmt vet

build: $(BINARY)  ## Build the vibrate binary into ./build/

$(BINARY): $(shell find . -name '*.go' -not -path './previous-implementation/*' 2>/dev/null) go.mod
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $@ $(CMD)
	@echo "Built: $@ ($(VERSION))"

test:  ## Run unit tests
	$(GO) test -race -timeout=60s ./...

fmt:  ## Run gofmt -w
	$(GO) fmt ./...

vet:  ## Run go vet
	$(GO) vet ./...

lint: vet  ## Run go vet (+ golangci-lint if available)
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed — skipping (vet only). Install: https://golangci-lint.run/"; \
	fi

integration:  ## Run integration tests (requires real docker daemon)
ifndef INTEGRATION
	@echo "Integration tests skipped (set INTEGRATION=1 to enable)"
else
	$(GO) test -race -tags=integration -timeout=10m ./...
endif

tidy:  ## Run go mod tidy
	$(GO) mod tidy

clean:  ## Remove build artifacts
	rm -rf $(BIN_DIR)

help:  ## Show this help
	@echo "vibrator (Go) — Make targets"
	@echo
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*##"}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
	@echo
	@echo "Usage: make build VERSION=x.y.z"
