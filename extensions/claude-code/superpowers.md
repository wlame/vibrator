---
name: Superpowers (obra)
kind: plugin
default: false
size_mb: 8
category: harness-specific
install: |
  # obra/superpowers-marketplace is a manifest ONLY — its
  # marketplace.json declares the "superpowers" plugin with a
  # `source: { source: "url", url: "https://github.com/obra/superpowers.git" }`.
  # The actual plugin code lives in a separate repo (obra/superpowers)
  # and must be cloned independently for the cache. This differs from
  # claude-mem / codex-plugin-cc, where the plugin sources live inside
  # the marketplace repo itself.
  mkdir -p "$HOME/.claude/plugins/marketplaces"

  # 1. Clone the marketplace manifest. Directory basename matches
  #    the marketplace's "name" field ("superpowers-marketplace") so
  #    Claude Code's `/plugin marketplace list` recognises it.
  git clone --depth 1 https://github.com/obra/superpowers-marketplace.git \
    "$HOME/.claude/plugins/marketplaces/superpowers-marketplace"

  # 2. Clone the actual plugin repo into a scratch dir, then copy its
  #    contents into the cache under the marketplace + plugin name.
  #    The plugin's files (.claude-plugin/, skills/, hooks/, ...)
  #    live at the REPO ROOT, not under any subdirectory.
  SP_TMP=$(mktemp -d)
  git clone --depth 1 https://github.com/obra/superpowers.git "$SP_TMP/sp"
  SP_GIT_SHORT=$(cd "$SP_TMP/sp" && git rev-parse --short=12 HEAD)
  SP_DEST="$HOME/.claude/plugins/cache/superpowers-marketplace/superpowers/$SP_GIT_SHORT"
  mkdir -p "$SP_DEST"
  cp -r "$SP_TMP/sp/." "$SP_DEST/"
  rm -rf "$SP_TMP"
source: https://github.com/obra/superpowers
---

# Superpowers (obra)

Jesse Vincent's "agentic skills framework + software-development
methodology". A curated bundle of composable skills that imposes
discipline on otherwise free-form coding sessions:

| Skill | What it forces |
|---|---|
| **brainstorming** | Socratic intent + requirements pass before any code |
| **writing-plans** | Multi-step task → written plan first |
| **test-driven-development** | Tests before implementation, no exceptions |
| **systematic-debugging** | Reproducing → isolating → understanding → fixing |
| **verification-before-completion** | Run the check, paste the output, then claim done |
| **subagent-driven-development** | Independent tasks dispatched to fresh subagents |
| **requesting-code-review** / **receiving-code-review** | Review loops with rigor |
| **writing-skills** | Meta-skill for authoring new skills |
| **using-git-worktrees** | Isolated workspaces for parallel feature work |
| **dispatching-parallel-agents** | 2+ independent tasks → parallel agents |

## Why opt-in

Opinionated by design. The skills bind hard rules ("you MUST use this
before any creative work", "evidence before assertions always"). Great
if you want that scaffolding, frustrating if you don't.

It's also the only entry in the `harness-specific` category that
substantially overlaps with `cc-thingz` and `superclaude` —
pick one workflow framework, don't enable all three.

## Installation channels

Two routes, same skills:

- **Obra marketplace** (this extension): `obra/superpowers-marketplace`
- **Anthropic official**: `/plugin install superpowers@claude-plugins-official`

We install from the obra marketplace because it tracks upstream releases
faster.

## Bundled commands

The skills register slash commands like `/brainstorm`, `/plan`,
`/tdd`, `/debug`, plus the Skill tool itself for in-conversation
activation. After install, run `/help` inside Claude Code to see the
full surface.
