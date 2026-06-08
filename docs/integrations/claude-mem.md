# claude-mem

[claude-mem](https://github.com/thedotmack/claude-mem) gives the agent **persistent
memory** — it records observations from every session and injects relevant context into
future prompts. Vibrator wires it into the [Claude Code](../guides/harnesses.md#claude-code)
harness in **server-beta** mode: a long-running stack on your host handles storage, and each
workspace gets a project-scoped bearer token.

## Architecture

```text
Host
├── Postgres (your own instance or bundled)
├── claude-mem-server   (HTTP API: /healthz, /v1/events)
├── claude-mem-worker   (processes observations from the queue)
└── Valkey              (job queue)

Workspace container
├── CLAUDE_MEM_RUNTIME=server-beta
├── CLAUDE_MEM_SERVER_BETA_URL=http://host.docker.internal:37877
├── CLAUDE_MEM_SERVER_BETA_API_KEY=cmem_...
├── CLAUDE_MEM_SERVER_BETA_TEAM_ID / _PROJECT_ID
└── ~/.claude-mem/settings.json   (written by the entrypoint)
```

Credentials flow in three stages:

1. **Admin config** (host-only, never forwarded): Postgres DSN, server URL, team name.
2. **Workspace bootstrap** (per project, stored in `.vb`): a project-scoped bearer token
   minted via a one-shot `postgres:16-alpine` container.
3. **Container launch**: Vibrator reads both and forwards `CLAUDE_MEM_*` env vars; the
   [entrypoint](../lifecycle/startup.md#2-the-entrypoint-entrypointsh) writes
   `~/.claude-mem/settings.json` and probes auth.

## One-time host setup

Run once per machine:

```bash
vibrate integrations claude-mem
```

This creates `~/.config/vibrator/claude-mem.toml` (path printed in the UI):

```toml
runtime      = "server-beta"
server_url   = "http://host.docker.internal:37877"
database_url = "postgres://user:pass@localhost:5432/claude_mem"
team_name    = "vibrators"            # default
stack_dir    = "~/dev/claude-mem-stack"  # default
```

!!! warning "The DSN never leaves the host"
    `database_url` is a host-only credential. It is never forwarded to any container — only
    the resulting bearer token crosses the boundary.

### Admin config path

Resolved in order: `$VIBRATOR_CLAUDE_MEM_CONFIG` → `$XDG_CONFIG_HOME/vibrator/claude-mem.toml`
→ `~/.config/vibrator/claude-mem.toml`.

### Using your own Postgres

Set `database_url` to your existing instance. The bootstrap runs a one-shot
`postgres:16-alpine` container that rewrites `localhost`/`127.0.0.1` → `host.docker.internal`
so it can reach your host Postgres. You can skip `stack_dir` and run only the server/worker
containers, pointing their `DATABASE_URL` at `host.docker.internal`.

## Per-workspace bootstrap

Run once per project (or accept the inline prompt at launch):

```bash
cd ~/my-project
vibrate prereqs bootstrap claude-mem-server-beta
```

What it does:

1. Spins up a one-shot `postgres:16-alpine` container.
2. Upserts a **team** (`vibrators` by default, configurable in the admin config).
3. Upserts a **project** (your workspace basename).
4. Revokes any prior live key for this `(team, project, actor)` tuple.
5. Inserts a fresh `cmem_<48-hex>` key, storing only its SHA-256 hash in Postgres.
6. Saves `api_key`, `team_id`, `project_id`, `actor_id` to `.vb` under
   [`[prereqs.claude-mem-server-beta]`](../reference/vb-file.md#prereqs).
7. Adds `.vb` to `.gitignore`.

The actor identifier is `vibrator:<hostname>:<workspace-path>`, used in audit logs.

To **rotate** a key:

```bash
vibrate prereqs bootstrap claude-mem-server-beta --force
```

## Pre-launch checks

When `claude-mem` is in the workspace's [extensions](../guides/extensions.md), Vibrator runs
three checks before entering the container:

| Check | Tests | On failure |
|-------|-------|------------|
| `admin-config` | `claude-mem.toml` exists with `server_url` | warning + hint |
| `server-probe` | `GET <server_url>/healthz` returns 200 | warning + hint |
| `workspace-key` | `.vb` has `[prereqs.claude-mem-server-beta].api_key` | **offer inline bootstrap** |

For a missing key you'll see:

```text
⚠  [claude-mem] no workspace key — all auth'd requests will return 401
   hint: a project-scoped bearer token must be minted against the host postgres
   run:  vibrate prereqs bootstrap claude-mem-server-beta
   Bootstrap now? [y/N]
```

Answering `y` mints the key inline, saves it to `.vb`, and proceeds with the launch.

## Inside the container

The entrypoint's claude-mem step runs when `CLAUDE_MEM_RUNTIME=server-beta` and
`CLAUDE_MEM_SERVER_BETA_URL` are set:

1. Writes `~/.claude-mem/settings.json` from the env vars (idempotent — preserves existing
   fields when a var is empty).
2. Probes `GET <server_url>/healthz`.
3. Auth-probes `POST <server_url>/v1/events` with the bearer token:

| HTTP status | Meaning | Logged as |
|-------------|---------|-----------|
| 200–202 | auth OK, event accepted | `claude-mem: auth OK` |
| 400, 422 | server reachable, token *not* rejected (bad/empty body — expected) | verbose only |
| 401, 403 | **key rejected** — rotate it | `claude-mem: WARNING — auth REJECTED` |
| connection refused | server not running | skipped silently |

## Status check

```bash
cd ~/my-project
vibrate prereqs status
```

Reports the admin config path and fields, the server probe result, and the workspace cache
(`api_key` prefix, `team_id`, `project_id`).

## Troubleshooting

**No `CLAUDE_MEM_*` vars in the container** — Vibrator forwards them only when *both* the
admin config and the workspace key exist. Check `cat ~/.config/vibrator/claude-mem.toml` and
`grep -A5 prereqs .vb`, then run `vibrate prereqs status`.

**`auth REJECTED` on startup** — the key was revoked. Rotate and rebuild:

```bash
vibrate prereqs bootstrap claude-mem-server-beta --force
vibrate --rebuild
```

**Bootstrap: "docker: command not found"** — the bootstrap needs Docker running on the host
(it uses a one-shot `postgres:16-alpine` container).

**Bootstrap: psql connection error** — if Postgres is on `localhost`/`127.0.0.1`, the DSN is
rewritten to `host.docker.internal` automatically; ensure Postgres accepts connections from
Docker's bridge network (`pg_hba.conf`).

## Related pages

- [Integrations guide](../guides/integrations.md) — hosting modes and readiness checks.
- [`vibrate prereqs`](../reference/commands/prereqs.md) — status + bootstrap.
- [`vibrate integrations`](../reference/commands/integrations.md) — host admin config + stack.
- [`.vb` `[prereqs]`](../reference/vb-file.md#prereqs) — where the key is stored.
