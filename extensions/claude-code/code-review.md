---
name: code-review
description: Slash-command plugin for opinionated, structured code reviews
kind: plugin
default: true
size_mb: 1
install: |
  # Register Anthropic's official marketplace if it isn't already.
  # `marketplace add` is idempotent against an existing entry but emits
  # a non-zero exit when the marketplace already exists — `|| true`
  # keeps the snippet safe to run across cached / fresh layers.
  claude plugin marketplace add anthropics/claude-plugins-official 2>/dev/null || true
  claude plugin install code-review@claude-plugins-official
source: https://github.com/anthropics/claude-plugins-official
---

# code-review (Anthropic official)

`/code-review` slash command for structured PR-style review of the current
working tree or a specified diff range.

Pulls in the `feature-dev:code-reviewer` subagent under the hood. Pairs
naturally with `feature-dev` — most users enable both.

## When to invoke

Run it before committing on a non-trivial change, or before pushing a PR.
The review surfaces:

- Logic errors and edge cases the implementer missed
- Security concerns (injection, missing input validation, secrets in logs)
- Adherence to project conventions (CLAUDE.md, language idioms)
- Test coverage gaps

Confidence-based filtering — only high-priority issues that truly matter,
not style nits.
