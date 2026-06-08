---
name: ECC — Everything Claude Code (core)
description: ECC core profile for Codex — agents + platform + quality into ~/.codex
kind: plugin
default: false
size_mb: 2
category: harness-specific
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  # Everything Claude Code (ECC) — affaan-m/ECC. The SAME unified installer used
  # for claude-code, driven with --target codex so it writes into ~/.codex
  # (AGENTS.md, config.toml, agents/, skills/). ECC auto-reduces the module set
  # for Codex: claude-only rules/commands/hooks are skipped, leaving agents +
  # platform configs + quality workflow. See claude-code/ecc-developer for the
  # full ECC overview.
  #
  # Pinned to a reviewed commit; bump deliberately across all ecc-* entries.
  ECC_REF=99baa8250096f2d295583572399a5c9aba2ce312

  # Shallow-fetch EXACTLY the pinned commit (survives upstream advancing).
  mkdir -p /tmp/ecc
  cd /tmp/ecc
  git init -q
  git remote add origin https://github.com/affaan-m/ECC.git
  git fetch -q --depth 1 origin "$ECC_REF"
  git checkout -q FETCH_HEAD

  npm install --no-audit --no-fund --omit=dev --loglevel=error
  node scripts/install-apply.js --target codex --profile core

  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (core profile, Codex)

[ECC](https://github.com/affaan-m/ECC) for the Codex harness. The same unified
installer as claude-code, with `--target codex`. See `claude-code/ecc-developer`
for the full overview of what ECC is.

## What this profile is

ECC's `core` profile, reduced to what applies to Codex: `agents-core`,
`platform-configs`, `workflow-quality`. Codex doesn't consume ECC's
claude-specific `rules-core` / `commands-core` / `hooks-runtime` modules, so the
installer skips them automatically.

Installs into `~/.codex/`: `AGENTS.md`, `config.toml` (MCP servers merged
add-only), `agents/`, and `skills/`. Approx. **1.9 MB** installed.

> On Codex, ECC's `minimal` and `core` profiles are identical (the only
> difference upstream is the hook runtime, which Codex skips). This entry is the
> lean Codex baseline — there is no separate `ecc-minimal` for codex.

## When to pick a different profile

- Default engineering preset (adds database)? → `ecc-developer`.
- Security or research focus? → `ecc-security` / `ecc-research`.
- Everything ECC ships for Codex? → `ecc-full`.

## Source

<https://github.com/affaan-m/ECC>
