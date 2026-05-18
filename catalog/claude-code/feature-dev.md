---
name: feature-dev
kind: plugin
default: true
size_mb: 2
install: |
  # Installed via Anthropic's official plugins marketplace (claude-plugins-official).
  # See setup-marketplace.sh in the harness Install step for the marketplace clone.
  claude plugin install feature-dev@anthropics/claude-plugins-official
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
