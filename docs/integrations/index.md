# Integrations

Vibrator integrations connect your workspace container to **host-side services** — MCP
servers, memory systems, observability tools — without exposing host credentials to the
container or requiring manual Docker networking.

This page covers the two built-in integrations and the mechanics they share. For the
conceptual guide (hosting modes, transport switching, readiness checks), see
[Integrations](../guides/integrations.md).

## Built-in integrations

| ID | Name | Category | Setup |
|----|------|----------|-------|
| `serena` | [Serena MCP](serena.md) | code intelligence | [Serena](serena.md) |
| `claude-mem` | [claude-mem](claude-mem.md) | memory | [claude-mem](claude-mem.md) |

## How an integration is defined

Each integration is a Go descriptor (in `internal/integration/<name>/`) that self-registers
at startup. A descriptor specifies:

| Field | Purpose |
|-------|---------|
| `Runtimes` | Ways to run it on the host — background **process**, **Docker** container, or **Compose** stack |
| `ProbeFn` | An HTTP/TCP reachability check |
| `Wiring` | How the container consumes it (MCP entry + env vars) |
| `AdminConfig` | Path to a host-only config file (may hold credentials) |
| `Workspace` | Per-workspace credential bootstrap (optional) |
| `LaunchChecks` | Pre-launch readiness checks |

### Build-time manifest

At image build, each integration's wiring (filtered to the harness being built) is
serialized into `/etc/vibrator/integrations.json` inside the image. The
[`claude-exec` wrapper](../lifecycle/startup.md#3-the-claude-exec-wrapper) reads it on every
session entry and:

- For **MCP HTTP** entries — probes the URL, writes the transport to `~/.claude.json`.
- For **MCP stdio** entries — writes the command (unconditionally, or as the fallback).
- Respects the per-workspace `VIBRATOR_INTEGRATION_MODE_<ID>` override.

### Hosting modes

Set per workspace in `.vb` under `[integrations]`:

| Mode | Behavior |
|------|----------|
| `auto` *(default)* | Probe the host server → HTTP if reachable, else stdio fallback. |
| `host` | HTTP only; warn loudly if unreachable (no fallback). |
| `local` | stdio only; never probe the host. |
| `off` | Remove the MCP entry entirely. |

### Pre-launch readiness checks

Before entering the container, Vibrator runs each integration's `LaunchChecks`. A failing
check prints a warning and, for fixable gaps, offers an inline `Bootstrap now? [y/N]`
prompt. **Failing checks never block the launch** — a dormant integration beats blocking
you from your container.

## Runtime kinds

An integration can run on the host in one of three ways:

| Kind | Used by | What it does |
|------|---------|--------------|
| **process** | Serena | A detached background process tracked by a PID file + log under `~/.local/share/vibrator/`. |
| **docker** | Serena | A single managed container with `--restart unless-stopped`. |
| **compose** | claude-mem | A `docker compose` stack (Postgres + worker + server). |

## Adding a new integration

1. Create `internal/integration/<name>/descriptor.go`.
2. Call `integration.Register(descriptor())` from `init()`.
3. Blank-import the package so it registers.
4. Add a `LaunchChecks` slice with at least a server-probe check.
5. Add a docs page here.

It then appears in `vibrate integrations`, gets its wiring baked into every new image, and
its checks run on every launch.

## Related pages

- [Integrations guide](../guides/integrations.md) — the conceptual walkthrough.
- [`vibrate integrations`](../reference/commands/integrations.md) — host-side setup command.
- [`vibrate prereqs`](../reference/commands/prereqs.md) — per-workspace bootstrap.
