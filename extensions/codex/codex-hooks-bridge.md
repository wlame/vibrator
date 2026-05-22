---
name: codex-hooks bridge
kind: plugin
default: false
size_mb: 10
category: harness-specific
deps:
  features: [python]
install: |
  # codex-hooks is a Python install script. It clones the repo and adds
  # a `codex` shell function that wraps the real Codex CLI with a
  # background session monitor reading Claude-style hook settings.
  git clone https://github.com/hatayama/codex-hooks.git /opt/codex-hooks
  cd /opt/codex-hooks && python3 install.py
source: https://github.com/hatayama/codex-hooks
host_aliases: [codex-hooks]
---

# codex-hooks bridge

Translates Claude Code-style hook configuration into Codex's narrower
hook surface. The upstream Codex CLI fires hooks for a small set of
events (and `PreToolUse`/`PostToolUse` only for the Bash tool in stable
Codex); codex-hooks adds a wrapper that fires three normalised events
from session monitoring: `TaskStarted`, `TaskComplete`, `TurnAborted`.

It reads hook config in this precedence order:

1. `~/.codex/hooks.json` (Codex-native)
2. `<repo>/.claude/settings.json` (Claude Code project settings)
3. `~/.claude/settings.json` (Claude Code user settings)

The fallback to Claude Code's settings is the value-add — if you
already maintain hook config for Claude Code, the same JSON works in
Codex via this bridge.

## How it works

The installer drops a `codex` shell function on `$PATH` ahead of the
real Codex CLI. The function:

- Spawns the underlying `codex` process
- Runs a background watcher reading Codex's session transcript
- Fires normalised events via `/bin/sh -lc` whenever the transcript
  state changes
- Each hook command receives JSON on stdin with stable fields:
  `hook_event_name`, `transcript_path`, `cwd`, `session_id`, `raw_event`

## When to enable

- You already maintain Claude Code hooks and want them to fire for
  Codex sessions too.
- You want session-level events (`TaskComplete`) that Codex's native
  hook surface doesn't yet emit.
- You're building a cross-harness vibrator setup and want one source
  of truth for hook config.

## Disabling temporarily

```bash
CODEX_HOOKS_DISABLE=1 codex ...
```

This bypasses the wrapper and runs the bare Codex binary.

## Caveats

- Wrapper-based monitoring means hooks fire **after** the transcript is
  written, not strictly synchronously with tool calls. Don't rely on it
  for tool-call interception (that requires Codex-native `PreToolUse`).
- The bridge mirrors Claude Code semantics best-effort — Anthropic's
  hook surface evolves faster than this bridge; expect drift.
