---
name: SuperClaude
kind: plugin
default: false
size_mb: 30
deps:
  features: [python]
install: |
  uvx superclaude install
source: https://github.com/SuperClaude-Org/SuperClaude_Framework
---

# SuperClaude

Framework that adds ~25 `/sc:*` slash commands (`/sc:load`, `/sc:save`,
`/sc:analyze`, `/sc:implement`, `/sc:design`, `/sc:business-panel`,
`/sc:spec-panel`, etc.) and corresponding behavior layers on top of Claude
Code.

Installs via `uvx` (Python isolated venv) — provides a single CLI installer
that writes commands into `~/.claude/commands/sc/`. Activation is then
implicit on slash command invocation.

## When to enable

- Heavy users who want a richer "command palette" of orchestration
  shortcuts.
- Teams that have standardized on the `/sc:*` workflow patterns.

Default = off because it overlaps significantly with the cc-thingz bundle
and the Anthropic-official feature-dev/code-review plugins. Pick one
workflow framework and stick with it.

## Conflict matrix

- `/sc:reflect` overlaps with `/review:git-review`
- `/sc:implement` overlaps with `/feature-dev`
- `/sc:business-panel` is unique (no overlap)

If you enable SuperClaude alongside cc-thingz, expect command-name
collisions at slash-completion time — Claude Code will list multiple
matches and you'll have to disambiguate manually.
