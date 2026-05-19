---
name: oh-my-pi (omp)
kind: plugin
default: false
size_mb: 0
deps:
  features: [node]
install: |
  # oh-my-pi (omp) is a Rust-rewrite fork of pi-mono by can1357. Heavier
  # surface — 40+ providers, 32 built-in tools, 13 LSP ops, 27 DAP ops.
  npm install -g @can1357/oh-my-pi
source: https://github.com/can1357/oh-my-pi
---

# oh-my-pi (omp)

A more feature-rich fork of pi-mono by can1357. ~27k lines of Rust core,
extended with a batteries-included coding workflow:

- 40+ provider integrations
- 32 built-in tools (vs pi's 4-tool core)
- 13 LSP operations
- 27 DAP (Debug Adapter Protocol) operations

## When to enable

If you want pi's philosophy (minimal core, extensible) but find the
upstream's "no MCP / no subagents / no plan mode" stance too austere.
omp adds those concepts via its tool set.

## Conflict with vanilla pi

omp installs as `omp`, not `pi` — so both can coexist on the same image.
But you'll typically pick one or the other.

Default = off. Pick this if you've already tried vanilla pi and want more
batteries-included features.
