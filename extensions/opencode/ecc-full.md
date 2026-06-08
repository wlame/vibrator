---
name: ECC — Everything Claude Code (full)
description: ECC full profile for OpenCode — all applicable modules into ~/.opencode (heaviest)
kind: plugin
default: false
size_mb: 5
category: harness-specific
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  # Everything Claude Code (ECC) — affaan-m/ECC. Unified installer with
  # --target opencode → writes ~/.opencode. The "full" profile installs every
  # OpenCode-applicable module — heaviest footprint. See claude-code/ecc-developer
  # for the full ECC overview.
  #
  # OpenCode needs the compiled plugin payload (.opencode/dist) built first via
  # `npm run build:opencode`, so this does a FULL npm install then build before
  # installing. Clone + node_modules (~90 MB) dropped afterward; ~/.opencode
  # (~4.8 MB) persists.
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
  node scripts/install-apply.js --target opencode --profile full

  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (full profile, OpenCode)

The complete [ECC](https://github.com/affaan-m/ECC) install for OpenCode — every
OpenCode-applicable module. See `claude-code/ecc-developer` for the full overview
of ECC.

## What this profile is

ECC's `full` profile for OpenCode: commands + hooks + platform + database +
quality + orchestration + security + research-apis + business-content +
operator/optimization workflows + devops-infra + supply-chain +
document-processing + more.

Installs into `~/.opencode/`. Approx. **4.8 MB** installed — the largest OpenCode
footprint.

## Read before enabling — heaviest context

This is the largest ECC footprint for OpenCode. For most work `ecc-developer` is
a better balance of capability vs. context cost. Don't stack ECC with another
workflow framework.

## OpenCode build step

OpenCode's installer requires `.opencode/dist` compiled first
(`npm run build:opencode`); the snippet handles this automatically.

## Source

<https://github.com/affaan-m/ECC>
