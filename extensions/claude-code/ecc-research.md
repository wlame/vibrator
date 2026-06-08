---
name: ECC — Everything Claude Code (research)
description: ECC research profile — core + research/content/synthesis workflows into ~/.claude
kind: plugin
default: false
size_mb: 4
category: harness-specific
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  # Everything Claude Code (ECC) — affaan-m/ECC. Installed into ~/.claude via
  # ECC's own manifest-driven installer. The "research" profile is the core
  # baseline plus research/content modules (research-apis, business-content,
  # social-distribution). See ecc-developer for the full overview.
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
  node scripts/install-apply.js --target claude --profile research

  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (research profile)

The research/content-oriented [ECC](https://github.com/affaan-m/ECC) install.
See `ecc-developer` for the full overview of ECC and the complete `ecc-*` family.

## What this profile is

ECC's `research` profile: the `core` baseline plus research- and content-oriented
modules (`research-apis`, `business-content`, `social-distribution`) — skills for
current-state research, multi-source synthesis with citations, market research,
and publishing workflows.

Installs into `~/.claude/`: 63 agents · 41 skills · 115 rule files · 79 commands
· 4 hooks. Approx. **4.4 MB** installed.

> Many research skills make outbound calls to web-search / model-provider
> backends at runtime (hence the `outbound_net` badge). Some adjacent ECC search
> integrations may want their own API keys — enable those separately if needed.

## When to pick a different profile

- Just the lean baseline? → `ecc-core`.
- Default engineering preset? → `ecc-developer`.
- Security-focused? → `ecc-security`.
- Everything (incl. research)? → `ecc-full`.

## Source

<https://github.com/affaan-m/ECC>
