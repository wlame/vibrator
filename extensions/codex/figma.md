---
name: Figma
kind: plugin
default: false
size_mb: 1
auth:
  env: FIGMA_ACCESS_TOKEN
install: |
  codex plugins install figma
source: https://developers.openai.com/codex/plugins
---

# Figma (Codex official)

Inspect Figma designs, extract specs, and document components from Codex.
Useful when implementing a design — pull dimensions, colors, fonts, and
spacing directly into code without screenshot ferrying.

## Auth

Generate a personal access token at
[figma.com/developers/api#access-tokens](https://www.figma.com/developers/api#access-tokens).

Set `FIGMA_ACCESS_TOKEN` on the host.

## Workflow

Paste a Figma file URL into the conversation. The agent can then:

- List frames and components on each page
- Extract design tokens (colors, type styles)
- Generate TypeScript / Tailwind / styled-components code that matches

Default = off; opt-in for frontend teams using Figma as the design source.
