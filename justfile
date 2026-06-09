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

# Install the binary system-wide with a `vb` short alias + shell completion.
#
# Usage:
#   just install                  # → /usr/local/bin (uses sudo if needed)
#   just install ~/.local/bin     # → user-local, no sudo
#
# If `dest` is writable by the current user (e.g., ~/.local/bin),
# sudo is skipped and completion goes to user-level paths
# (~/.local/share/bash-completion/completions, ~/.zsh/completions,
# ~/.config/fish/completions). Otherwise sudo is used and completion
# lands in system locations.
#
# Detects $SHELL to pick which shell's completion to install. Pass
# --shell=name as an env override if your $SHELL doesn't match what
# you actually use (e.g., `SHELL=zsh just install`).
install dest="/usr/local/bin": build
    #!/usr/bin/env bash
    set -euo pipefail
    DEST="{{dest}}"
    # Expand a leading ~ since the shell here is invoked with -u and
    # tilde expansion happens at unquoted-arg parse time — values
    # forwarded as variables don't get expanded.
    DEST="${DEST/#\~/$HOME}"
    mkdir -p "$DEST" 2>/dev/null || true
    if [[ -w "$DEST" ]]; then
      SUDO=""
      USER_LEVEL=1
    else
      SUDO="sudo"
      USER_LEVEL=0
      echo "→ Destination $DEST is not user-writable; using sudo"
    fi

    # --- Binary + alias ---
    $SUDO install {{binary}} "$DEST/"
    $SUDO ln -sf "$DEST/vibrate" "$DEST/vb"
    echo "✓ Installed: $DEST/vibrate (alias: vb)"

    # --- Shell completion ---
    SHELL_NAME="$(basename "${SHELL:-}")"
    case "$SHELL_NAME" in
      bash)
        if [[ $USER_LEVEL -eq 1 ]]; then
          COMP_DIR="$HOME/.local/share/bash-completion/completions"
        else
          COMP_DIR="/etc/bash_completion.d"
        fi
        COMP_FILE="vibrate"
        ;;
      zsh)
        if [[ $USER_LEVEL -eq 1 ]]; then
          COMP_DIR="$HOME/.zsh/completions"
        else
          COMP_DIR="/usr/local/share/zsh/site-functions"
        fi
        COMP_FILE="_vibrate"
        ;;
      fish)
        if [[ $USER_LEVEL -eq 1 ]]; then
          COMP_DIR="$HOME/.config/fish/completions"
        else
          COMP_DIR="/usr/local/share/fish/vendor_completions.d"
        fi
        COMP_FILE="vibrate.fish"
        ;;
      *)
        echo "  (shell '$SHELL_NAME' has no auto-completion support — skipping)"
        echo "  Run \`vibrate completion <shell>\` manually to generate one."
        exit 0
        ;;
    esac

    $SUDO mkdir -p "$COMP_DIR"
    # Pipe through tee so the redirect happens under the correct
    # privilege level (sudo if needed). > redirection wouldn't.
    "$DEST/vibrate" completion "$SHELL_NAME" | $SUDO tee "$COMP_DIR/$COMP_FILE" > /dev/null
    echo "✓ Installed $SHELL_NAME completion: $COMP_DIR/$COMP_FILE"

    # zsh user-level completions only work if $COMP_DIR is in $fpath.
    # bash and fish auto-discover from the standard XDG paths.
    if [[ "$SHELL_NAME" == "zsh" && $USER_LEVEL -eq 1 ]]; then
      echo
      echo "  zsh note: ensure $COMP_DIR is in your fpath. Add to ~/.zshrc:"
      echo "    fpath=(\$HOME/.zsh/completions \$fpath)"
      echo "    autoload -U compinit && compinit"
    fi

# Remove the installed binary + alias + completion from `dest`.
# Same dest semantics as `install` — pass the same arg you installed with.
uninstall dest="/usr/local/bin":
    #!/usr/bin/env bash
    set -euo pipefail
    DEST="{{dest}}"
    DEST="${DEST/#\~/$HOME}"
    if [[ -w "$DEST" ]]; then
      SUDO=""
      USER_LEVEL=1
    else
      SUDO="sudo"
      USER_LEVEL=0
    fi

    $SUDO rm -f "$DEST/vibrate" "$DEST/vb"
    echo "✓ Removed: $DEST/vibrate and $DEST/vb"

    # Remove completion for whichever shell we'd have installed for.
    SHELL_NAME="$(basename "${SHELL:-}")"
    case "$SHELL_NAME" in
      bash)
        if [[ $USER_LEVEL -eq 1 ]]; then COMP="$HOME/.local/share/bash-completion/completions/vibrate"; else COMP="/etc/bash_completion.d/vibrate"; fi
        ;;
      zsh)
        if [[ $USER_LEVEL -eq 1 ]]; then COMP="$HOME/.zsh/completions/_vibrate"; else COMP="/usr/local/share/zsh/site-functions/_vibrate"; fi
        ;;
      fish)
        if [[ $USER_LEVEL -eq 1 ]]; then COMP="$HOME/.config/fish/completions/vibrate.fish"; else COMP="/usr/local/share/fish/vendor_completions.d/vibrate.fish"; fi
        ;;
      *)
        exit 0
        ;;
    esac
    if [[ -e "$COMP" ]]; then
      $SUDO rm -f "$COMP"
      echo "✓ Removed completion: $COMP"
    fi

# Composite check — what CI runs on every PR
ci: lint test build

# Build the release artifacts locally — the exact raw per-platform binaries +
# checksums.txt that CI attaches to a published GitHub Release (see
# .github/workflows/release.yml). Outputs to ./dist/. Nothing is tagged or
# pushed. Pass VERSION (without a leading "v") to stamp a real version:
#   VERSION=0.3.0 just dist
dist:
    #!/usr/bin/env bash
    set -euo pipefail
    rm -rf dist && mkdir -p dist
    # Same targets, ldflags, and naming as the release workflow. Pure-Go
    # (CGO disabled) cross-compiles cleanly from any host. Windows is omitted —
    # the code uses Unix-only syscalls and does not build for windows/*.
    platforms="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64"
    for p in $platforms; do
      goos="${p%/*}"
      goarch="${p#*/}"
      ext=""
      [[ "$goos" == "windows" ]] && ext=".exe"
      out="dist/vibrate_{{version}}_${goos}_${goarch}${ext}"
      echo "Building ${out}"
      CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
        {{go}} build -trimpath -ldflags '{{ldflags}}' -o "${out}" {{cmd}}
    done
    ( cd dist && sha256sum vibrate_* > checksums.txt )
    echo "✓ Release artifacts in ./dist/ (version: {{version}})"

# Remove build artifacts
clean:
    rm -rf {{bin_dir}} coverage.out dist/

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
