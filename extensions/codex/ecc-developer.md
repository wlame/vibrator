---
name: ECC — Everything Claude Code (developer)
description: ECC developer profile for Codex — agents + platform + database + quality into ~/.codex
kind: plugin
default: false
size_mb: 2
category: harness-specific
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  # Everything Claude Code (ECC) — affaan-m/ECC. Unified installer with
  # --target codex → writes ~/.codex. ECC's "developer" profile, auto-reduced for
  # Codex (adds the database module on top of core). See claude-code/ecc-developer
  # for the full ECC overview.
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
  node scripts/install-apply.js --target codex --profile developer

  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (developer profile, Codex)

The default engineering preset of [ECC](https://github.com/affaan-m/ECC) for the
Codex harness. See `claude-code/ecc-developer` for the full overview of ECC.

## What this profile is

ECC's `developer` profile, auto-reduced for Codex: `agents-core`,
`platform-configs`, `database`, `workflow-quality`. (Codex skips ECC's
claude-only rules/commands/hooks modules.)

Installs into `~/.codex/`: `AGENTS.md`, `config.toml` (MCP merged add-only),
`agents/`, `skills/`. Approx. **2.0 MB** installed.

## When to pick a different profile

- Leaner baseline? → `ecc-core`.
- Security or research focus? → `ecc-security` / `ecc-research`.
- Everything ECC ships for Codex? → `ecc-full`.

## Source

<https://github.com/affaan-m/ECC>
