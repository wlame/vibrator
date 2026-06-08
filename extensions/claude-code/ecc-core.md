---
name: ECC — Everything Claude Code (core)
description: ECC core profile — lean baseline (rules + agents + commands + hooks) into ~/.claude
kind: plugin
default: false
size_mb: 4
category: harness-specific
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  # Everything Claude Code (ECC) — affaan-m/ECC. Installed into ~/.claude via
  # ECC's own manifest-driven installer. The "core" profile is the lean harness
  # baseline: rules + agents + commands + hook runtime + platform configs +
  # quality workflow. See ecc-developer for the full overview and ecc-* family.
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
  node scripts/install-apply.js --target claude --profile core

  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (core profile)

The lean baseline [ECC](https://github.com/affaan-m/ECC) install. See
`ecc-developer` for the full overview of ECC and the complete `ecc-*` family.

## What this profile is

ECC's `core` profile: `rules-core`, `agents-core`, `commands-core`,
`hooks-runtime`, `platform-configs`, `workflow-quality`. Same as `ecc-minimal`
but **with the hook runtime**. A solid, low-context starting point for cautious
adoption.

Installs into `~/.claude/`: 63 agents · 21 skills · 115 rule files · 79 commands
· 4 hooks. Approx. **4.2 MB** installed.

## When to pick a different profile

- Lighter, no hooks? → `ecc-minimal` (~3.1 MB).
- Default engineering preset (more skills + language/db/orchestration)? →
  `ecc-developer` (~5.5 MB).
- Security- or research-focused? → `ecc-security` / `ecc-research`.
- Everything? → `ecc-full` (~249 skills, heaviest context).

## Source

<https://github.com/affaan-m/ECC>
