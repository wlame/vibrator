---
name: ECC — Everything Claude Code (developer)
description: ECC developer profile for OpenCode — commands + hooks + db + orchestration into ~/.opencode
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
  # --target opencode → writes ~/.opencode. The "developer" profile is ECC's
  # default engineering preset (adds database + orchestration on top of core).
  # See claude-code/ecc-developer for the full ECC overview.
  #
  # OpenCode needs the compiled plugin payload (.opencode/dist) built first via
  # `npm run build:opencode`, so this does a FULL npm install then build before
  # installing. Clone + node_modules (~90 MB) dropped afterward; ~/.opencode
  # (~3.0 MB) persists.
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
  node scripts/install-apply.js --target opencode --profile developer

  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (developer profile, OpenCode)

The default engineering preset of [ECC](https://github.com/affaan-m/ECC) for
OpenCode. See `claude-code/ecc-developer` for the full overview of ECC.

## What this profile is

ECC's `developer` profile for OpenCode: `commands-core`, `hooks-runtime`,
`platform-configs`, `database`, `workflow-quality`, `orchestration`.

Installs into `~/.opencode/`: `opencode.json`, `commands/`, `hooks/`, `skills/`,
`plugins/`, `tools/`, `dist/`, `prompts/`, `instructions/`. Approx. **3.0 MB**
installed.

## OpenCode build step

OpenCode's installer requires `.opencode/dist` compiled first
(`npm run build:opencode`); the snippet handles this automatically.

## When to pick a different profile

- Leaner baseline? → `ecc-core` (or `ecc-minimal` for no hooks).
- Security / research focus? → `ecc-security` / `ecc-research`.
- Everything? → `ecc-full`.

## Source

<https://github.com/affaan-m/ECC>
