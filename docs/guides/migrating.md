# Migrating from bash

Earlier Vibrator workspaces used a shell dotenv file, `.vb.env`. The current tool uses a
TOML pin, `.vb`. The [`vibrate migrate-pin`](../reference/commands/migrate-pin.md) command
converts one to the other, losslessly.

## Convert a workspace

```bash
cd ~/my-project

# Preview the conversion — nothing is written.
vibrate migrate-pin --dry-run

# Convert: writes .vb, archives the old file to .vb.env.bak, updates .gitignore.
vibrate migrate-pin
```

By default the original `.vb.env` is archived to `.vb.env.bak`. Pass `--keep-old` to leave
it in place.

## What gets mapped

| Legacy `.vb.env` | New `.vb` |
|------------------|-----------|
| `HARNESS` | `harness` |
| `PROFILE` | `profile` |
| `SHELL` | `shell` |
| `WITH` | `with` (comma-split) |
| `NO` | `no` (comma-split) |
| `CATALOG` | `extensions` (comma-split) |
| `CLAUDE_MEM_SERVER_BETA_API_KEY` etc. | `[prereqs.claude-mem-server-beta]` |
| `USERNAME` | dropped — now a [build-time flag](../reference/commands/index.md#spec-resolution-flags) |
| anything else | preserved verbatim under `[env]` |

!!! tip "Nothing is lost"
    Unrecognized keys are preserved under `[env]`, so custom variables you set in the old
    file survive the conversion. Review the result (or use `--dry-run` first).

## Example

Before — `.vb.env`:

```bash
HARNESS=claude-code
PROFILE=full
WITH=docker-cli
CATALOG=context7,ecc-developer
CLAUDE_MEM_SERVER_BETA_API_KEY=cmem_abc...
OPENAI_API_KEY=sk-...
```

After — `.vb`:

```toml
harness = "claude-code"
profile = "full"
with = ["docker-cli"]
extensions = ["context7", "ecc-developer"]

[prereqs.claude-mem-server-beta]
api_key = "cmem_abc..."

[env]
OPENAI_API_KEY = "sk-..."
```

## After converting

The new `.vb` is everything subsequent runs need:

```bash
vibrate          # resolves from the converted .vb
```

If you want to re-pick anything through the wizard while keeping your credentials, use
[`vibrate reconfigure`](../reference/commands/wizard.md#vibrate-reconfigure).

## Related

- [`vibrate migrate-pin`](../reference/commands/migrate-pin.md) — the command reference.
- [The `.vb` file](configuration.md) — the target format explained.
- [`.vb` schema](../reference/vb-file.md) — every field.
