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
| `VIBRATOR_CLAUDE_MEM_CONFIG` | Override the [claude-mem](../integrations/claude-mem.md) admin config path. | — |
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
| `[env]` overrides | [`.vb` `[env]`](vb-file.md) — literal or `$NAME` indirection |
| `VIBRATOR_INTEGRATION_MODE_<ID>` | per-integration [hosting mode](../guides/integrations.md#hosting-modes) |
| `CLAUDE_MEM_RUNTIME`, `CLAUDE_MEM_SERVER_BETA_URL`, `CLAUDE_MEM_SERVER_BETA_API_KEY`, `CLAUDE_MEM_SERVER_BETA_TEAM_ID`, `CLAUDE_MEM_SERVER_BETA_PROJECT_ID` | [claude-mem](../integrations/claude-mem.md) when bootstrapped |

!!! note "Integration mode var naming"
    `VIBRATOR_INTEGRATION_MODE_<ID>` upper-cases the integration ID and replaces
    non-alphanumerics with `_` — so `claude-mem` becomes
    `VIBRATOR_INTEGRATION_MODE_CLAUDE_MEM`.

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

## Related

- [Authentication](../guides/authentication.md) — credential forwarding and precedence.
- [What happens on start](../lifecycle/startup.md) — when each var is set.
- [Runtime detection](../lifecycle/runtime-detection.md) — the Docker socket vars.
