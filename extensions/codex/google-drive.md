---
name: Google Drive
kind: plugin
default: false
size_mb: 1
install: |
  # OAuth-based, same as gmail. Credentials persist to ~/.codex/auth.json.
  codex plugins install google-drive
source: https://developers.openai.com/codex/plugins
---

# Google Drive (Codex official)

Edit and manage files in Google Drive from Codex. Useful when project docs,
design specs, or PRDs live in Drive rather than git.

## Auth

OAuth. First use opens a browser; subsequent runs reuse `~/.codex/auth.json`
(host-mounted).

## Workflow

- "Read the PRD at <drive URL> and summarize the success criteria"
- "Update the changelog doc with this section"

Same risk profile as Gmail — write operations can produce unexpected diffs.
Prefer read-only flows in interactive sessions.

Default = off.
