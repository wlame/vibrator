---
name: Supabase MCP
kind: mcp
default: false
size_mb: 2
category: databases
auth:
  env: SUPABASE_ACCESS_TOKEN
runtime_needs:
  third_party_api: Supabase
  outbound_net: true
install: |
  # Supabase hosts a remote MCP at https://mcp.supabase.com/mcp. The
  # default flow is OAuth via `codex mcp login supabase`, which is
  # awkward in headless containers — we register a bearer-token form
  # here instead, reading from SUPABASE_ACCESS_TOKEN.
  mkdir -p "$HOME/.codex"
  cat >> "$HOME/.codex/config.toml" <<'EOF'

  [mcp_servers.supabase]
  url = "https://mcp.supabase.com/mcp"
  bearer_token_env_var = "SUPABASE_ACCESS_TOKEN"
  EOF
source: https://github.com/supabase-community/supabase-mcp
host_aliases: [supabase]
---

# Supabase MCP

Official Supabase MCP server. Manage projects, run SQL against Supabase
Postgres, inspect schemas, read logs, deploy Edge Functions — all from
inside a Codex session.

## Auth

Modern Supabase MCP defaults to **browser-based OAuth** via dynamic
client registration. That path is fine on a laptop but breaks inside a
headless container.

Vibrator's recommended flow:

1. On your laptop, mint a Personal Access Token at
   [supabase.com/dashboard/account/tokens](https://supabase.com/dashboard/account/tokens)
2. Set `SUPABASE_ACCESS_TOKEN` on the host
3. Vibrator forwards it into the container; Codex sends it as a Bearer
   header to `https://mcp.supabase.com/mcp`

The token is org-scoped — prefer one PAT per project with a clear name
like `vibrator-<project>`.

## Why opt-in

Pulls in your Supabase org from inside the container. Always scope the
PAT to the minimum permissions you need (read-only for inspection-only
sessions).
