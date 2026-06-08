# `vibrate update`

```bash
vibrate update
```

Upgrades the **harness's own CLI** to its latest version, in place — without regenerating
the whole image from the Dockerfile. The harness, profile, and rest of the spec come from
the workspace [`.vb`](../vb-file.md). Takes no flags.

## Decision tree

| State | What happens | Persistence |
|-------|--------------|-------------|
| Container **running** | Exec the harness's update command inside it (e.g. `claude update`). | Survives restart, lost on container removal. |
| Container **stopped** (exited/etc.) | Start the container, then run the update. | Same as above. |
| Container **missing**, image exists | Add a `RUN <update-cmd>` layer on top of the image and re-tag it with the same name. The old image becomes dangling; the layer cache keeps it fast. | Baked into the image. |
| **Neither** exists | Error — run [`vibrate`](launch.md) first to bootstrap the workspace. | — |

!!! tip "Promoting a container update into the image"
    A container-level update survives restarts but not container removal. To bake it into
    the image instead, remove the container first (e.g.
    [`vibrate variants prune --containers`](variants.md)) and run `vibrate update` again —
    it then takes the image-layer path.

## Per-harness update command

| Harness | Command run |
|---------|-------------|
| `claude-code` | `claude update` |
| `codex` | `npm install -g @openai/codex@latest` |
| `opencode` | `opencode upgrade` |
| `pi` | `npm install -g @mariozechner/pi-coding-agent@latest` |

## When to use a full rebuild instead

`vibrate update` only touches the harness binary. To rebuild everything from the
Dockerfile — new base packages, features, extensions, re-fetched bundles — use
[`vibrate --rebuild`](launch.md#options) instead.

## Related

- [`vibrate variants`](variants.md) — inspect/clean up images and containers.
- [Harnesses](../../guides/harnesses.md) — what each harness installs.
