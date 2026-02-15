# Vibrator

Run Claude Code CLI in YOLO mode inside Docker containers with automatic Docker runtime detection, pre-configured MCP servers, and enhanced security.
As easy as `vibrate` - no configuration needed!

Go vibe yourself!

[![Build Status](https://img.shields.io/badge/build-passing-brightgreen)](./tests)
[![Tests](https://img.shields.io/badge/tests-52%2F52-brightgreen)](./tests)
[![License](https://img.shields.io/badge/license-MIT-blue)](./LICENSE)

**TLDR:** put the `vibrate.sh` script in your PATH, cd to your project and just run `vibrate` - it will:
- automatically detect your Docker runtime
- build the image with Claude Code CLI and plugins (pre-configured + host Claude config)
- automatically connect to MCP servers (Serena, Context7, agent-browser) if running on the host
- run an interactive shell in a container with your project mounted inside, forwarding SSH/GPG agents and needed environment variables
- Profit! 

---

## Features

### 🚀 Automatic Docker Runtime Detection

Vibrator automatically detects and supports multiple Docker runtime environments:

- Docker Desktop (macOS, Linux)
- OrbStack (macOS)
- Colima (macOS)
- Rancher Desktop (macOS, Linux)
- Podman (macOS, Linux)
- Native Docker (Linux)

No manual configuration needed - just run `vibrate` and it works!

### Secure by Default (as much as possible)

- **Graduated privilege system**: Minimal privileges by default
- **Docker-in-Docker opt-in**: Explicit `--docker` flag for elevated privileges
- **Clear security warnings**: Know when running with elevated permissions
- **Container rules**: Automatic safety restrictions in containerized environment

### Pre-configured MCP Servers

- **Serena**: Semantic code analysis (LSP-based). Automatically connects to host Serena server if running
- **Context7**: Library documentation lookup
- **Playwright**: Browser automation with Chromium (stdio mode, in-container)
- **agent-browser**: Headless web debugging and interaction (SSE mode)

### Pre-installed Tools

- **[ralphex](https://ralphex.com/)**: Autonomous coding loop for Claude Code - executes implementation plans task-by-task in fresh sessions with multi-agent code review

### 🎯 Developer Experience

- **Smart agent forwarding**: SSH and GPG agents auto-detected and forwarded. Can be disabled with `--no-agents` for maximum security
- **OAuth token support**: Long-lived authentication
- **Anthropic API key support**: Legacy short-lived authentication
- **Workspace isolation**: Each project in its own container
- **Fast builds**: Efficient caching and multi-stage Dockerfiles (inspired by icanhasjonas/run-claude-docker)

---

## Quick Start

### Installation

```bash
# Clone the repository
git clone https://github.com/wlame/vibrator.git
cd vibrator

# Build the script
make build

# Install (optional)
sudo cp build/vibrate.sh /usr/local/bin/vibrate
```

### Basic Usage

```bash
# Interactive shell with auto-detected Docker runtime
vibrate

# Run Claude with a prompt
vibrate claude "help me refactor this code"

# Custom workspace
vibrate --workspace /path/to/project

# Build image only
vibrate --build
```

### Docker Runtime Detection

Vibrator automatically detects your Docker runtime - no configuration needed! Just run `vibrate` and it will find the right socket.
Override if needed:

```bash
# Use custom socket
vibrate --docker-socket=/custom/docker.sock

# Specify Colima profile if you have multiple
vibrate --colima-profile=staging
```

### Authentication

Vibrator requires a Claude Code OAuth token to authenticate with Anthropic's API.

#### Obtaining a Long-Lived OAuth Token

1. **Run the Claude Code CLI setup command:**

   ```bash
   claude setup-token
   ```

   This will:
   - Open your browser for authentication
   - Generate a long-lived OAuth token
   - Display the token in your terminal

2. **Save the token to file (recommended):**

   ```bash
   echo "eyJhbGcgOvIBey0RSElfHgsWR5cCI6IkpXVCJ9..." > ~/.claude-docker-token
   ```

   Replace `eyJhbGc...` with your actual token from step 1.

   Vibrator will automatically load the token from this file on every run.

3. **Verify it works:**
   ```bash
   vibrate
   # You should see Claude Code starting without /login request
   ```

#### Legacy: Anthropic API Key

You can also use a legacy API key:
```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

**Note:** OAuth tokens are preferred as they're longer-lived

---

## Command Line Options

### Core Options

```bash
-E, --forward-env VAR        Forward environment variable
--config PATH                Claude config directory (default: ~/.claude)
--workspace PATH             Workspace directory to mount (default: current)
--image NAME                 Docker image name
--name NAME                  Container name (auto-generated from workspace)
--verbose                    Show detailed output and Docker commands
--help                       Display help message
```

### Security Options

```bash
--dind, --docker             Enable Docker-in-Docker mode (elevated privileges inside container)
--docker-socket PATH         Override Docker socket path (auto-detected)
--colima-profile NAME        Colima profile name (default: default)
--privileged                 Enable Docker privileged mode
--no-agents                  Disable SSH and GPG agent forwarding
```

### Container Options

```bash
--rm                         Auto-remove container on exit
--non-interactive            Disable interactive mode (no TTY)
--recreate                   Delete and recreate container
--memory LIMIT               Memory limit (e.g., 2g, 512m)
--cpu COUNT                  CPU limit (e.g., 2, 0.5, 1.5)
```

### Build Options

```bash
--build                      Build Docker image and exit
--rebuild                    Rebuild image from scratch (ignore cache)
--export-dockerfile FILE     Write generated Dockerfile to file
--no-plugins                 Skip Claude plugin installation
```

### Additional Options

```bash
--username NAME              Container username (default: current user)
--aws                        Enable AWS integration
--mount-full-config          Mount entire ~/.claude directory
--version                    Show version information
```

---

### Security Warning

When `--dind` is enabled, you'll see a warning banner:

```
+--------------------------------------------------------------+
|  ⚠️  DOCKER-IN-DOCKER MODE ENABLED                          |
+--------------------------------------------------------------+
   This container runs with ELEVATED PRIVILEGES:
   • Docker socket mounted (/var/run/docker.sock)
   • CAP_SYS_ADMIN capability granted
   • Seccomp filtering disabled

   Security Impact:
   • Container escape is possible if compromised
   • Full access to host Docker daemon
   • Use only when Docker commands are needed
+--------------------------------------------------------------+
```

## Environment Variables

### Core Variables

```bash
VIBRATOR_IMAGE               Override default Docker Hub image
VIBRATOR_VERBOSE             Set to 1 for verbose output
VIBRATOR_EXTRA_ENV           Space-separated list of extra env vars to forward
```

### Runtime Detection

```bash
VIBRATOR_DOCKER_SOCKET       Override Docker socket path
COLIMA_PROFILE               Colima profile name
DOCKER_HOST                  Docker daemon URL (standard Docker env var)
```

### Authentication

```bash
CLAUDE_CODE_OAUTH_TOKEN      Claude OAuth token (long-lived, preferred)
ANTHROPIC_API_KEY            Anthropic API key (legacy, short-lived)
```

---

## Examples

### Basic Development

```bash
# Interactive shell
vibrate

# Run Claude command
vibrate claude "analyze this codebase"

# Custom workspace
vibrate --workspace ~/projects/myapp

# Verbose mode (show runtime detection)
vibrate --verbose
```

### Build Operations

```bash
# Build image only
vibrate --build

# Rebuild from scratch
vibrate --rebuild

# Export Dockerfile
vibrate --export-dockerfile Dockerfile.generated
```

### Runtime Override

```bash
# Use specific Docker socket
vibrate --docker-socket=/var/run/docker.sock

# Use Colima staging profile
vibrate --colima-profile=staging

# Combine with other options
vibrate --docker-socket=/custom/socket --workspace /project
```

---

## Building from Source

### Prerequisites

- Bash 4+
- Docker (any supported runtime)
- GNU make
- Standard Unix tools (sed, awk, base64, etc.)

### Build Process

```bash
# Clone repository
git clone https://github.com/wlame/vibrator.git
cd vibrator

# Build development version
make build

# Build specific version
make build VERSION=1.0.0

# Run tests
make validate

# Clean build artifacts
make clean
```

### Build Targets

```bash
make build              # Build vibrate.sh
make clean              # Remove build artifacts
make validate           # Build and run all tests
make lint               # Run shellcheck (if available)
make help               # Show available targets
```

---

## Testing

Vibrator includes comprehensive test coverage:

### Test Suites

```bash
# Build validation (18 tests)
bash tests/validate-build.sh

# Docker runtime unit tests (15 tests)
bash tests/test-docker-runtime.sh

# Integration tests (19 tests)
bash tests/test-runtime-integration.sh

# Run all tests
make validate
```

### Test Coverage

- **Build tests**: Syntax, placeholders, functions, Dockerfile generation
- **Unit tests**: Runtime detection logic, socket identification, helper functions
- **Integration tests**: CLI flags, help output, runtime detection flow

**Total: 52 tests, all passing ✅**

---

## Architecture

### Modular Design

Vibrator is built with a modular architecture:

```
src/
├── header.sh              # Version, strict mode
├── lib/
│   ├── args.sh           # CLI argument parsing
│   ├── checks.sh         # Pre-flight validation
│   ├── config.sh         # Configuration management
│   ├── container.sh      # Container lifecycle
│   ├── docker_cmd.sh     # Docker command builder
│   ├── docker_runtime.sh # Runtime detection (NEW)
│   ├── dockerfile.sh     # Dockerfile generation
│   ├── image.sh          # Image operations
│   ├── logging.sh        # Colorized logging
│   └── plugins.sh        # Plugin detection
└── main.sh               # Main orchestration

templates/
├── Dockerfile.template   # Multi-stage build
├── entrypoint.sh        # Container initialization
├── claude-exec.sh       # Exec wrapper
├── setup-plugins.sh     # Plugin installation
├── zshrc                # Shell configuration
└── container-rules/     # Claude safety rules
    ├── docker-container-context.md
    └── safety-restrictions.md
```

### Build System

The Makefile-based build system:
1. Concatenates source modules in dependency order
2. Base64-encodes template files
3. Replaces placeholders (VERSION, etc.)
4. Generates single distributable script

---

### Container Rules

Vibrator merges two sets of Claude rules:
- **Host rules**: Your personal rules at `~/.claude/rules/` (read-only)
- **Container rules**: Safety restrictions for containerized environment

### Agent Forwarding

SSH and GPG agents are automatically forwarded (unless `--no-agents`):
- Enables git operations with SSH keys
- GPG signing support
- Secure, socket-based forwarding

---

## Project Status

**Status**: Active Development
**License**: MIT
**Maintainer**: wlame

## License

MIT License - see LICENSE file for details

---

**Happy vibing with vibrator!**
