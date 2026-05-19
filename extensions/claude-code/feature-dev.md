---
name: feature-dev
kind: plugin
default: true
size_mb: 2
install: |
  # Anthropic's official plugins marketplace — registered as the short
  # name `claude-plugins-official` regardless of the GitHub repo path
  # (`anthropics/claude-plugins-official`). `|| true` makes the
  # registration idempotent across cached layers.
  claude plugin marketplace add anthropics/claude-plugins-official 2>/dev/null || true
  claude plugin install feature-dev@claude-plugins-official
source: https://github.com/anthropics/claude-plugins-official
---

# feature-dev (Anthropic official)

Turns a feature brief into working code by imposing the structured
explore→plan→implement→review process that senior engineers use instinctively.
89k+ installs as of mid-2026 — the most popular plugin in the official
marketplace by a wide margin.

Workflow:

1. `/feature-dev <brief>` — explore-codebase phase
2. The skill produces a planning artifact
3. Reviews + iterates before any code is written

## Why on by default

Vibrator's primary user is a developer running Claude Code on real
codebases. This is the most-used plugin in that audience by far.

## Bundles

`feature-dev` registers four subagents internally: `code-architect`,
`code-explorer`, `code-reviewer`, and a glue agent. They surface as
`Task(subagent_type=...)` calls in your sessions.
