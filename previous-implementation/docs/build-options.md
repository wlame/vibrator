# Build options

Vibrator builds a Docker image with a configurable set of features. The
defaults install everything except `aider` — the same as historical behaviour
— so existing users see no change. Below is what each knob does and how
they interact.

---

## Quick choice: pick a profile

```bash
vibrate --profile minimal       # ~150MB
vibrate --profile backend       # ~600MB
vibrate                         # default profile, ~2GB
vibrate --profile kitchen-sink  # ~2.1GB (default + aider)
```

You can then fine-tune with `--with-FEATURE` and `--no-FEATURE`:

```bash
# Backend preset but I also want Playwright
vibrate --profile backend --with-playwright

# Default but drop the heavy audit toolkit
vibrate --no-audit-toolkit

# Minimal but add Python + GitHub CLI
vibrate --profile minimal --with-python --with-gh

# Preview what will be installed without actually building
vibrate --profile backend --with-playwright --explain
```

`--explain` prints the resolved feature set and exits — no image
is built. Use it when you're unsure what your flag combo will produce.

---

## Always-on substrate (not toggleable)

Some pieces are installed in every image. They're small, load-bearing for
the entrypoint, and several features assume their presence. They can't be
disabled:

| What | Why it's always in |
|---|---|
| `bash`, `sh`, `zsh` | Shell, entrypoint, welcome message |
| `jq` | Settings/plugin-registration parsing in entrypoint |
| `curl`, `wget` | Health probes, downloads |
| `git`, `gpg`, `ssh-client` | Source control, signing, agent forwarding |
| `sudo`, `vim`, `tree`, `sqlite3` | Basic interactive ergonomics |
| `uv`, `bun`, `node` (22) | Always-on JS/runtime substrate (Stage 1). `uv` provisions Python in Stage 2; `bun` is the runtime claude-mem's `bun-runner.js` re-execs into; `node` 22 is required by claude-mem's npm installer (needs `node:util.styleText`, Node 20+) and used by `npx`-installed MCP servers. All three are pulled via multi-stage `COPY --from=…` from their official images. |

Everything else is one of the toggleable features below.

---

## Feature catalog

| Feature | Default | Size | What gets installed | Implicit deps |
|---|---|---|---|---|
| **`playwright`** | on | ~500 MB | Chromium binary, `playwright-core` (npm), chromium runtime apt deps, Playwright MCP wired in entrypoint | — |
| **`audit-toolkit`** | on | ~400 MB | `trivy`, `syft`, `grype`, `semgrep`, `gitleaks`, `trufflehog`, `osv-scanner`, `scc`, `lizard`, `bandit`, `checkov`, `dockle`, `shellcheck`, `hadolint` | `python` |
| **`python`** | on | ~100 MB | Python 3.13 via `uv` | — |
| **`go`** | on | ~200 MB | Go toolchain | — |
| **`gh`** | on | ~20 MB | GitHub CLI | — |
| **`dev-cli`** | on | ~65 MB | `jq` extras, `yq`, `fzf`, `fd`, `ripgrep`, `tree`, `httpie`, `websocat`, `csvkit`, `delta`, `lazygit`, `glow` (markdown viewer) | — |
| **`serena`** | on | trivial | Serena MCP runtime detection block in entrypoint; uvx-based stdio fallback | `python` |
| **`claude-mem`** | on | trivial | claude-mem plugin marketplace bind-mount, env-var forwarding, plugin auto-registration in entrypoint. See [claude-mem integration guide](./integrations/claude-mem.md). | — |
| **`codex`** | on | ~30 MB | OpenAI Codex CLI (used by the `/planning:exec` skill for adversarial review) | — |
| **`aider`** | **off** | ~80 MB | `aider-chat` via `uv tool install` | `python` |

---

## Profile presets in detail

### `minimal` (~150 MB)

The smallest usable image. Just the always-on substrate plus `dev-cli`
(`jq`, `yq`, `ripgrep`, etc.) for general scripting. No language toolchains,
no Playwright, no audit tools, no integrations.

Useful for: CI runs, tight resource environments, "I just want Claude CLI
in a box."

```bash
vibrate --profile minimal
# enabled: dev-cli
# disabled: everything else
```

### `backend` (~600 MB)

Drops the two heaviest items — `playwright` (~500 MB) and `audit-toolkit`
(~400 MB) — but keeps language runtimes (`python`, `go`), `gh`, integrations
(`serena`, `claude-mem`, `codex`), and `dev-cli`.

Useful for: backend/API/CLI development where you don't render web pages and
don't run a security audit on every session.

```bash
vibrate --profile backend
# enabled: python, go, gh, dev-cli, serena, claude-mem, codex
# disabled: playwright, audit-toolkit, aider
```

### `default` (~2 GB) — what `vibrate` runs today

Everything except `aider`. Identical to historical behaviour, kept as the
default so existing users see no change.

```bash
vibrate                         # implicit
vibrate --profile default       # explicit
# enabled: everything in the catalog except aider
```

### `kitchen-sink` (~2.1 GB)

Everything in the catalog including `aider`. Use when you want every tool
you might possibly need without having to remember flags.

```bash
vibrate --profile kitchen-sink
# enabled: everything in the catalog
```

---

## Dependency resolution

Some features need others to function. Vibrator handles this for you:

> **Rule:** if you enable a feature whose dependencies are off, vibrator
> **auto-enables the missing deps** and logs a warning. Your build still
> succeeds; you're just told what was force-enabled.

The dependency map:

| Feature | Requires |
|---|---|
| `audit-toolkit` | `python` (for `bandit`, `checkov`, `pip-audit`) |
| `serena` | `python` (for `uvx`) |
| `aider` | `python` (for `uv tool install`) |

So `python` is the only "hub" today. If any of the four above are on,
`python` is also on.

### Examples

```bash
# Inconsistent flags — vibrator fixes it
vibrate --no-python --with-serena
# WARN: Feature 'serena' requires 'python' — auto-enabling.
#       To avoid, also pass --no-serena.

# To truly skip Python, drop everything that needs it
vibrate --no-python --no-aider --no-serena --no-audit-toolkit
# (Or just: --profile minimal --with-go --with-gh — same result.)
```

You can also pre-flight any combination with `--explain`:

```bash
vibrate --profile minimal --with-serena --explain
# Profile: minimal
# Features:
#   playwright       false
#   audit-toolkit    false
#   python           true       ← auto-enabled
#   go               false
#   gh               false
#   dev-cli          true
#   serena           true
#   claude-mem       false
#   codex            false
#   aider            false
```

---

## Worked scenarios

### "I write Go backend services and don't touch frontend"

```bash
vibrate --profile backend
```

You get: Go toolchain, Python (for audit-Python tools you might invoke
ad-hoc), Codex review, Serena, claude-mem, dev-cli. You skip Chromium
(~500 MB) and the audit toolkit (~400 MB).

### "Same, but I want the audit toolkit on hand"

```bash
vibrate --profile backend --with-audit-toolkit
```

Adds the trivy/syft/grype/etc. binaries.

### "I work on web apps and need Playwright but not Go"

```bash
vibrate --no-go
```

Default profile minus `go`. Saves ~200 MB.

### "Single-purpose CI image that runs only `claude --print '…'`"

```bash
vibrate --profile minimal --with-claude-mem --rm
```

Plus your `~/.config/vibrator/claude-mem.env` if you want memory
persistence across CI runs (see the [claude-mem guide](./integrations/claude-mem.md)).

### "Everything, plus aider"

```bash
vibrate --profile kitchen-sink
```

---

## Image size at a glance

These are rough numbers — exact size depends on apt mirror state and
which CLI tools have updated since the base image was built.

| Profile | Size | Build time (cold cache) |
|---|---|---|
| `minimal` | ~150 MB | ~2 min |
| `backend` | ~600 MB | ~4 min |
| `default` | ~2 GB | ~10 min |
| `kitchen-sink` | ~2.1 GB | ~10 min |

After first build, the layer cache makes subsequent builds with the same
profile near-instant. Switching profiles forces a rebuild of the affected
layers.

---

## Multiple image variants in parallel

Vibrator builds a distinct Docker image per distinct feature set. The image
tag encodes profile + an 8-char content hash so different variants never
collide on a shared tag:

```
claude-vb-<user>-<profile>-<fingerprint>:latest
                  └─────┴── e.g. backend-c1d2e3f4
```

Examples after running several `vibrate` invocations with different flags:

```
$ docker images claude-vb-wlame-*
REPOSITORY                                  TAG       SIZE
claude-vb-wlame-default-a4f9c2b1            latest    2.0GB
claude-vb-wlame-backend-c1d2e3f4            latest    600MB
claude-vb-wlame-backend-9bd71028            latest    1.1GB    # backend + playwright
claude-vb-wlame-minimal-7d3e5f8a            latest    150MB
```

The container name encodes the same fingerprint plus the workspace, so:

| Scenario | Result |
|---|---|
| `cd ~/proj-a && vibrate` (default), then again same flags | Reuses container `claude-vb-proj-a-<wsHash>-a4f9c2b1` |
| `cd ~/proj-a && vibrate --profile minimal` | New container `claude-vb-proj-a-<wsHash>-7d3e5f8a` — old default container untouched |
| `cd ~/proj-b && vibrate --profile minimal` | Reuses the minimal **image** but creates a workspace-scoped container `claude-vb-proj-b-<wsHash>-7d3e5f8a` |

So you can switch profiles in the same workspace without losing the old
container's state. Stale containers can be removed manually:

```bash
# List all your vibrator containers (running + stopped)
docker ps -a --filter "label=vibrator.managed=true"

# Remove one by name
docker rm claude-vb-proj-a-<wsHash>-a4f9c2b1

# Remove all stopped vibrator containers
docker container prune --filter "label=vibrator.managed=true"
```

Same applies to unused images:

```bash
docker images claude-vb-${USER}-*    # list variants
docker rmi claude-vb-${USER}-default-a4f9c2b1   # remove one
```

A `vibrate --list-variants` / `--prune-variants` helper is planned.

---

## First run in a fresh workspace: the interactive menu

When you run `vibrate` for the first time in a folder that has **no
`.vb.env`** (walking up to the git root) **and no existing container**, you
get an interactive picker:

```
[vibrator] /home/wlame/work/new-proj is a fresh workspace and no .vb.env was found.

Use an existing variant on this machine:
   1) backend         c1d2e3f4   612MB     built 2 days ago
   2) default         a4f9c2b1   2.0GB     built 7 days ago
   3) minimal         7d3e5f8a   148MB     built 4 days ago

Build a new variant:
   4) minimal       (~150 MB)
   5) backend       (~600 MB)
   6) default       (~2 GB)        [default if you press Enter]
   7) kitchen-sink  (~2.1 GB)

Other:
   q) Quit (re-run with explicit --profile / --with-* / --no-* flags)

Choice [6]:
```

If you pick **build new**, you get a second checkbox screen to toggle
individual features on top of the chosen profile:

```
Customize features for profile 'backend':
(type a number to toggle, or:  d) done   r) reset   q) quit)

  [ ]  1) playwright       (~500 MB)
  [ ]  2) audit-toolkit    (needs: python)
  [x]  3) python
  [x]  4) go
  [x]  5) gh
  [x]  6) dev-cli
  [x]  7) serena           (needs: python)
  [x]  8) claude-mem
  [x]  9) codex
  [ ] 10) aider            (needs: python)

  7 of 10 enabled

Toggle [d=done]:
```

Then you're asked whether to record the choice into `.vb.env` so the
next `vibrate` in this workspace skips the menu entirely.

### When the menu does NOT fire (intentional)

| Condition | Why |
|---|---|
| stdin is not a terminal | Don't break scripts / CI |
| Any of `--profile`, `--with-*`, `--no-*`, `--simple`, `--aider` was passed | You already said what you want |
| `.vb.env` exists (`$PWD` or anywhere up to git root) | Project pin wins |
| A container for this workspace already exists | Re-entry takes the existing one |
| `--no-menu` or `VIBRATOR_NO_MENU=1` | Explicit opt-out |
| `--build`, `--explain`, `--pull`, `--export-dockerfile` | Non-interactive modes |

If you want to never see the menu, add `export VIBRATOR_NO_MENU=1` to your
shell rc.

---

## Project-pinned config: `.vb.env`

To make a project always build with the same variant, drop a `.vb.env`
file in the workspace root:

```dotenv
# .vb.env — committed for team sharing, or gitignored for personal use
PROFILE=backend
WITH="playwright audit-toolkit"
NO="aider"
```

Recognized keys:
- `PROFILE` — one of `minimal`, `backend`, `default`, `kitchen-sink`
- `WITH` — space-separated feature names to enable on top of the profile
- `NO` — space-separated feature names to disable

Vibrator's startup order (highest precedence wins):

1. CLI flags (`--profile`, `--with-X`, `--no-X`)
2. `./.vb.env` (workspace pin)
3. Default profile

So `cd ~/proj-a && vibrate` automatically uses proj-a's pinned variant; a
one-off `vibrate --no-aider` still works because CLI overrides the pin.

The file is parsed manually (not `source`'d), so a malicious project file
can't execute arbitrary code in your host shell. Unknown keys are ignored
with a verbose-mode log; unknown feature names in `WITH`/`NO` cause an
error.

### Pin file examples

A frontend project that needs Playwright but no Go:

```dotenv
PROFILE=default
NO="go"
```

A small CI image with just the bits needed to run `claude --print`:

```dotenv
PROFILE=minimal
WITH="claude-mem"
```

A backend service with audit toolkit always on hand:

```dotenv
PROFILE=backend
WITH="audit-toolkit"
```

To see what your pin (combined with any CLI overrides) resolves to:

```bash
vibrate --explain
# Profile: backend
# Features:
#   playwright       false
#   audit-toolkit    true    ← added by WITH
#   python           true
#   ...
```

---

## Inside the container: what's enabled?

The chosen profile and feature set are baked into the image as environment
variables at build time:

```bash
echo $VIBRATOR_PROFILE              # e.g. backend
echo $VIBRATOR_FEATURES_LIST        # e.g. python go gh dev-cli serena claude-mem codex
echo $VIBRATOR_VARIANT_FINGERPRINT  # e.g. c1d2e3f4
```

The welcome message reads these on shell startup and lists what's
available as a comma-separated `Features:` line.

In-container scripts can gate behavior on a feature:

```bash
# Crude but POSIX
case " $VIBRATOR_FEATURES_LIST " in
  *" playwright "*)  echo "Playwright is available" ;;
esac
```

Docker LABELs mirror the same info so `docker inspect <image>` shows it
without entering the container:

```bash
docker inspect claude-vb-wlame-backend-c1d2e3f4:latest \
  --format '{{json .Config.Labels}}' | jq
# {
#   "vibrator.profile": "backend",
#   "vibrator.features": "python go gh dev-cli serena claude-mem codex",
#   "vibrator.variant": "c1d2e3f4"
# }
```

A richer `/opt/vibrator/features.json` (with versions, timestamps, and a
typed feature map) is planned for a follow-up — useful when in-container
tooling wants more than just "is feature X enabled".
