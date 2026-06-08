---
name: ECC — Everything Claude Code (research)
description: ECC research profile for Codex — core + research/content modules into ~/.codex
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
  # --target codex → writes ~/.codex. ECC's "research" profile, auto-reduced for
  # Codex (core + research-apis + business-content + social-distribution). See
  # claude-code/ecc-developer for the full ECC overview.
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
  node scripts/install-apply.js --target codex --profile research

  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (research profile, Codex)

The research/content-oriented [ECC](https://github.com/affaan-m/ECC) install for
the Codex harness. See `claude-code/ecc-developer` for the full overview of ECC.

## What this profile is

ECC's `research` profile, auto-reduced for Codex: `agents-core`,
`platform-configs`, `workflow-quality`, plus `research-apis`, `business-content`,
and `social-distribution` — research, synthesis, and publishing skills.

Installs into `~/.codex/`: `AGENTS.md`, `config.toml`, `agents/`, `skills/`.
Approx. **2.1 MB** installed.

> Many research skills make outbound calls at runtime (hence `outbound_net`).

## When to pick a different profile

- Leaner baseline? → `ecc-core`.
- Default engineering preset? → `ecc-developer`.
- Security focus? → `ecc-security`.
- Everything ECC ships for Codex? → `ecc-full`.

## Source

<https://github.com/affaan-m/ECC>
