# The `.vb` file

`.vb` is a per-workspace **pin** file in TOML. It records the choices you made so future
`vibrate` runs in that workspace skip the wizard and resolve to the same image. It's
auto-managed — the wizard writes it — but it's plain text you can read and edit by hand.

For the exhaustive field-by-field schema, see the [`.vb` reference](../reference/vb-file.md).
This page is the practical guide.

## What it captures

```toml
# vibrator workspace pin (`.vb`) — auto-managed by `vibrate`.
harness = "claude-code"
profile = "full"
shell   = "zsh"
with    = ["docker-cli"]
no      = ["aider"]
extensions = ["context7", "ecc-developer"]

[integrations]
serena = "auto"
```

| Section | Holds |
|---------|-------|
| `harness` / `profile` / `shell` | the core selection ([harness](harnesses.md), [profile](../reference/profiles.md)) |
| `with` / `no` | [feature](../reference/features.md) deltas on the profile |
| `extensions` | selected [extension](extensions.md) IDs |
| `[llm]` | [LLM provider](llm-providers.md) choice (provider-agnostic harnesses only) |
| `[prereqs.*]` | minted credentials (e.g. claude-mem keys) |
| `[env]` | host→container env forwarding |
| `[integrations]` | per-integration [hosting modes](integrations.md#hosting-modes) |

## How it's found

`vibrate` walks **up** from your current directory looking for `.vb`, stopping at:

- the **git root**, if you're inside a repo, or
- the **filesystem root** otherwise.

The first `.vb` found wins, and its directory becomes the **workspace root**. So a `.vb` at
your repo root applies to every subdirectory of the repo.

## How values are resolved

For each field the precedence is **CLI flag → `.vb` value → built-in default**:

```bash
# .vb says profile = "full", but this run uses backend:
vibrate --profile=backend
```

Flags are a per-invocation overlay; they don't rewrite `.vb` unless the wizard runs and
saves. To try a combination once without persisting it, add `--no-save`.

## Security and gitignore

`.vb` is written with mode `0600` because it may contain **plaintext credentials** (minted
prereq keys, or a pasted LLM API key). When you first save a pin, Vibrator idempotently
appends `.vb` to your `.gitignore` (only if one already exists — it won't create one).

!!! warning "Keep `.vb` out of version control"
    Treat `.vb` like a dotenv file. If your repo has no `.gitignore`, add one with a `.vb`
    line yourself before committing.

## Editing by hand

You can edit `.vb` directly — it's just TOML. Common edits:

```toml
# Forward a host env var into the container.
[env]
OPENAI_API_KEY = "$OPENAI_API_KEY"   # $NAME is resolved from the host at run time
MY_FLAG        = "literal-value"     # literals pass through as-is

# Pin an integration to host-only mode.
[integrations]
serena = "host"
```

After editing, the next `vibrate` picks the changes up. Note that build-affecting fields
(`harness`, `profile`, `shell`, `with`, `no`, `extensions`) change the
[variant fingerprint](../reference/naming-and-labels.md), so they map to a different
image/container — `vibrate` will build the new one.

## Changing setup safely

To re-pick harness/profile/extensions through the wizard while **preserving** your
`[prereqs.*]` and `[env]`, use [`vibrate reconfigure`](../reference/commands/wizard.md#vibrate-reconfigure)
rather than editing by hand.

## Related pages

- [`.vb` schema reference](../reference/vb-file.md) — every field and its type.
- [Migrating from bash](migrating.md) — converting a legacy `.vb.env`.
- [The wizard](../reference/commands/wizard.md) — what writes `.vb`.
