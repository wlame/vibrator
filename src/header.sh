#!/bin/bash
# vibrator - Docker runner for Claude Code
# https://github.com/wlame/vibrator
# Version: %%VERSION%%

set -euo pipefail

VIBRATOR_VERSION="%%VERSION%%"
VIBRATOR_REGISTRY="ghcr.io/wlame/vibrator"
# Claude CLI version this vibrator build targets. Baked from CLAUDE_CLI_VERSION
# at the repo root by the Makefile. Used as a Docker build-arg for cache-busting
# and labelling, and as the reference value for `vibrate --upgrade-claude`.
CLAUDE_CLI_VERSION="%%CLAUDE_CLI_VERSION%%"
