---
id: claude-mem
name: claude-mem
kind: plugin
default: false
size_mb: 8
deps:
  features: [node, postgres-client]
prereq: claude-mem-server-beta
install: |
  # Host-side bootstrap pre-mints a project-scoped API key into .vb; the
  # plugin scripts inside the container only need the marketplace clone +
  # settings.json hook entries. This mirrors the bash version's manual
  # install path (Dockerfile Stage 3) — we deliberately bypass
  # `npx claude-mem install`, whose non-TTY defaults silently say "No" to
  # the "Overwrite existing installation?" prompt.
  #
  # Pre-create ~/.claude-mem as the unprivileged user. The D7 bind mount
  # (internal/app/launch.go) mounts the host cache onto ~/.claude-mem/cache;
  # if the parent ~/.claude-mem doesn't already exist, the Docker daemon
  # auto-creates it as ROOT, leaving the worker daemon (running as this user)
  # unable to write its logs/DB/port file. Creating it here keeps it
  # user-owned so the worker runtime can start.
  mkdir -p "$HOME/.claude-mem"
  mkdir -p "$HOME/.claude/plugins/marketplaces"
  git clone --depth 1 https://github.com/thedotmack/claude-mem.git \
    "$HOME/.claude/plugins/marketplaces/thedotmack"
  # `git rev-parse --short=12` returns the 12-char short SHA directly,
  # which is both POSIX-safe (the install RUN block runs under /bin/sh,
  # often dash, which doesn't support bash's ${VAR:0:N} substring) and
  # one variable shorter than computing the full SHA then slicing it.
  CM_GIT_SHORT=$(cd "$HOME/.claude/plugins/marketplaces/thedotmack" && git rev-parse --short=12 HEAD)
  CM_DEST="$HOME/.claude/plugins/cache/thedotmack/claude-mem/$CM_GIT_SHORT"
  mkdir -p "$CM_DEST"
  cp -r "$HOME/.claude/plugins/marketplaces/thedotmack/plugin/." "$CM_DEST/"
auth:
  env: CLAUDE_MEM_SERVER_BETA_API_KEY
source: https://github.com/thedotmack/claude-mem
---

# claude-mem

Persistent memory across Claude Code sessions. Captures tool-use observations
during a session, summarizes them with an LLM in a background worker, and
re-injects relevant context at the start of subsequent sessions.

Vibrator integrates with claude-mem's **server-beta** runtime — the multi-client
architecture that runs as a host-side Docker stack and talks to vibrator
containers over HTTP. Per-workspace project-scoped keys are auto-minted on
first `vibrate` and cached in `.vb`.

## Host setup

Bring up the docker-compose stack once on your host:

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
curl -fsS http://127.0.0.1:37877/healthz   # → 200 OK
```

Then drop the DSN where vibrator can find it (three keys, that's it):

```bash
mkdir -p ~/.config/vibrator
cat > ~/.config/vibrator/claude-mem.toml <<EOF
runtime = "server-beta"
server_url = "http://host.docker.internal:37877"
database_url = "postgres://claudemem:<password>@host.docker.internal:5432/claudemem"
EOF
chmod 600 ~/.config/vibrator/claude-mem.toml
```

`database_url` is optional — leave it out and vibrator skips auto-bootstrap,
expecting you to mint keys yourself. An optional `team_name` key overrides the
default team name (`"vibrators"`).

> **Migrating from the bash-era setup?** The previous implementation read a
> `claude-mem.env` file with `CLAUDE_MEM_*` shell-style keys. The Go rewrite
> uses TOML with unprefixed keys. Convert by renaming the file, swapping
> `KEY=value` for `key = "value"`, and shortening keys:
> `CLAUDE_MEM_RUNTIME` → `runtime`, `CLAUDE_MEM_SERVER_BETA_URL` →
> `server_url`, `CLAUDE_MEM_SERVER_DATABASE_URL` → `database_url`. Delete the
> old `.env` once the new file is in place.

The DSN never enters the vibrator container — only the project-scoped Bearer
token does, minted on first `vibrate` via a one-shot `postgres:16-alpine`
container.

## Verification

```bash
vibrate prereqs status      # probe host stack, dump resolved wiring
```

Should report: `Health probe: OK`, `Server verify: OK`, and a recent event
count if you've already produced any.

## Security model

- **Project-scoped keys**, not team-wide. Each workspace's `.vb` carries a
  key bound to one `project_id`. Server enforces via `ensureProjectAllowed`.
- **No DB credentials in the container.** Only HTTP + Bearer crosses the
  boundary.
- **`.vb` is chmod 600** and gitignored automatically.
