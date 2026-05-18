---
name: code-review
kind: plugin
default: true
size_mb: 1
install: |
  claude plugin install code-review@anthropics/claude-plugins-official
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
