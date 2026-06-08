# `vibrate migrate-pin`

```bash
vibrate migrate-pin [flags]
```

Converts a bash-era `.vb.env` (shell dotenv) into the current `.vb` (TOML) pin format. See
the [migration guide](../../guides/migrating.md) for the narrative version.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--input=<path>` | `./.vb.env` | Path to the legacy `.vb.env`. |
| `--output=<path>` | `.vb` beside the input | Path to write the new `.vb`. |
| `--dry-run` | `false` | Print the conversion without touching disk. |
| `--keep-old` | `false` | Keep `.vb.env` instead of archiving it to `.vb.env.bak`. |

## What it maps

| Legacy key | New location |
|------------|--------------|
| `HARNESS` / `PROFILE` / `SHELL` | top-level `harness` / `profile` / `shell` |
| `WITH` / `NO` / `CATALOG` | `with` / `no` / `extensions` (comma-split lists) |
| `CLAUDE_MEM_SERVER_BETA_*` | `[prereqs.claude-mem-server-beta]` |
| `USERNAME` | dropped (now a build-time flag) |
| anything else | preserved verbatim under `[env]` |

Unknown keys are never lost — they land under `[env]`.

## Examples

```bash
# Preview the conversion.
vibrate migrate-pin --dry-run

# Convert: writes .vb, archives the old file to .vb.env.bak, updates .gitignore.
vibrate migrate-pin

# Convert a file elsewhere, keeping the original.
vibrate migrate-pin --input=old/.vb.env --output=.vb --keep-old
```

## Related

- [Migrating from bash](../../guides/migrating.md) — the full guide.
- [The `.vb` file](../vb-file.md) — the target format.
