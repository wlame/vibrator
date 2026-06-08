---
name: ECC — Everything Claude Code (minimal)
description: ECC minimal profile for OpenCode — commands + platform + quality, no hooks (lightest)
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
  # --target opencode → writes ~/.opencode. The "minimal" profile is the lightest
  # (commands + platform + quality, no hook runtime). See claude-code/ecc-developer
  # for the full ECC overview.
  #
  # OpenCode is the one harness that needs a build step first: its installer
  # requires the compiled plugin payload under .opencode/dist, which is produced
  # by `npm run build:opencode` (TypeScript). So this snippet does a FULL npm
  # install (not --omit=dev) and builds before installing. The clone +
  # node_modules (~90 MB transient) are dropped afterward; only ~/.opencode
  # (~1.7 MB) persists.
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
  node scripts/install-apply.js --target opencode --profile minimal

  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (minimal profile, OpenCode)

The lightest [ECC](https://github.com/affaan-m/ECC) install for the OpenCode
harness. See `claude-code/ecc-developer` for the full overview of ECC.

## What this profile is

ECC's `minimal` profile for OpenCode: `commands-core`, `platform-configs`,
`workflow-quality` — **no hook runtime**. (OpenCode consumes ECC's commands/hooks
modules rather than the agents/rules modules used by claude-code.)

Installs into `~/.opencode/`: `opencode.json`, `commands/`, `skills/`,
`plugins/`, `tools/`, `dist/` (compiled plugin payload), `prompts/`,
`instructions/`. Approx. **1.7 MB** installed.

## OpenCode build step

Unlike the other harnesses, OpenCode's installer needs `.opencode/dist` compiled
first (`npm run build:opencode`). The install snippet handles this automatically
with a full dependency install; the transient build deps are not kept in the
image.

## When to pick a different profile

- Want hooks too? → `ecc-core` (~2.8 MB).
- Default engineering preset? → `ecc-developer`.
- Security / research focus? → `ecc-security` / `ecc-research`.
- Everything? → `ecc-full` (heaviest).

## Source

<https://github.com/affaan-m/ECC>
