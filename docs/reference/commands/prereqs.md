# `vibrate prereqs`

Inspect and bootstrap **host-side prerequisites** — things that must exist on your machine
for an integration to work, like a reachable claude-mem server and a per-workspace API key.

```bash
vibrate prereqs <status|bootstrap> [args] [flags]
```

A prerequisite has a *verifier* (probes whether it's satisfied) and an optional
*bootstrapper* (sets it up). Today the built-in prereq is `claude-mem-server-beta`.

---

## `vibrate prereqs status` { #vibrate-prereqs-status }

```bash
vibrate prereqs status [--no-color]
```

Probes every registered prerequisite and prints a one-screen report. For
[claude-mem](../../integrations/claude-mem.md) that covers:

1. **Admin config** — path, and the parsed `runtime` / `server_url` / `database_url`.
2. **Server probe** — whether `GET <server_url>/healthz` is reachable.
3. **Workspace cache** — the `.vb` values for this workspace (`api_key` prefix, `team_id`,
   `project_id`).

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--no-color` | `false` | Disable ANSI colors (for logs / non-TTY output). |

### Exit code

`0` when every required step passes; `1` when at least one required step fails — so it's
usable as a gate in scripts.

---

## `vibrate prereqs bootstrap` { #vibrate-prereqs-bootstrap }

```bash
vibrate prereqs bootstrap PREREQ_ID [--force] [--no-color]
```

Runs the host-side setup for a prerequisite. For `claude-mem-server-beta` this mints a
**project-scoped API key** against your host's claude-mem Postgres and persists it in `.vb`
under `[prereqs.claude-mem-server-beta]`.

It is **idempotent**: if `.vb` already has a cached value it skips re-minting unless you
pass `--force`. It also idempotently adds `.vb` to `.gitignore`.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--force` | `false` | Re-mint (rotate) even if a cached key exists. |
| `--no-color` | `false` | Disable ANSI colors. |

### Example

```bash
cd ~/my-project
vibrate prereqs bootstrap claude-mem-server-beta
```

```
✓ team_id    = ...
✓ project_id = ...
✓ api_key    = cmem_…  (length 53)
✓ persisted in .vb [prereqs.claude-mem-server-beta]
✓ added .vb to .gitignore
```

To rotate a revoked key:

```bash
vibrate prereqs bootstrap claude-mem-server-beta --force
```

!!! note "Inline bootstrap at launch"
    You usually don't need to run this by hand. When you launch a workspace whose
    extensions include claude-mem and the key is missing, `vibrate` warns and offers to
    bootstrap inline — `Bootstrap now? [y/N]`. See
    [claude-mem](../../integrations/claude-mem.md#pre-launch-checks).

## Related

- [claude-mem integration](../../integrations/claude-mem.md) — the full credential flow.
- [`vibrate integrations`](integrations.md) — the host-side admin config + stack
  management.
- [The `.vb` file](../vb-file.md#prereqs) — where minted keys are stored.
