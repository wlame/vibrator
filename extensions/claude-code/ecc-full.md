---
name: ECC — Everything Claude Code (full)
description: ECC full profile — ALL modules (~195 skills) into ~/.claude (heaviest context)
kind: plugin
default: false
size_mb: 8
category: harness-specific
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  # Everything Claude Code (ECC) — affaan-m/ECC. Installed into ~/.claude via
  # ECC's own manifest-driven installer. The "full" profile installs ALL
  # classified modules — every agent, skill, rule, command, and hook ECC ships.
  # Heaviest agent-context footprint. See ecc-developer for the full overview.
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
  node scripts/install-apply.js --target claude --profile full

  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (full profile)

The complete [ECC](https://github.com/affaan-m/ECC) install — every classified
module. See `ecc-developer` for the full overview of ECC and the `ecc-*` family.

## What this profile is

ECC's `full` profile: all modules — `rules-core`, `agents-core`, `commands-core`,
`hooks-runtime`, `platform-configs`, `framework-language`, `database`,
`workflow-quality`, `security`, `research-apis`, `business-content`,
`operator-workflows`, `optimization-workflows`, and more.

Installs into `~/.claude/`: 63 agents · **~195 skills** · 115 rule files · 79
commands · 4 hooks. Approx. **7.4 MB** installed.

## Read before enabling — heaviest context

This is the **largest** ECC footprint. ~195 skills + 63 agents in `~/.claude`
means a very large capability surface for Claude Code to consider every session.
Only choose `ecc-full` if you genuinely want the entire library available; for
most work `ecc-developer` (79 skills) is the better balance of capability vs.
context cost. Stacking ECC with another workflow framework (`superpowers`,
`superclaude`) is not recommended.

## Source

<https://github.com/affaan-m/ECC>
