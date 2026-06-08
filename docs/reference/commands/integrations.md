# `vibrate integrations`

Interactive setup for host-side [integrations](../../guides/integrations.md) — services
that run on your machine and get wired into the container (currently
[Serena](../../integrations/serena.md) and [claude-mem](../../integrations/claude-mem.md)).

```bash
vibrate integrations [INTEGRATION_ID]
```

## Bare invocation

```bash
vibrate integrations
```

Shows an interactive picker listing every registered integration (built-ins plus any
user-defined TOML descriptors). Select one to manage it.

## `vibrate integrations claude-mem`

```bash
vibrate integrations claude-mem
```

The hand-written setup + management flow for [claude-mem](../../integrations/claude-mem.md):

1. **Admin config form** — server URL, team name, stack directory, and the Postgres DSN.
   Saved to `~/.config/vibrator/claude-mem.toml` (mode `0600`).
2. **Runner** — start/stop the Docker Compose stack, probe the server, bootstrap the
   workspace key, or tail logs.

The Postgres DSN is a **host-only** credential — it never crosses into a container. Only
the resulting project-scoped bearer token does.

## `vibrate integrations serena`

```bash
vibrate integrations serena
```

Manage the [Serena](../../integrations/serena.md) host server — show status, start/stop it
as a background process or a Docker container, and tail logs.

## Related

- [Integrations guide](../../guides/integrations.md) — hosting modes, transport switching,
  and how integrations wire into the container.
- [Serena](../../integrations/serena.md) · [claude-mem](../../integrations/claude-mem.md) —
  per-integration setup pages.
- [`vibrate prereqs`](prereqs.md) — the per-workspace bootstrap side (e.g. minting a
  claude-mem key).
