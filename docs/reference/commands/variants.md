# `vibrate variants`

Manage the set of locally-built Vibrator images and their containers. Vibrator finds them
by the `vibrator.managed=true` (containers) and `vibrator.harness` (images)
[labels](../naming-and-labels.md), so it only ever touches its own objects.

```bash
vibrate variants <list|prune> [flags]
```

---

## `vibrate variants list` { #vibrate-variants-list }

```bash
vibrate variants list
```

Prints every Vibrator-managed image and container with its metadata, read from the labels
baked at [build](../../lifecycle/build.md) and set at [run](../../lifecycle/startup.md)
time.

```
Images (2 managed)
------------------------------
  vb-claude-code-full-you-a1b2c3d4:latest
    harness:  claude-code
    profile:  full
    features: python,go,node,playwright,gh,postgres-client,audit-toolkit,codex-cli,ralphex
    extensions:  context7,ecc-developer
    size:     2.1GB, built 2 days ago

Containers (1 managed)
------------------------------
  vb-my-project-7f8e9a0b-a1b2c3d4  Up 3 minutes
    image:     vb-claude-code-full-you-a1b2c3d4:latest
    workspace: /Users/you/my-project
```

Running containers are highlighted; the `workspace:` line is read from the
`vibrator.path` label.

---

## `vibrate variants prune` { #vibrate-variants-prune }

```bash
vibrate variants prune [flags]
```

Removes Vibrator-managed objects. **By default it removes stopped containers and unused
(removable) images** — running containers are skipped unless you pass `--force`.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--containers` | `false` | Remove containers only (skip images). |
| `--images` | `false` | Remove images only (skip containers). |
| `--force` | `false` | Force-remove running containers (kill + `rm`). |

With neither `--containers` nor `--images`, both are pruned. In-use images (those a
container still references) are skipped automatically.

### Examples

```bash
# Remove stopped containers and unused images.
vibrate variants prune

# Only clean up containers, including running ones.
vibrate variants prune --containers --force

# Only remove images.
vibrate variants prune --images
```

## Related

- [Naming & labels](../naming-and-labels.md) — the labels these commands filter on.
- [`vibrate update`](update.md) — note that image-layer updates leave a dangling old image
  that `prune` will collect.
