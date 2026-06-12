# Troubleshooting

Concrete fixes for the failures you're most likely to hit, grouped by symptom. Each entry
covers **what you see**, **why it happens**, and **the fix**.

!!! tip "Turn on diagnostics first"
    Set `VIBRATOR_VERBOSE=1` to print `[vibrator] ...` diagnostics from the entrypoint and
    the [`claude-exec` wrapper](lifecycle/startup.md#3-the-claude-exec-wrapper). Most
    "why did it do that?" questions answer themselves once verbose output is on. Silence
    the launch banner with `VIBRATOR_NO_BANNER=1`.

## No Docker runtime found

**What you see.** `vibrate` exits before building anything, complaining it can't find a
Docker socket. [`vibrate runtime detect`](reference/commands/runtime.md) exits non-zero and
reports no reachable socket.

**Why it happens.** Vibrator talks to Docker by shelling out to the `docker` CLI, so it
needs a reachable Unix socket. If no runtime is running — and none of the override
mechanisms point at a live socket — [detection](lifecycle/runtime-detection.md) fails.

**Fix.** Start a runtime, or tell Vibrator where the socket is:

```bash
# 1. Start a runtime (Docker Desktop, OrbStack, Colima, Rancher Desktop, …) and recheck:
vibrate runtime detect

# 2. Or point Vibrator at a specific socket, one-off:
vibrate runtime detect --docker-socket=/custom/path/docker.sock

# 3. Or persist it for the session:
export VIBRATOR_DOCKER_SOCKET=/custom/path/docker.sock
```

`$DOCKER_HOST` (when it's a `unix://` socket) is also honored. See
[Runtime detection](lifecycle/runtime-detection.md) for the full resolution order.

## Docker-in-Docker fails on Colima or Rancher Desktop

**What you see.** Everything works until you add [`--dind`](guides/docker-in-docker.md);
then `docker` commands *inside* the container can't reach the daemon, specifically on Colima
or Rancher Desktop.

**Why it happens.** These are VM-based runtimes. The macOS-side socket is a **proxy** that
only listens on the host and is unreachable from inside the VM (and therefore from inside a
container). Vibrator works around this by mounting the daemon's own
`/var/run/docker.sock` — which the daemon, running inside the VM, resolves to its real
socket — with a sudo wrapper baked into the base image handling group access.

**Fix.** If docker-in-docker still fails on these runtimes, this proxy/socket mismatch is
almost always the cause. Confirm the runtime and socket Vibrator is using, then retry:

```bash
vibrate runtime detect            # confirm runtime = colima / rancher-desktop
vibrate runtime detect --colima-profile=work   # if you use a non-default Colima profile
```

!!! warning "`--dind` grants host-root-equivalent access"
    Mounting the Docker socket into a container lets the agent control the host daemon —
    effectively root on the host. Only use `--dind` for trusted workspaces.

## Serena host server unreachable

**What you see.** With `[integrations] serena = "host"` in [`.vb`](guides/configuration.md), a pre-launch check
**warns** that the Serena server is unreachable. In `auto` mode you instead notice a brief
delay each session (a `uvx` cold-start).

**Why it happens.** In `host` mode Vibrator expects a persistent host server bound on
`0.0.0.0:8765`; if it isn't bound there, the [pre-launch check](integrations/serena.md#pre-launch-check)
warns and — because there's no fallback in `host` mode — Serena is simply absent. In `auto`
mode there's no host server to reach, so `claude-exec` spawns Serena locally via `uvx`,
paying a cold-start on each new session.

**Fix.** Start the host server and make sure it binds the right interface:

```bash
vibrate integrations serena       # start the host server (process or docker runtime)
```

If you run it yourself, bind `0.0.0.0` (not `127.0.0.1`) on port `8765`:

```bash
uvx --from git+https://github.com/oraios/serena \
    serena start-mcp-server --transport=streamable-http \
    --host=0.0.0.0 --port=8765
```

Override the port with `SERENA_PORT` on the host before starting both the server and
Vibrator. A failing check **never blocks** the launch — see [Serena](integrations/serena.md).

## claude-mem token rejected

**What you see.** On launch, the claude-mem entrypoint auth-probe fails. A `401`/`403`
means the token was rejected; a `400`/`422` means the server is fine but the token is bad.
A *connection refused* means the server isn't running (and is skipped silently).

**Why it happens.** Only a bearer token crosses into the container — the Postgres DSN stays
host-only. If that token is stale or wrong, the probe rejects it. If the server isn't up at
all, there's nothing to probe.

**Fix.** Re-mint the token:

```bash
vibrate prereqs bootstrap claude-mem-server-beta --force
```

For *connection refused*, start the claude-mem server first (it's skipped silently when
down, so the agent still launches). If your DSN uses `localhost`, note it's auto-rewritten
to `host.docker.internal` for you. See [claude-mem](integrations/claude-mem.md).

## A hook references a tool that isn't installed

**What you see.** A hook in your `~/.claude/settings.json` shells out to a tool — `node`,
`python`, etc. — that isn't in the resolved [feature set](reference/features.md). You may
see `node: not found` (or similar) from a hook.

**Why it happens.** A hook assumes a tool that the chosen profile/features don't include —
common with the `minimal` profile. Vibrator has a two-layer defense: at **launch** it
prompts to add the missing feature (and rebuild); at **runtime** the entrypoint guard runs
`command -v` and silences any hook whose tool isn't on `PATH`.

**Fix.** Take the launch-time prompt to add the feature, or add it yourself and rebuild:

```bash
vibrate --with=node --rebuild
```

To keep a hook intentionally tool-less, acknowledge it under `[hooks] acknowledged_missing`
in `.vb`. See [Missing-tool hooks](lifecycle/startup.md#missing-tool-hooks).

## `--no-wizard` fails on a required field

**What you see.** With `--no-wizard`, Vibrator errors out that a required field (commonly
the harness) is unset, instead of prompting.

**Why it happens.** `--no-wizard` makes missing required fields a hard error rather than a
prompt. The harness — and any other required field — must come from flags or an existing
`.vb`.

**Fix.** Supply the required flags, or drop `--no-wizard` to let the
[wizard](reference/commands/wizard.md) fill the gaps:

```bash
vibrate --no-wizard --harness=claude-code --profile=full --shell=zsh
```

## Stale image or container after a config change

**What you see.** You changed `.vb` (or passed new flags) but the container still behaves as
before — old tools, old extensions.

**Why it happens.** Vibrator reuses an existing container when the resolved spec matches.
Some changes don't force a fresh build on their own, so the old image/container lingers.

**Fix.** Force a rebuild, or re-run the wizard and rebuild:

```bash
vibrate --rebuild        # force a fresh build from the Dockerfile
vibrate reconfigure      # re-run the wizard, then rebuild (preserves credentials + [env])
```

To clean up afterwards, remove stopped containers and unused images:

```bash
vibrate variants prune
```

!!! tip "Profile changes alone won't change the image"
    Profile is excluded from the [variant fingerprint](reference/naming-and-labels.md) —
    only the *resolved feature set* is hashed. `--profile=full` and the implicit default
    build the same image. If you expected a new image from a profile swap that resolves to
    the same features, that's why.

## First build is very slow (claude-code)

**What you see.** The first `vibrate` run for a Claude Code workspace takes a long time to
build.

**Why it happens.** The default profile is `full` (~2 GB) — everything except aider. That's
a lot to install on the first build.

**Fix.** Pick a smaller profile for a faster first build:

```bash
vibrate --profile=minimal      # ~150 MB, base toolkit only
vibrate --profile=backend      # ~600 MB, python/go/gh/postgres-client/ralphex
```

Subsequent runs reuse the image and are instant. See [Profiles](reference/profiles.md).

## Diagnostic commands

When you don't have a specific error, these commands tell you what Vibrator sees:

| Command | Purpose |
|---------|---------|
| [`vibrate runtime detect`](reference/commands/runtime.md) | Show the detected Docker runtime, socket path, and how it was found. |
| [`vibrate hostprobe`](reference/commands/hostprobe.md) | Scan the host for installed harness plugins/extensions Vibrator can reuse. |
| [`vibrate prereqs status`](reference/commands/prereqs.md) | Report the state of host-side prerequisites (e.g. claude-mem) for this workspace. |
| [`vibrate variants list`](reference/commands/variants.md) | List the images and containers Vibrator has built, with their fingerprints. |

## Still stuck?

[Open an issue](https://github.com/wlame/vibrator/issues) and include the output of
`vibrate runtime detect`, the relevant `VIBRATOR_VERBOSE=1` output, your `.vb` (redact any
credentials), and the steps to reproduce.

## Related pages

- [FAQ](faq.md) — quick answers to common questions.
- [Runtime detection](lifecycle/runtime-detection.md) — how the Docker socket is resolved.
- [Integrations](integrations/index.md) — hosting modes, readiness checks, and transport
  fallback for Serena and claude-mem.
- [Docker-in-Docker](guides/docker-in-docker.md) — `--dind`, socket mounting, and its risks.
