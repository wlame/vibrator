# claude-mem integration

[claude-mem](https://docs.claude-mem.ai/) gives Claude Code persistent memory across
sessions: it captures tool-use observations during a session, compresses them with an
LLM into summaries, and re-injects the relevant context at the start of the next
session.

Vibrator integrates with the **server-beta** runtime — the multi-client deployable
architecture that runs as a host-side Docker stack and talks to vibrator containers
over HTTP. The integration is **fully automatic**: you bring up the compose stack
once, drop the Postgres DSN into a single config file, and vibrator handles team
creation, project creation, and per-workspace API key minting on first run.

- **Zero manual SQL** — no `INSERT INTO teams`, no `api-key create`, no copy-paste UUIDs.
- **Per-workspace project-scoped keys** — a leaked key compromises one project, not your whole stack.
- **DB credentials stay host-side** — the vibrator container only ever sees its project-scoped Bearer token.
- **Idempotent** — re-running `vibrate` reuses the cached key; the bootstrap fires only when needed.

---

## How it works

```
┌─ host ──────────────────────────────────────────────────────────┐
│                                                                 │
│  docker-compose stack (one set per host):                       │
│  ┌──────────┐  ┌────────┐  ┌──────────────┐ ┌─────────────────┐ │
│  │ postgres │  │ valkey │  │  cm-server   │ │   cm-worker     │ │
│  │ (canon)  │  │ (queue)│  │  HTTP :37877 │ │ (LLM summaries) │ │
│  └──────────┘  └────────┘  └──────┬───────┘ └─────────────────┘ │
│                                   ▲                             │
│                                   │ http://host.docker.internal:37877
│                                   │                             │
│  ~/.config/vibrator/claude-mem.env                              │
│   ├── CLAUDE_MEM_RUNTIME=server-beta                            │
│   ├── CLAUDE_MEM_SERVER_BETA_URL=http://host.docker.internal:…  │
│   └── CLAUDE_MEM_SERVER_DATABASE_URL=postgres://…  ← HOST-ONLY  │
└───────────────────────────────────┼─────────────────────────────┘
                                    │
              ┌─────────────────────┼─────────────────────┐
              │   docker run (one-shot pg) for bootstrap  │
              │   ── upsert team, project, mint key ──    │
              ▼                     │                     ▼
        <ws-A>/.vb.env              │              <ws-B>/.vb.env
        cached project key          │              cached project key
              │                     │                     │
              ▼                     ▼                     ▼
         vibrator A             vibrator B            vibrator C
         hooks → /v1/*          hooks → /v1/*        hooks → /v1/*
         (HTTP + Bearer)        (HTTP + Bearer)      (HTTP + Bearer)
```

The vibrator entrypoint reads `CLAUDE_MEM_RUNTIME=server-beta` and the three
forwarded vars (`URL`, `API_KEY`, plus the resolved `TEAM_ID` and `PROJECT_ID`),
writes them into `~/.claude-mem/settings.json`, and the plugin's `runtime-selector`
POSTs hook events to `/v1/*` on the host stack. No local worker, no SQLite, no
DB driver inside the container.

---

## Setup

### Step 1 — Install the plugin on the host

```bash
# Inside any Claude Code session on the host:
/plugin marketplace add thedotmack/claude-mem
/plugin install thedotmack/claude-mem
```

This populates `~/.claude/plugins/marketplaces/thedotmack/`. The vibrator image
runs `npx claude-mem install` at build time so the hook scripts are present
inside the container too.

> **You do not need to run a Claude session on the host afterward.** The host
> install is purely a delivery mechanism for the plugin files.

### Step 2 — Bring up the docker-compose stack

```bash
git clone https://github.com/thedotmack/claude-mem.git ~/dev/claude-mem-stack
cd ~/dev/claude-mem-stack

cat > .env <<EOF
POSTGRES_USER=claudemem
POSTGRES_PASSWORD=$(openssl rand -hex 24)
POSTGRES_DB=claudemem
ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY
EOF
chmod 600 .env

docker compose up -d --build
docker compose ps     # all four services should be "healthy"
curl -fsS http://127.0.0.1:37877/healthz
```

### Step 3 — Tell vibrator about the stack

Three keys. The DSN is the only one that matters for auto-bootstrap.

```bash
mkdir -p ~/.config/vibrator
cat > ~/.config/vibrator/claude-mem.env <<EOF
CLAUDE_MEM_RUNTIME=server-beta
CLAUDE_MEM_SERVER_BETA_URL=http://host.docker.internal:37877
CLAUDE_MEM_SERVER_DATABASE_URL=postgres://claudemem:YOUR_PG_PASSWORD@host.docker.internal:5432/claudemem
EOF
chmod 600 ~/.config/vibrator/claude-mem.env
```

> **DSN format:** Use `host.docker.internal` as the host name. Vibrator runs the
> bootstrap from a one-shot `postgres:16-alpine` container with
> `--add-host=host.docker.internal:host-gateway`, which makes that hostname work
> uniformly on macOS (Docker Desktop, OrbStack, Colima, Rancher) and Linux.
> `localhost`/`127.0.0.1` also work — vibrator rewrites them to
> `host.docker.internal` automatically.
>
> **Port:** `37877` is the upstream default. If your compose stack publishes
> the server on a different port (`ports: ["43210:37877"]`), update
> `SERVER_BETA_URL` accordingly.
>
> **Team name:** defaults to `vibrators`. Override with
> `VIBRATOR_CLAUDE_MEM_TEAM_NAME=<name>` in the same dotenv if you want to
> consolidate vibrator-bootstrapped projects under an existing team.

### Step 4 — Verify

```bash
vibrate --claude-mem-status
```

Output covers each link in the chain: admin config, `/healthz`, the cached
key (or "NONE — will auto-bootstrap"), and a server-side verification that
the cached key still matches a live row in `api_keys`.

### Step 5 — Run vibrate normally

```bash
cd ~/dev/your-project
vibrate
```

On the **first** run in a workspace:

```
[vibrator] claude-mem: no cached key for this workspace — bootstrapping…
[vibrator] claude-mem: bootstrapped 'your-project' (team_id=abc12345… project_id=def67890…)
[vibrator] claude-mem: project-scoped key cached in /Users/you/dev/your-project/.vb.env
```

On subsequent runs:

```
claude-mem: settings.json bootstrapped (runtime=server-beta)
claude-mem: server-beta reachable at http://host.docker.internal:37877
claude-mem: auth OK (POST /v1/events → 400, server rejected empty body — expected)
```

---

## What gets persisted where

| Location | Contents | Visibility |
|---|---|---|
| `~/.config/vibrator/claude-mem.env` | DSN, server URL, runtime | Host only |
| `<workspace>/.vb.env` | Project-scoped API key, team_id, project_id | Host + container |
| Container env | URL, API key, team_id, project_id | Container only |
| Server Postgres | `teams`, `projects`, `api_keys` (hash only), `agent_events`, `observations` | Server |

**Important**: `<workspace>/.vb.env` carries the plaintext token. Vibrator
sets it to mode 600. **Add it to `.gitignore`** for any workspace that has
a remote repo — see the entry below.

### `.gitignore` recommendation

In every workspace you `vibrate` into, ensure `.gitignore` contains:

```
.vb.env
```

The bootstrap writes a "# claude-mem (auto-bootstrapped …)" block to this
file alongside any existing `PROFILE=` / `WITH=` / `NO=` pins from the
interactive menu writer.

---

## CLI surface

| Command | Purpose |
|---|---|
| `vibrate` | Run normally; auto-bootstraps if needed. |
| `vibrate --claude-mem-status` | Probe the host stack, show resolved wiring for cwd. |
| `vibrate --claude-mem-bootstrap` | Mint/rotate the project-scoped key for cwd; no container. |
| `vibrate --claude-mem-setup` | Print the host setup instructions. |

### Rotating a leaked key

```bash
# 1. Delete the cached lines
sed -i.bak '/^CLAUDE_MEM_SERVER_BETA_/d' .vb.env

# 2. Mint a fresh project-scoped key (revokes the old one in the same txn)
vibrate --claude-mem-bootstrap
```

The bootstrap's UPDATE step sets `revoked_at = now()` on any prior live key
for `(team, project, vibrator-actor)` before INSERT'ing the new one, so old
keys can't be reused.

---

## Security model

- **Project-scoped keys, not team-wide.** Every workspace gets its own key
  bound to a single `project_id`. The auth helper `ensureProjectAllowed`
  rejects any cross-project request server-side. Blast radius of a leaked
  key = one project's events.
- **No DB credentials in the container.** The vibrator container can only
  talk HTTP to `/v1/*` with its Bearer token. It cannot read the canonical
  store, mint new keys, or escalate scope.
- **Host-side bootstrap via ephemeral container.** The one-shot
  `postgres:16-alpine` container runs the SQL and exits. The DSN never
  hits the vibrator image's env.
- **Wildcard scope (`["*"]`) is intentional.** claude-mem's scope check
  function (`K9`) short-circuits on `"*"`. We use it because the
  `project_id` boundary is the real security perimeter; explicit scope
  arrays add brittleness (any new required scope upstream would break our
  keys) without meaningfully limiting the blast radius.

---

## Advanced: external Postgres

If you already run Postgres in your network and want claude-mem to use it instead of
the bundled one, override the compose stack to skip the bundled `postgres` service
and point at your own database.

### Database setup

```sql
-- As superuser on your existing Postgres instance:
CREATE ROLE claudemem WITH LOGIN ENCRYPTED PASSWORD 'STRONG_PASSWORD';
CREATE DATABASE claudemem OWNER claudemem;

\c claudemem
-- PG 15+: public schema lost default CREATE for non-owner. Pick one:
GRANT ALL ON SCHEMA public TO claudemem;
-- or:
ALTER SCHEMA public OWNER TO claudemem;
```

Make sure `pg_hba.conf` allows the `claudemem` role from whichever IP the
Docker host will source from, and reload Postgres.

### Compose override

Save next to the upstream `docker-compose.yml`:

```yaml
# docker-compose.override.yml — use external Postgres
services:
  claude-mem-server:
    depends_on:
      postgres: !reset null
    environment:
      CLAUDE_MEM_SERVER_DATABASE_URL: ${CLAUDE_MEM_SERVER_DATABASE_URL:?set in .env}

  claude-mem-worker:
    depends_on:
      postgres: !reset null
    environment:
      CLAUDE_MEM_SERVER_DATABASE_URL: ${CLAUDE_MEM_SERVER_DATABASE_URL:?set in .env}

  postgres:
    profiles: ["never"]
```

In your `~/.config/vibrator/claude-mem.env`, point `CLAUDE_MEM_SERVER_DATABASE_URL`
at your external instance instead of `host.docker.internal:5432`. Vibrator's
bootstrap will run there too — same flow.

---

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `--claude-mem-status` shows Health probe FAIL | Stack isn't running. `cd ~/dev/claude-mem-stack && docker compose ps`. |
| `--claude-mem-status` shows Cached key NONE but DATABASE_URL set | Normal — next `vibrate` will bootstrap. Or run `vibrate --claude-mem-bootstrap` to seed without launching. |
| `claude-mem: bootstrap SQL failed` | Check the error body — usually a DB connectivity issue or pg_hba rejection. Verify `psql "$CLAUDE_MEM_SERVER_DATABASE_URL" -c '\dt'` works from the host (substituting `host.docker.internal` → `127.0.0.1`). |
| Server verify FAIL: "key hash not found in api_keys" | The cached `.vb.env` key was deleted server-side (or DB URL points elsewhere). Delete `CLAUDE_MEM_SERVER_BETA_*` lines from `.vb.env` and re-run vibrate. |
| Server verify REVOKED | Someone (or another vibrator session) revoked this key. Re-bootstrap as above. |
| Container log: `auth REJECTED (POST /v1/events → 401/403)` | The cached key has been revoked or deleted. Same fix as above. |
| Container log: `auth REJECTED → 403 ... scoped to a different project` | A pre-existing legacy key in `~/.config/vibrator/claude-mem.env` is project-bound to a different project. Remove `CLAUDE_MEM_SERVER_BETA_API_KEY` from the admin dotenv to let auto-bootstrap take over. |
| `agent_events` rows appearing but no `observations` | Worker can't reach the LLM provider. `docker compose logs claude-mem-worker` on host — usually `ANTHROPIC_API_KEY` missing from the stack's `.env`. |
| Connection refused at `host.docker.internal:37877` | Linux native Docker without Docker Desktop: ensure the compose stack publishes on `0.0.0.0`, not just `127.0.0.1`. |
| Bootstrap pulls a 70MB `postgres:16-alpine` image on first run | Expected. One-time download, cached locally; subsequent bootstraps are fast. |

---

## Migration from the legacy explicit-key flow

If your `~/.config/vibrator/claude-mem.env` currently contains
`CLAUDE_MEM_SERVER_BETA_API_KEY`, `_TEAM_ID`, `_PROJECT_ID` (the pre-auto-bootstrap
shape), the integration **still works** — vibrator detects the explicit key and
forwards it as-is.

To switch to per-workspace project-scoped keys (recommended for blast-radius
reduction):

```bash
# 1. Remove the explicit key / ids from admin dotenv, leave DSN + URL + runtime
sed -i.bak '/^CLAUDE_MEM_SERVER_BETA_API_KEY=/d' ~/.config/vibrator/claude-mem.env
sed -i.bak '/^CLAUDE_MEM_SERVER_BETA_TEAM_ID=/d' ~/.config/vibrator/claude-mem.env
sed -i.bak '/^CLAUDE_MEM_SERVER_BETA_PROJECT_ID=/d' ~/.config/vibrator/claude-mem.env

# 2. (Optional) Revoke the old team-wide key in Postgres so it can't be reused.
#    Otherwise it remains valid alongside the new per-workspace keys.

# 3. cd into each workspace and let auto-bootstrap mint a fresh per-project key:
cd ~/dev/some-project && vibrate --claude-mem-bootstrap
cd ~/dev/another     && vibrate --claude-mem-bootstrap
```

---

## What gets stored where

- **Postgres**: events, observations, sessions, summaries, API keys (SHA-256 hashes),
  projects, audit log. The canonical truth.
- **Valkey**: BullMQ job queue only. AOF-persisted, but if it disappears, the outbox
  rows in Postgres re-enqueue.
- **`<workspace>/.vb.env`**: plaintext API key (only place plaintext exists after mint).
- **`~/.config/vibrator/claude-mem.env`**: DSN + server URL. Host only.
- **Container**: nothing persistent. The plugin's local data dir is unused.
