# Environment variables

A reference for every environment variable Vibrator reads on the host, forwards into the
container, or bakes into images.

## Read by Vibrator on the host

These influence how `vibrate` behaves on your machine.

| Variable | Used for | Default |
|----------|----------|---------|
| `VIBRATOR_DOCKER_SOCKET` | Force the Docker socket path (highest priority). | — |
| `DOCKER_HOST` | Docker socket when it's a `unix://` URL. | — |
| `COLIMA_PROFILE` | Colima VM profile to probe. | `default` |
| `SERENA_PORT` | Port the [Serena](../integrations/serena.md) integration uses. | `8765` |
| `VIBRATOR_CLAUDE_MEM_CONFIG` | Override the [claude-mem](../integrations/claude-mem.md) admin config path (`~/.config/vibrator/claude-mem.toml`). | — |
| `VIBRATOR_INTEGRATIONS_DIR` | Override the directory scanned for user-defined integration descriptors. | — |
| `XDG_CONFIG_HOME` | Base for config dirs (e.g. `claude-mem.toml`). | `~/.config` |
| `XDG_DATA_HOME` | Base for data dirs (Serena PID/log). | `~/.local/share` |

See [Runtime detection](../lifecycle/runtime-detection.md) for the socket-related vars.

## Forwarded into the container

Set at `docker run` (see [What happens on start](../lifecycle/startup.md#forwarded-environment)).
Listed by source, in precedence order (later wins on name collision).

| Variable(s) | Source |
|-------------|--------|
| `WORKSPACE_PATH` | always — the workspace absolute path |
| Harness auth vars (`CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, …) | forwarded from host when set — see [Authentication](../guides/authentication.md) |
| LLM-derived vars (`OPENAI_API_KEY`, `OPENAI_BASE_URL`, provider keys, …) | computed from [`[llm]`](vb-file.md#llm) |
| Extension `auth.env` vars | declared by selected [extensions](../guides/extensions.md) |
| `VIBRATOR_IDENTITY_NAME`, `VIBRATOR_IDENTITY_EMAIL` | Set by vibrate from the pin's `[identity]` table; the entrypoint uses them to override git identity and rewrite the Claude account email inside the container. |
| `[env]` overrides | [`.vb` `[env]`](vb-file.md) — literal or `$NAME` indirection |
| `VIBRATOR_INTEGRATION_MODE_<ID>` | per-integration [hosting mode](../guides/integrations.md#hosting-modes) |
| `CLAUDE_MEM_RUNTIME`, `CLAUDE_MEM_SERVER_BETA_URL`, `CLAUDE_MEM_SERVER_BETA_API_KEY`, `CLAUDE_MEM_SERVER_BETA_TEAM_ID`, `CLAUDE_MEM_SERVER_BETA_PROJECT_ID` | [claude-mem](../integrations/claude-mem.md) when bootstrapped |

!!! note "Integration mode var naming"
    `VIBRATOR_INTEGRATION_MODE_<ID>` upper-cases the integration ID and replaces
    non-alphanumerics with `_` — so `claude-mem` becomes
    `VIBRATOR_INTEGRATION_MODE_CLAUDE_MEM`.

!!! warning "The claude-mem DSN never crosses the boundary"
    The Postgres `database_url` (DSN) from the claude-mem admin config is **host-only**. Only
    the `CLAUDE_MEM_SERVER_BETA_*` token/IDs above are forwarded into the container — the DSN
    itself is never set as a container env var.

### How forwarded values reach the container, and their residual exposure

`vibrate` never puts secret values on the `docker run`/`docker exec` command line. It passes
only the variable *name* via `-e NAME` (the docker CLI then resolves it from its own process
environment) and separately writes `NAME=VALUE` pairs to a private, mode-`0600` `--env-file`
temp file that's cleaned up after the call. Neither form is visible via `ps` or
`/proc/*/cmdline` while the command runs.

That said, two residual exposures are inherent and worth knowing about:

- **`docker inspect` shows container env regardless of how it was passed.** Once a container
  is running, `docker inspect <container> --format '{{.Config.Env}}'` (or the `Config.Env`
  field in the full JSON) lists every env var baked into it, including secrets forwarded at
  `docker run` time. This is true for *any* container on the host, not a vibrator-specific
  gap — env vars are not a confidentiality mechanism once a container exists. Anyone able to
  run `docker inspect` against your containers can already reach your host process list, so
  this doesn't introduce a new privilege boundary, but it's worth remembering if you share
  Docker access with others. Avoiding it entirely would require file-mounted secrets instead
  of env vars, which vibrator doesn't currently do (tracked as future work, not implemented).
- **The claude-mem bootstrap's psql DSN has a narrow argv fallback.** The one-shot postgres
  container that [claude-mem](../integrations/claude-mem.md) bootstrapping uses normally
  receives the connection details as `PGHOST`/`PGPORT`/`PGUSER`/`PGPASSWORD`/`PGDATABASE` env
  vars, not as a positional DSN argument. Two DSN shapes fall back to the legacy positional
  argument instead: the rare `key=value` conninfo form (which isn't a URL `net/url` can parse), and any
  DSN carrying query parameters (e.g. `?sslmode=require&connect_timeout=10`) — dropping those
  silently via the env-var decomposition would be a connection-security downgrade, so the
  simpler and safer choice is to keep the full DSN as a single positional argument in those two
  cases. The argv-visibility window described above only applies to those two fallback shapes.

## Baked into the image

Set in the Dockerfile [runtime stage](../lifecycle/build.md#stage-5-runtime); readable
inside the container.

| Variable | Value |
|----------|-------|
| `VIBRATOR_HARNESS` | harness ID |
| `VIBRATOR_PROFILE` | profile ID |
| `VIBRATOR_FEATURES_LIST` | comma-separated resolved features |
| `VIBRATOR_EXTENSIONS_LIST` | comma-separated extensions (or `(none)`) |
| `VIBRATOR_VERSION` | vibrator version (or `dev`) |
| `VIBRATOR_BUILD_ID` | build sentinel (also at `/etc/vibrator/build`) |
| `VIBRATOR_LAUNCH_BIN` | the harness's launch binary (e.g. `claude`) — used by the shell rc to build the YOLO alias |
| `VIBRATOR_YOLO_ARGS` | the harness's permission-bypass args (default), e.g. `--dangerously-skip-permissions`. Overridden at launch to empty by [`--no-yolo`](commands/launch.md); an empty value suppresses the alias |
| `LANG`, `LC_ALL` | `en_US.UTF-8` |
| `COLORTERM` | `truecolor` |
| `PATH`, `NPM_CONFIG_PREFIX`, `UV_TOOL_BIN_DIR`, `UV_PYTHON_INSTALL_DIR` | install/lookup paths |

## Read by in-container scripts

These control the [entrypoint](../lifecycle/startup.md#2-the-entrypoint-entrypointsh) and
[`claude-exec`](../lifecycle/startup.md#3-the-claude-exec-wrapper) wrapper behavior.

| Variable | Effect |
|----------|--------|
| `VIBRATOR_VERBOSE=1` | Print `[vibrator] ...` diagnostics for each setup step. |
| `VIBRATOR_NO_BANNER=1` | Suppress the welcome banner. |

!!! tip "Debugging startup"
    Set `VIBRATOR_VERBOSE=1` before launching to see exactly which env vars and setup steps
    the entrypoint runs — handy when an integration or forwarded credential isn't behaving.

## Related pages

- [Authentication](../guides/authentication.md) — credential forwarding and precedence.
- [What happens on start](../lifecycle/startup.md) — when each var is set.
- [Runtime detection](../lifecycle/runtime-detection.md) — the Docker socket vars.
