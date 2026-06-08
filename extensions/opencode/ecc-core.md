---
name: ECC — Everything Claude Code (core)
description: ECC core profile for OpenCode — commands + hooks + platform + quality into ~/.opencode
kind: plugin
default: false
size_mb: 3
category: harness-specific
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  # Everything Claude Code (ECC) — affaan-m/ECC. Unified installer with
  # --target opencode → writes ~/.opencode. The "core" profile is the lean
  # baseline (commands + hook runtime + platform + quality). See
  # claude-code/ecc-developer for the full ECC overview.
  #
  # OpenCode needs the compiled plugin payload (.opencode/dist) built first via
  # `npm run build:opencode` (TypeScript), so this does a FULL npm install (not
  # --omit=dev) then build before installing. Clone + node_modules (~90 MB) are
  # dropped afterward; only ~/.opencode (~2.8 MB) persists.
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

  npm install --no-audit --no-fund --loglevel=error
  npm run build:opencode
  node scripts/install-apply.js --target opencode --profile core

  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (core profile, OpenCode)

The lean baseline [ECC](https://github.com/affaan-m/ECC) install for OpenCode.
See `claude-code/ecc-developer` for the full overview of ECC.

## What this profile is

ECC's `core` profile for OpenCode: `commands-core`, `hooks-runtime`,
`platform-configs`, `workflow-quality`. Same as `ecc-minimal` but with the hook
runtime.

Installs into `~/.opencode/`: `opencode.json`, `commands/`, `hooks/`, `skills/`,
`plugins/`, `tools/`, `dist/`, `prompts/`, `instructions/`. Approx. **2.8 MB**
installed.

## OpenCode build step

OpenCode's installer requires `.opencode/dist` compiled first
(`npm run build:opencode`); the snippet handles this with a full dependency
install whose build deps are not kept in the image.

## When to pick a different profile

- Lighter, no hooks? → `ecc-minimal`.
- Default engineering preset? → `ecc-developer`.
- Security / research focus? → `ecc-security` / `ecc-research`.
- Everything? → `ecc-full`.

## Source

<https://github.com/affaan-m/ECC>
