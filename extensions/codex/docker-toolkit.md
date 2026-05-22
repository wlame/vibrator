---
name: Docker MCP Toolkit
kind: mcp
default: false
size_mb: 50
category: cloud-infrastructure
runtime_needs:
  local_only: true
install: |
  # Docker's MCP Gateway is a single MCP that proxies many containerised
  # MCP servers from Docker's curated catalog. The CLI helper writes the
  # right config.toml entry for Codex. Requires Docker Desktop / the
  # docker mcp plugin to be installed in the base image (the `docker`
  # feature provides this).
  docker mcp client configure codex
source: https://github.com/docker/mcp-gateway
host_aliases: [docker-toolkit, mcp-gateway]
---

# Docker MCP Toolkit

A single MCP entry — `MCP_DOCKER` — that fronts Docker's catalog of
200+ containerised MCP servers. Instead of installing each MCP into the
image, you enable servers in Docker Desktop / the gateway profile and
Codex sees them through one stdio connection.

Verify the install with:

```bash
codex mcp list
# MCP_DOCKER  ...  enabled
```

## Why opt-in

Two reasons it isn't a default:

1. **Footprint** — pulls Docker tooling and the gateway binary. Worth it
   if you'd otherwise install 5+ MCPs individually, but heavy for users
   who only need 1–2 servers.
2. **Trust posture** — the gateway runs MCP servers in containers on
   *your* machine, so its network and filesystem access need explicit
   scoping. Review the per-server profile before flipping things on.

## How it composes

Inside the gateway, each MCP server runs in its own container with the
gateway acting as a multiplexer. From Codex's perspective there is one
MCP; from Docker's perspective there are N containers managed by
profiles.

Use this when you want to **try MCPs without polluting your base image**
or when an MCP isn't ergonomic to install directly (heavy native deps,
language runtimes you don't otherwise need).

## Alternatives

If you only need 1–3 MCPs and they install cleanly, prefer adding them
directly via the dedicated vibrator extensions (Serena, Context7,
Playwright, etc.). The toolkit shines as N grows.
