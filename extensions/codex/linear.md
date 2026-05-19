---
name: Linear
kind: plugin
default: false
size_mb: 1
auth:
  env: LINEAR_API_KEY
install: |
  codex plugins install linear
source: https://developers.openai.com/codex/plugins
---

# Linear (Codex official)

Create and manage Linear issues, projects, and workflows from Codex.

## Auth

Linear uses personal API keys. Get one at
[linear.app/settings/api](https://linear.app/settings/api) and set
`LINEAR_API_KEY` on the host.

## Workflow examples

- "Create a Linear issue from this bug report"
- "Move ENG-1234 to in-review and link the PR"
- "List my open Linear issues in the API project"

Default = off because not everyone uses Linear. Enable for teams that
standardize on it.
