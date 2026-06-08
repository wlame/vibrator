---
name: ECC — Everything Claude Code (research)
description: ECC research profile for OpenCode — core + research/content modules into ~/.opencode
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
  # --target opencode → writes ~/.opencode. The "research" profile is the core
  # baseline plus research/content modules. See claude-code/ecc-developer for the
  # full ECC overview.
  #
  # OpenCode needs the compiled plugin payload (.opencode/dist) built first via
  # `npm run build:opencode`, so this does a FULL npm install then build before
  # installing. Clone + node_modules (~90 MB) dropped afterward; ~/.opencode
  # (~3.1 MB) persists.
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
  node scripts/install-apply.js --target opencode --profile research

  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (research profile, OpenCode)

The research/content-oriented [ECC](https://github.com/affaan-m/ECC) install for
OpenCode. See `claude-code/ecc-developer` for the full overview of ECC.

## What this profile is

ECC's `research` profile for OpenCode: `commands-core`, `hooks-runtime`,
`platform-configs`, `workflow-quality`, plus `research-apis`, `business-content`,
and `social-distribution`.

Installs into `~/.opencode/`. Approx. **3.1 MB** installed.

> Many research skills make outbound calls at runtime (hence `outbound_net`).

## OpenCode build step

OpenCode's installer requires `.opencode/dist` compiled first
(`npm run build:opencode`); the snippet handles this automatically.

## When to pick a different profile

- Leaner baseline? → `ecc-core`.
- Default engineering preset? → `ecc-developer`.
- Security focus? → `ecc-security`.
- Everything? → `ecc-full`.

## Source

<https://github.com/affaan-m/ECC>
