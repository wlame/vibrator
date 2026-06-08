# Integrations

An **integration** connects your workspace container to a **host-side service** — an MCP
server, a memory system — without exposing host credentials to the container or making you
hand-configure Docker networking. Vibrator ships two built-ins:

| Integration | What it is | Page |
|-------------|-----------|------|
| [Serena](../integrations/serena.md) | code-aware MCP server (symbol search, references) | [Serena](../integrations/serena.md) |
| [claude-mem](../integrations/claude-mem.md) | persistent agent memory | [claude-mem](../integrations/claude-mem.md) |

This guide covers the mechanics common to all integrations. The linked pages cover setup
for each.

## How integrations wire in

Each integration is declared as a Go descriptor that self-registers. At
[image-build time](../lifecycle/build.md), the descriptors are serialized — filtered to the
harness being built — into `/etc/vibrator/integrations.json` inside the image. Then the
[`claude-exec` wrapper](../lifecycle/startup.md#3-the-claude-exec-wrapper) reads that
manifest **on every session entry** and wires each integration's MCP transport into
`~/.claude.json`, plus any env vars it declares.

```mermaid
flowchart LR
    D[Integration descriptor] -->|build time| M[/etc/vibrator/integrations.json]
    M -->|every session| CE[claude-exec]
    CE -->|http or stdio| CJ[~/.claude.json mcpServers]
```

Because the wiring is re-evaluated on every entry (not just first run), starting or
stopping a host server between sessions is picked up automatically — no rebuild.

## Hosting modes

You choose a policy per integration, per workspace. It's stored in
[`.vb`](configuration.md) under `[integrations]` (keyed by integration ID), and forwarded
into the container as `VIBRATOR_INTEGRATION_MODE_<ID>`:

| Mode | Behavior |
|------|----------|
| `auto` *(default)* | Probe the host server. Reachable → use HTTP. Unreachable → fall back to a container-local instance (stdio), with a visible warning. |
| `host` | Use HTTP only. If the host server is unreachable, **warn loudly and don't fall back** — the failure stays visible. |
| `local` | Always use the container-local instance (stdio); never probe the host. |
| `off` | Remove the integration's MCP entry entirely. |

```toml
# .vb
[integrations]
serena     = "host"     # require my host Serena server
claude-mem = "auto"     # probe, fall back if down
```

A missing key means `auto`.

## Transport switching in practice

In `auto` mode the container probes the integration's HTTP URL with a short timeout on
session entry:

- **Reachable** → writes an `{ type: "http", url: ... }` MCP entry.
- **Unreachable** → writes the stdio fallback (e.g. spawning the server locally) and warns.

Set `VIBRATOR_VERBOSE=1` to see the decision on each entry
(`serena: switched to http transport` / `serena: fell back to stdio`).

## Pre-launch readiness checks

Before entering the container, `vibrate` runs each integration's launch checks. A failing
check prints a warning with a fix hint and, for fixable gaps, offers an inline prompt:

```
⚠  [claude-mem] no workspace key — all auth'd requests will return 401
   hint: a project-scoped bearer token must be minted against the host postgres
   run:  vibrate prereqs bootstrap claude-mem-server-beta
   Bootstrap now? [y/N]
```

!!! info "Checks never block the launch"
    A failing readiness check is a warning, never a hard stop. A dormant integration is
    better than blocking you from reaching your container. Answering `y` to a fixable check
    runs the fix inline and saves the result to `.vb` before continuing.

## Setting up the host side

Use [`vibrate integrations`](../reference/commands/integrations.md) to configure and run
the host service, and [`vibrate prereqs`](../reference/commands/prereqs.md) for
per-workspace bootstrap (like minting a claude-mem key).

```bash
vibrate integrations            # interactive picker
vibrate integrations serena     # manage the Serena host server
vibrate integrations claude-mem # configure + run claude-mem
```

## Related

- [Serena](../integrations/serena.md) — code intelligence MCP, with `auto`/`host`/`local`
  fallback.
- [claude-mem](../integrations/claude-mem.md) — persistent memory, with host admin config
  and per-workspace keys.
- [Environment variables](../reference/environment-variables.md) —
  `VIBRATOR_INTEGRATION_MODE_*`, `SERENA_PORT`, and more.
