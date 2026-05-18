---
name: GitHub
kind: plugin
default: true
size_mb: 1
auth:
  env: GITHUB_PERSONAL_ACCESS_TOKEN
install: |
  # Codex plugins are installed by writing to ~/.codex/config.toml at image-
  # bake time. The Phase 3 harness Install will template the [plugins.github]
  # subtable here based on this catalog entry.
  codex plugins install github
source: https://developers.openai.com/codex/plugins
---

# GitHub (Codex official)

Review changes, manage issues, and interact with GitHub repositories from
Codex. Official OpenAI plugin launched March 2026.

## Auth

Set `GITHUB_PERSONAL_ACCESS_TOKEN` on the host before running `vibrate`.
The token is forwarded into the container; Codex picks it up from the env.

Scope minimally: `repo` for normal workflows, plus `workflow` if you'll
read Actions logs.

## Why on by default

Codex users overwhelmingly want GitHub integration — issues, PRs, repos
are the primary surface for code review and management. Disable with
`--no=github` if you intentionally don't want it.
