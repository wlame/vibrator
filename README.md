# Vibrator

Run Claude Code in YOLO mode inside Docker containers — automatic runtime detection, pre-configured MCP servers, security restrictions baked in.

[![Tests](https://img.shields.io/badge/tests-52%2F52-brightgreen)](./tests)
[![License](https://img.shields.io/badge/license-MIT-blue)](./LICENSE)

---

## Install

```bash
git clone https://github.com/wlame/vibrator.git
cd vibrator
make build
cp build/vibrate.sh /usr/local/bin/vibrate
```

Or download the latest `vibrate.sh` from releases and put it in your PATH.

---

## Quick Start

```bash
cd ~/my-project
vibrate
```

That's it. Vibrator auto-detects your Docker runtime (Docker Desktop, OrbStack, Colima, Podman, etc.), builds the image on first run, and drops you into an interactive shell with Claude Code ready to go.

---

## Authentication

### Recommended: OAuth token (long-lived)

1. On the **host**, generate a token:
   ```bash
   claude setup-token
   ```
2. Save it:
   ```bash
   echo "eyJhbGc..." > ~/.claude-docker-token
   ```
   Vibrator picks this up automatically on every run.

> **Note:** Using a pre-set OAuth token (`CLAUDE_CODE_OAUTH_TOKEN` or `~/.claude-docker-token`) does **not** grant access to the 1M context window, even if your Claude plan includes it. The extended context is tied to browser-based OAuth login.
>
> To use 1M context inside the container, skip the token file and log in interactively:
> ```bash
> unset CLAUDE_CODE_OAUTH_TOKEN
> vibrate
> # then inside the container:
> claude auth login
> ```

### Alternative: Anthropic API key

```bash
ANTHROPIC_API_KEY=sk-ant-... vibrate
```

---

## Common Options

```bash
vibrate                          # Interactive shell (current directory)
vibrate --workspace /path        # Custom workspace
vibrate --build                  # Build image and exit
vibrate --rebuild                # Force rebuild from scratch
vibrate --dind                   # Docker-in-Docker (elevated privileges)
vibrate --ssh-gpg-agents         # Forward SSH/GPG agents (opt-in)
vibrate --verbose                # Show Docker commands and runtime info
vibrate --export-dockerfile FILE # Dump generated Dockerfile
```

### Environment variables

```bash
VIBRATOR_IMAGE          # Override Docker image
VIBRATOR_VERBOSE=1      # Verbose output
VIBRATOR_DOCKER_SOCKET  # Override Docker socket
CLAUDE_CODE_OAUTH_TOKEN # OAuth token (preferred auth)
ANTHROPIC_API_KEY       # API key (legacy auth)
```

---

## Without Claude Code on the Host

```bash
# Pull pre-built image (~2GB, skips 10+ min build)
vibrate --pull

# Or build locally
vibrate --generic --build
```

Authenticate inside the container with `claude auth login`.

---

## Building from Source

```bash
make build             # Build vibrate.sh
make build VERSION=1.2 # With specific version
make validate          # Build + run all tests (52 tests)
make lint              # shellcheck source files
make clean             # Remove build artifacts
```

---

## License

MIT — see [LICENSE](./LICENSE)
