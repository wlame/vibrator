---
name: Codex Plugin for Claude Code (official OpenAI)
description: Delegate code reviews and background tasks to Codex from inside Claude Code
kind: plugin
default: true
size_mb: 8
category: ai-integration
deps:
  features: [node, codex-cli]
auth:
  env: OPENAI_API_KEY
runtime_needs:
  third_party_api: OpenAI
  outbound_net: true
install: |
  # Official OpenAI plugin — gives Claude Code slash commands to invoke
  # the Codex CLI for second-opinion code reviews and background task
  # delegation. Marketplace install mirrors what
  #   /plugin marketplace add openai/codex-plugin-cc
  #   /plugin install codex@openai-codex
  # would do inside an interactive Claude Code session, but at image-
  # bake time so the slash commands are live on first launch.
  mkdir -p "$HOME/.claude/plugins/marketplaces"

  # The marketplace directory's basename MUST match the marketplace's
  # "name" field in .claude-plugin/marketplace.json — "openai-codex"
  # for this repo — so /reload-plugins resolves the cached install.
  git clone --depth 1 https://github.com/openai/codex-plugin-cc.git \
    "$HOME/.claude/plugins/marketplaces/openai-codex"

  # rev-parse --short=12 returns the 12-char SHA directly — POSIX-safe
  # (dash's /bin/sh doesn't support bash's ${VAR:0:N} substring).
  CPC_GIT_SHORT=$(cd "$HOME/.claude/plugins/marketplaces/openai-codex" && git rev-parse --short=12 HEAD)
  CPC_DEST="$HOME/.claude/plugins/cache/openai-codex/codex/$CPC_GIT_SHORT"
  mkdir -p "$CPC_DEST"

  # marketplace.json declares the plugin source as ./plugins/codex/ —
  # that's what we cache for Claude Code to load.
  cp -r "$HOME/.claude/plugins/marketplaces/openai-codex/plugins/codex/." "$CPC_DEST/"
source: https://github.com/openai/codex-plugin-cc
---

# Codex Plugin for Claude Code (official OpenAI)

OpenAI's first-party Claude Code plugin: lets you invoke the Codex CLI
from inside a Claude Code session for code reviews and background task
delegation. Adds slash commands:

- `/codex:review` — adversarial / second-opinion code review
- `/codex:delegate` — fire-and-forget Codex job in the background
- `/codex:status` / `/codex:result` — poll and collect the delegated job
- `/codex:setup` — first-run wizard (installs Codex CLI if absent,
  walks auth)

## Auth

The plugin shells out to the `codex` CLI inside the container, which
needs its own credentials. Two paths:

1. **`OPENAI_API_KEY` (recommended for containers)** — set it on the
   host; vibrator forwards it. Usage counts against your OpenAI API
   quota.
2. **OAuth via Codex/ChatGPT subscription** — run `codex login` inside
   the container once. The token persists to
   `~/.codex/auth.json` (under the container user's home) and survives
   `docker start`, but is lost on container removal or
   `vibrate --rebuild`. Re-run `codex login` after a rebuild.

If you primarily run Codex on the host with OAuth and want that auth
state to survive across vibrator rebuilds, the cleanest workaround is
to mint an API key alongside the OAuth session and forward
`OPENAI_API_KEY`.

## Dependencies

Pulls in the `codex-cli` feature (and `node`, which codex-cli depends
on), so the standalone `codex` binary is on PATH inside the container.
If your profile is `backend` or `frontend`, vibrator will warn that
`codex-cli` was auto-pulled — that's expected.

## Caveats from the upstream README

- "The review gate can create a long-running Claude/Codex loop and may
  drain usage limits quickly — only enable when actively monitoring."
  Treat `/codex:review` as a deliberate action, not a background hook.
- "Code reviews might take a while" — delegate / review commands are
  designed to run in the background; the agent stays interactive while
  Codex chews.
- Plugin uses your local Codex CLI authentication and configuration —
  if Codex isn't already set up on first run, `/codex:setup` walks the
  install and login.

## Why default-on

Cross-harness collaboration is a strong workflow: one model drives the
edit loop, the other delivers the second opinion. OpenAI shipping a
first-party plugin under their own GitHub org signals the pattern is
load-bearing. Disable with `--no=codex-plugin-cc` if you only use one
model provider in this workspace.
