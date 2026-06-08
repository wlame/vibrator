---
name: ECC — Everything Claude Code (security)
description: ECC security profile — core + security agents/guidance into ~/.claude
kind: plugin
default: false
size_mb: 4
category: security
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  # Everything Claude Code (ECC) — affaan-m/ECC. Installed into ~/.claude via
  # ECC's own manifest-driven installer. The "security" profile is the core
  # baseline plus ECC's security module (security-reviewer agents, vulnerability
  # /secret/injection guidance). See ecc-developer for the full overview.
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
  node scripts/install-apply.js --target claude --profile security

  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (security profile)

The security-focused [ECC](https://github.com/affaan-m/ECC) install. See
`ecc-developer` for the full overview of ECC and the complete `ecc-*` family.

## What this profile is

ECC's `security` profile: the `core` baseline plus ECC's `security` module —
security-reviewer agents and security-specific guidance (injection, secrets,
path-traversal, dependency hygiene). Pick this when ECC's security-first tooling
is the main draw.

Installs into `~/.claude/`: 63 agents · 35 skills · 115 rule files · 79 commands
· 4 hooks. Approx. **4.4 MB** installed.

> ECC also ships an `aura` security adapter (an AgentShield-style scanner). It is
> not part of this content profile; if it later runs as a service it would become
> a separate vibrator integration (see the integration plan).

## When to pick a different profile

- Just the lean baseline? → `ecc-core`.
- Default engineering preset? → `ecc-developer`.
- Research/content workflows? → `ecc-research`.
- Everything (incl. security)? → `ecc-full`.

## Source

<https://github.com/affaan-m/ECC>
