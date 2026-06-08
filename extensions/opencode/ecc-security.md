---
name: ECC — Everything Claude Code (security)
description: ECC security profile for OpenCode — core + security module into ~/.opencode
kind: plugin
default: false
size_mb: 3
category: security
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  # Everything Claude Code (ECC) — affaan-m/ECC. Unified installer with
  # --target opencode → writes ~/.opencode. The "security" profile is the core
  # baseline plus ECC's security module. See claude-code/ecc-developer for the
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
  node scripts/install-apply.js --target opencode --profile security

  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (security profile, OpenCode)

The security-focused [ECC](https://github.com/affaan-m/ECC) install for OpenCode.
See `claude-code/ecc-developer` for the full overview of ECC.

## What this profile is

ECC's `security` profile for OpenCode: `commands-core`, `hooks-runtime`,
`platform-configs`, `workflow-quality`, `security` (security-reviewer agents +
security guidance).

Installs into `~/.opencode/`. Approx. **3.1 MB** installed.

## OpenCode build step

OpenCode's installer requires `.opencode/dist` compiled first
(`npm run build:opencode`); the snippet handles this automatically.

## When to pick a different profile

- Leaner baseline? → `ecc-core`.
- Default engineering preset? → `ecc-developer`.
- Research focus? → `ecc-research`.
- Everything? → `ecc-full`.

## Source

<https://github.com/affaan-m/ECC>
