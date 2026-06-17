# `vibrate` / `run` / `shell`

These three forms share one flag set and one orchestration path — they differ only in what
gets exec'd inside the container at the end.

| Form | Final exec target |
|------|-------------------|
| `vibrate` *(bare)* | the harness's CLI |
| `vibrate run` | the harness's CLI |
| `vibrate shell` | your shell |

All of them: resolve the workspace config (flags > `.vb` > defaults), run the
[wizard](wizard.md) for unset fields, build the image if it's missing, create or start the
container, and exec the target. See [Lifecycle](../../lifecycle/index.md) for the full
decision tree.

---

## `vibrate` (bare) { #vibrate-bare }

```bash
vibrate [flags]
```

The default action. Equivalent to `vibrate run`. Launches the
[harness](../../guides/harnesses.md)'s own CLI — `claude` for claude-code, `codex` for codex,
`opencode` for opencode, `pi` for pi.

---

## `vibrate run` { #vibrate-run }

```bash
vibrate run [flags]
```

The explicit form of the bare invocation. Identical behavior and flags.

---

## `vibrate shell` { #vibrate-shell }

```bash
vibrate shell [flags]
```

Like `vibrate run`, but execs your shell inside the container instead of the harness CLI.
Everything else — build-if-missing, start-if-stopped, workspace mount, credential
forwarding — is identical. Useful for:

- Debugging the container (inspect config files, check installed packages).
- Installing things the [extensions catalogue](../../guides/extensions.md) doesn't cover.
- Driving the harness via subcommands or piped input rather than its interactive TUI.

---

## Flags { #options }

### Spec-resolution flags

Shared with [`build`](build.md). See the [shared table](index.md#spec-resolution-flags):
`--harness`, `--profile`, `--shell`, `--with`, `--no`, `--extensions`, `--username`,
`--host-uid`, `--host-gid`.

### Orchestration flags

| Flag | Default | Description |
|------|---------|-------------|
| `--no-wizard` | `false` | Skip the interactive wizard; fail if a required field (harness) is unset. |
| `--no-save` | `false` | Don't write the resolved config to `.vb`. |
| `--rebuild` | `false` | Remove any existing container and rebuild the image from scratch (`docker build --no-cache`), then run fresh. |
| `--dind` | `false` | Mount the host's Docker socket so `docker` inside the container drives the host daemon. The `docker` client is always in the image, so toggling this never rebuilds — it just recreates the container with (or without) the socket mounted. See [Docker-in-Docker](../../guides/docker-in-docker.md). |
| `--mount=PATH[:rw]` | *(none)* | Mount an extra host folder at the same absolute path inside the container. Read-only unless `:rw` is appended. Repeatable. Saved to `.vb` as `mounts = [...]` and re-applied on later runs; for claude-code the folders are also passed to the agent via `--add-dir`. |
| `--login` | `false` | Run `claude auth login` in the container before launching the harness; opens the auth URL in your host browser and writes auth state back to the host. See [Authentication](../../guides/authentication.md#vibrate-login). |

---

## Examples

```bash
# First run — wizard fills the gaps, then build + launch.
vibrate

# Fully specified, no wizard.
vibrate --no-wizard --harness=claude-code --profile=full --shell=zsh

# Backend profile, add a couple of features, drop one, pick extensions.
vibrate --harness=claude-code --profile=backend \
        --with=node,playwright --no=ralphex \
        --extensions=context7,ecc-developer

# Try a combo once without saving it to .vb.
vibrate --harness=codex --profile=minimal --no-save

# Force a clean rebuild (e.g. after changing host rules baked at build time).
vibrate --rebuild

# Authenticate Claude Code via the browser, then land in the agent.
vibrate --login

# Drop into a shell instead of the agent.
vibrate shell

# Let the container drive the host Docker daemon.
vibrate --dind
```

## Exit code

`vibrate` propagates the exit code of the process you ran inside the container — so exiting
the agent with a non-zero status surfaces it to your host shell.

## Related pages

- [The `.vb` file](../vb-file.md) — what gets saved and resolved.
- [What happens on build](../../lifecycle/build.md) / [on start](../../lifecycle/startup.md).
- [`vibrate reconfigure`](wizard.md#vibrate-reconfigure) — change an existing setup.
