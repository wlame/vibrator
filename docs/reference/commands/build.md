# `build` / `build-dockerfile`

Two commands for producing build artifacts without launching a container. Both share the
[spec-resolution flags](index.md#spec-resolution-flags); unlike the
[launch commands](launch.md), `--profile` defaults to `full` and `--shell` to `zsh` at the
flag level (rather than being resolved later from `.vb`).

---

## `vibrate build` { #vibrate-build }

```bash
vibrate build [flags]
```

Resolves the spec and runs `docker build` with the generated
[Dockerfile](../../lifecycle/build.md). Does **not** start a container. Use it to warm an
image ahead of time, or in CI.

### Flags

In addition to the [spec-resolution flags](index.md#spec-resolution-flags):

| Flag | Default | Description |
|------|---------|-------------|
| `--no-cache` | `false` | Pass `--no-cache` to `docker build` (forces a full rebuild). |
| `--tag=<tag>` | — | Override the image tag. Default is the [fingerprinted tag](../naming-and-labels.md) `vb-<harness>-<profile>-<fingerprint>:latest`. |

### Examples

```bash
# Build the default-spec image for this workspace.
vibrate build --harness=claude-code

# Clean rebuild of a specific spec.
vibrate build --harness=codex --profile=backend --no-cache

# Build under a custom tag for pushing to a registry.
vibrate build --harness=claude-code --tag=myregistry/vibrate-cc:latest
```

---

## `vibrate build-dockerfile` { #vibrate-build-dockerfile }

```bash
vibrate build-dockerfile [flags]
```

Runs the same generation pipeline as `build`, but writes the resulting **Dockerfile** to a
path (or stdout) instead of invoking `docker`. The output is byte-deterministic for a given
spec — ideal for diffing changes, golden tests, and CI inspection.

### Flags

In addition to the [spec-resolution flags](index.md#spec-resolution-flags):

| Flag | Default | Description |
|------|---------|-------------|
| `--out=<path>` | `-` | Output path. Use `-` for stdout. |

### Examples

```bash
# Print the Dockerfile for a full claude-code image to your terminal.
vibrate build-dockerfile --harness=claude-code --profile=full --shell=zsh

# Write it to a file for inspection / diffing.
vibrate build-dockerfile --harness=codex --profile=backend --out=Dockerfile.codex

# See how selecting an extension changes the build.
vibrate build-dockerfile --harness=claude-code --extensions=context7,ecc-developer
```

The generated header records the exact spec and a reproduction command, so a saved
Dockerfile documents itself.

## Related

- [What happens on build](../../lifecycle/build.md) — the five-stage generator explained.
- [Features](../features.md) and [Profiles](../profiles.md) — what populates the stages.
- [Naming & labels](../naming-and-labels.md) — how the default tag is derived.
