---
name: plugin-creator
description: Helper for scaffolding new Codex plugins
kind: skill
default: true
size_mb: 0
install: |
  # Built-in to the Codex CLI itself — no install step required. The skill
  # is registered automatically when the codex binary is on $PATH.
  true
source: https://developers.openai.com/codex/plugins
---

# plugin-creator (built-in)

Codex ships with the `@plugin-creator` skill out of the box. Used to scaffold
your own plugin: builds the directory structure, generates a marketplace
entry, and tests it locally before sharing.

Invoke inside a Codex session as `@plugin-creator` and follow the prompts.

## When to enable

It's free and built-in — keep on by default. The wizard pre-checks it for
every new Codex container.

## Build your own plugin

A plugin bundles skills + apps + MCP servers into a reusable package. The
official marketplace ([plugins directory]) launched March 2026 with 20+
plugins; self-serve publishing was not available at launch but is on the
roadmap.

[plugins directory]: https://developers.openai.com/codex/plugins
