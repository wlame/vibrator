# Serena

[Serena](https://github.com/oraios/serena) is a code-aware MCP server that gives the agent
semantic access to your project — symbol search, find-references, go-to-definition, and
more. Vibrator wires Serena into the [Claude Code](../guides/harnesses.md#claude-code)
harness automatically.

## How it works

Serena has a **container-local fallback** (stdio via `uvx`) and an optional **host server**
(HTTP). On every session entry, the
[`claude-exec` wrapper](../lifecycle/startup.md#3-the-claude-exec-wrapper) decides which to
use based on your [hosting mode](../guides/integrations.md#hosting-modes):

```text
Container (claude-exec, every session entry)
  └─ probes http://host.docker.internal:<port>/mcp
       ├─ reachable   → writes an http MCP entry in ~/.claude.json
       └─ unreachable → writes a stdio MCP entry (uvx spawns Serena locally)
```

In `auto` mode (the default) the fallback is transparent — the agent always has Serena,
whether or not a host server is running.

## Setup

### Auto mode (no setup)

Nothing to do. `claude-exec` spawns Serena locally via `uvx` on first use, so the agent gets
symbol search out of the box. The trade-off is a brief `uvx` cold-start on each new session.

### Host server mode (faster, persistent)

Running Serena as a persistent host process avoids the cold-start. Start it via Vibrator:

```bash
vibrate integrations serena
# choose the "process" runtime  → Start   (background process, survives terminal close)
# or the  "docker"  runtime     → Start   (container with --restart unless-stopped)
```

Or run it yourself (for systemd / launchd / screen):

```bash
uvx --from git+https://github.com/oraios/serena \
    serena start-mcp-server --transport=streamable-http \
    --host=0.0.0.0 --port=8765
```

Then pin the workspace to host mode:

```toml
# .vb
[integrations]
serena = "host"
```

With `host` mode, a [pre-launch check](#pre-launch-check) warns if the server is unreachable
(there's no fallback in host mode).

## Port

Serena listens on **`8765`** by default. Override it by setting `SERENA_PORT` on the host
before starting Vibrator (and the host server). The same value is used host-side and in the
container's `host.docker.internal:<port>` URL.

The Docker runtime managed by `vibrate integrations serena` runs the container named
`vibrate-serena` from `ghcr.io/astral-sh/uv` with the port bound to `127.0.0.1` and a
persistent uv cache volume.

## Pre-launch check

| Check | Condition | Severity |
|-------|-----------|----------|
| `server-probe` | mode = `host` and the server is unreachable | warning |

The check only runs in `host` mode. In `auto` mode an unreachable server simply falls back
to stdio, silently.

## Verifying it works

Inside the container:

```bash
jq '.mcpServers.serena' ~/.claude.json
# {"type":"http", ...}  → host server in use
# {"type":"stdio", ...} → local fallback (uvx)
```

With `VIBRATOR_VERBOSE=1`, `claude-exec` prints `serena: switched to http transport` or
`serena: fell back to stdio` on each session entry.

## Troubleshooting

**"Serena server not reachable"**

- Check status: `vibrate integrations serena`.
- Ensure the host server binds `0.0.0.0`, not just `127.0.0.1`.
- Verify `host.docker.internal` resolves inside the container:
  `ping host.docker.internal`.

**"uvx not found" inside the container**

- The local fallback needs `uvx` on PATH. With `profile=minimal` (no Python feature),
  install uv: `curl -LsSf https://astral.sh/uv/install.sh | sh`.

**Switching transports without rebuilding**

- `claude-exec` re-probes on every session entry, including `docker exec` re-entries. Start
  the host server, then re-enter: `exit` the session and run `vibrate` again.

## Related pages

- [Integrations guide](../guides/integrations.md) — hosting modes and transport switching.
- [`vibrate integrations`](../reference/commands/integrations.md) — managing the host server.
- [Environment variables](../reference/environment-variables.md) — `SERENA_PORT`.
