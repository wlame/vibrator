---
name: ECC — Everything Claude Code (minimal)
description: ECC minimal profile — rules + agents + commands into ~/.claude, no hook runtime (lightest)
kind: plugin
default: false
size_mb: 3
category: harness-specific
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  # Everything Claude Code (ECC) — affaan-m/ECC. Installed into ~/.claude via
  # ECC's own manifest-driven installer. The "minimal" profile is the lightest:
  # rules + agents + commands + platform configs + quality workflow, with NO
  # hook runtime. See ecc-developer for the full overview and the ecc-* family.
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
  node scripts/install-apply.js --target claude --profile minimal

  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (minimal profile)

The lightest [ECC](https://github.com/affaan-m/ECC) install. See `ecc-developer`
for the full overview of what ECC is and the complete `ecc-*` profile family.

## What this profile is

ECC's `minimal` profile: `rules-core`, `agents-core`, `commands-core`,
`platform-configs`, and `workflow-quality` — **but no hook runtime**. Choose it
when you want ECC's agents/rules/commands without any event-driven hooks and with
the smallest context + image footprint.

Installs into `~/.claude/` (ECC content under `rules/ecc/`, `skills/ecc/`):
63 agents · 21 skills · 115 rule files · 79 commands · **0 hooks**.
Approx. **3.1 MB** installed.

## When to pick a different profile

- Want hooks too? → `ecc-core` (adds the hook runtime, ~4.2 MB).
- Want the default engineering preset? → `ecc-developer`.
- Security- or research-focused? → `ecc-security` / `ecc-research`.
- Everything? → `ecc-full` (~249 skills, heaviest context).

## Source

<https://github.com/affaan-m/ECC>
