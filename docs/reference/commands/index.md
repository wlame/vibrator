# CLI reference

`vibrate` (alias `vb`) is built with [Cobra](https://github.com/spf13/cobra). Every
subcommand lives on its own page below, with all of its flags, defaults, and behavior.

```
vibrate [command] [flags]
```

Running `vibrate` with **no subcommand** behaves exactly like
[`vibrate run`](launch.md). Global flags `--help` / `-h` and `--version` are available
everywhere.

## All commands

| Command | Purpose | Reference |
|---------|---------|-----------|
| `vibrate` *(bare)* | Resolve `.vb` + flags, run the wizard, build/run/exec | [launch.md](launch.md) |
| `vibrate run` | Explicit form of the bare invocation | [launch.md](launch.md#vibrate-run) |
| `vibrate shell` | Same as `run`, but launch your shell instead of the agent | [launch.md](launch.md#vibrate-shell) |
| `vibrate build` | Build the image without running a container | [build.md](build.md) |
| `vibrate build-dockerfile` | Emit the generated Dockerfile to a file / stdout | [build.md](build.md#vibrate-build-dockerfile) |
| `vibrate wizard` | Run the setup wizard standalone (no build, no save) | [wizard.md](wizard.md) |
| `vibrate reconfigure` | Re-run the wizard and rebuild, preserving credentials | [wizard.md](wizard.md#vibrate-reconfigure) |
| `vibrate update` | Update the harness's CLI in place (no full rebuild) | [update.md](update.md) |
| `vibrate variants list` | List managed images + containers for this host | [variants.md](variants.md#vibrate-variants-list) |
| `vibrate variants prune` | Remove stopped containers / unused images | [variants.md](variants.md#vibrate-variants-prune) |
| `vibrate extensions list` | List extension entries for a harness | [extensions.md](extensions.md#vibrate-extensions-list) |
| `vibrate extensions show` | Show an extension entry's frontmatter + body | [extensions.md](extensions.md#vibrate-extensions-show) |
| `vibrate integrations` | Interactive setup for host-side integrations | [integrations.md](integrations.md) |
| `vibrate prereqs status` | Probe host prerequisites (e.g. claude-mem server) | [prereqs.md](prereqs.md#vibrate-prereqs-status) |
| `vibrate prereqs bootstrap` | Run host-side setup (e.g. mint a claude-mem key) | [prereqs.md](prereqs.md#vibrate-prereqs-bootstrap) |
| `vibrate hostprobe` | Show host-detected plugins + extension matches | [hostprobe.md](hostprobe.md) |
| `vibrate runtime detect` | Show the auto-detected Docker socket | [runtime.md](runtime.md) |
| `vibrate migrate-pin` | Convert a bash-era `.vb.env` → TOML `.vb` | [migrate-pin.md](migrate-pin.md) |

## Spec-resolution flags

These flags shape *what* gets built. They are shared by `vibrate`, `run`, `shell`,
`build`, and `build-dockerfile`. The same values can be pinned in
[`.vb`](../vb-file.md); flags always override the pin.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--harness=<id>` | string | — (required) | `claude-code`, `codex`, `opencode`, `pi` |
| `--profile=<id>` | string | `full` | `minimal`, `backend`, `frontend`, `full` |
| `--shell=<id>` | string | `zsh` | `bash`, `zsh`, `fish` |
| `--with=<feat,...>` | list | — | [Features](../features.md) to enable on top of the profile |
| `--no=<feat,...>` | list | — | Features to disable |
| `--extensions=<id,...>` | list | — | [Extension](../../guides/extensions.md) IDs to install |
| `--username=<name>` | string | host user | Unprivileged user created in the container |
| `--host-uid=<n>` | int | host UID | UID baked into the image |
| `--host-gid=<n>` | int | host GID | GID baked into the image |

!!! tip "Resolution order"
    For every field: **CLI flag → `.vb` value → built-in default**. The full decision tree
    is documented in [Lifecycle](../../lifecycle/index.md).
