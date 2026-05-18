# justfile for vibrator (Go rewrite).
#
# Run `just` (no args) to list available recipes.
# Run `just <recipe>` to execute one. Use `just --show <recipe>` to see its
# source.
#
# Conventions:
#   - All recipes assume the repo root as cwd.
#   - VERSION defaults to "dev"; override with `VERSION=x.y.z just build`.
#   - The `integration` recipe requires a real docker daemon and is skipped
#     unless INTEGRATION=1 is set in the environment.

# --- Settings ---

# Recipe arguments are passed to the shell as positional args ($1, $2, ...).
# Lets shebang recipes (#!/usr/bin/env bash) use "$@" naturally.
set positional-arguments

# Use bash for linewise recipes — POSIX `sh` would force us to lose
# `[[ ]]` and a few other niceties. Shebang recipes ignore this and pick
# their own interpreter.
set shell := ["bash", "-cu"]

# --- Variables ---

go      := "go"
bin_dir := "build"
binary  := bin_dir / "vibrate"
pkg     := "github.com/wlame/vibrator"
cmd     := "./cmd/vibrate"

# VERSION baked into the binary via -ldflags. Defaults to "dev"; release
# builds should pass `VERSION=0.2.0 just build` or set it in CI.
version := env("VERSION", "dev")
ldflags := "-s -w -X " + pkg + "/internal/cli.Version=" + version

# --- Recipes ---

# Default recipe — show the list of recipes when `just` is invoked bare.
default:
    @just --list --list-heading $'vibrator (Go) — Just recipes\n'

# Build the vibrate binary into ./build/
build:
    @mkdir -p {{bin_dir}}
    {{go}} build -trimpath -ldflags '{{ldflags}}' -o {{binary}} {{cmd}}
    @echo "Built: {{binary}} ({{version}})"

# Run unit tests with the race detector
test:
    {{go}} test -race -timeout=60s ./...

# Run unit tests with coverage profile written to coverage.out
test-cover:
    {{go}} test -race -timeout=60s -coverprofile=coverage.out ./...
    @echo "Coverage profile: coverage.out (use 'go tool cover -html=coverage.out')"

# Run gofmt -w on the tree
fmt:
    {{go}} fmt ./...

# Run go vet
vet:
    {{go}} vet ./...

# Run go vet + golangci-lint (if installed)
lint: vet
    #!/usr/bin/env bash
    set -euo pipefail
    if command -v golangci-lint >/dev/null 2>&1; then
      golangci-lint run ./...
    else
      echo "golangci-lint not installed — skipping (vet only)."
      echo "Install: https://golangci-lint.run/"
    fi

# Run integration tests (real docker daemon required; set INTEGRATION=1)
integration:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ -z "${INTEGRATION:-}" ]]; then
      echo "Integration tests skipped (set INTEGRATION=1 to enable)"
      exit 0
    fi
    {{go}} test -race -tags=integration -timeout=10m ./...

# Run go mod tidy
tidy:
    {{go}} mod tidy

# Composite check — what CI runs on every PR
ci: lint test build

# Remove build artifacts
clean:
    rm -rf {{bin_dir}} coverage.out

# Build then run the binary with arbitrary args, e.g.:
#   just run runtime detect
#   just run --help
run *args: build
    {{binary}} {{args}}

# Print the versions of toolchain components — helps debug "works on my machine"
versions:
    @echo "just:     $(just --version)"
    @echo "go:       $({{go}} version)"
    @echo "vibrator: {{version}}"
