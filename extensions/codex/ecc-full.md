---
name: ECC — Everything Claude Code (full)
description: ECC full profile for Codex — all Codex-applicable modules into ~/.codex (heaviest)
kind: plugin
default: false
size_mb: 4
category: harness-specific
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  # Everything Claude Code (ECC) — affaan-m/ECC. Unified installer with
  # --target codex → writes ~/.codex. ECC's "full" profile, auto-reduced for
  # Codex (every Codex-applicable module: agents, platform, database, quality,
  # security, research, optimization, devops, supply-chain, document-processing,
  # and more). Heaviest Codex footprint. See claude-code/ecc-developer for the
  # full ECC overview.
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
  node scripts/install-apply.js --target codex --profile full

  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (full profile, Codex)

The complete [ECC](https://github.com/affaan-m/ECC) install for the Codex harness
— every Codex-applicable module. See `claude-code/ecc-developer` for the full
overview of ECC.

## What this profile is

ECC's `full` profile, auto-reduced for Codex: agents + platform + database +
quality + security + research-apis + business-content + operator/optimization
workflows + devops-infra + supply-chain + document-processing + more.

Installs into `~/.codex/`: `AGENTS.md`, `config.toml`, `agents/`, `skills/`.
Approx. **3.9 MB** installed — the largest Codex footprint.

## Read before enabling — heaviest context

This is the largest ECC footprint for Codex. For most work `ecc-developer` is a
better balance of capability vs. context cost. Don't stack ECC with another
workflow framework.

## Source

<https://github.com/affaan-m/ECC>
