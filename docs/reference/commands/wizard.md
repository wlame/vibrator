# `wizard` / `reconfigure`

Two ways to drive the interactive setup form. `wizard` is a preview-only sandbox;
`reconfigure` actually updates an existing workspace and rebuilds it.

---

## `vibrate wizard` { #vibrate-wizard }

```bash
vibrate wizard
```

Runs the [setup wizard](#the-wizard-flow) standalone. **Nothing is written to disk and no
image is built.** It prints a summary of your selections and the equivalent
`vibrate` command. Useful for previewing what a configuration looks like, or for verifying
the wizard's behavior, without committing to anything.

To actually use a configuration, copy the printed equivalent command, or run
[`vibrate`](launch.md) (which runs the wizard inline and *does* save + build).

---

## `vibrate reconfigure` { #vibrate-reconfigure }

```bash
vibrate reconfigure [flags]
```

**Alias:** `reconfig`

Re-runs the wizard for an **existing** workspace and rebuilds the container with the new
selections. Requires an existing `.vb` — it's a modification flow, not first-time setup.

What's **preserved** (carried through untouched):

- `[prereqs.*]` — minted API keys, team/project IDs.
- `[env]` — custom host→container env forwarding.
- `with` / `no` — your per-workspace feature toggles.

What the wizard **re-asks** (and overwrites): harness, profile, shell, extensions, LLM
provider, integration hosting modes.

By default the old container — built for a spec that no longer matches — is removed, and a
fresh one is built (`--rebuild` semantics).

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--keep-container` | `false` | Leave the old container running/stopped instead of removing it (so you can switch back without rebuilding). |
| `--dry-run` | `false` | Walk the wizard and preview the new configuration without saving or rebuilding. |

### Examples

```bash
# Change harness/profile/extensions for this workspace and rebuild.
vibrate reconfigure

# Preview the change without touching anything.
vibrate reconfigure --dry-run

# Switch setups but keep the old container around to flip back to.
vibrate reconfigure --keep-container
```

---

## The wizard flow

Both commands present the same gated form. Each step is shown **only if** the corresponding
field isn't already set (by a flag, the existing `.vb`, or — for `reconfigure` — left
deliberately blank to be re-asked):

| Step | Shown when | Choices |
|------|------------|---------|
| **Harness** | harness unset | `claude-code`, `codex`, `opencode`, `pi` |
| **Profile** | profile unset | `minimal`, `backend`, `frontend`, `full` |
| **Shell** | shell unset | `bash`, `zsh`, `fish` |
| **LLM provider** | LLM unset *and* harness supports it | `openai`, `anthropic`, `ollama`, `lmstudio`, `openai-compat` |
| **Extensions** | always | multi-select from the harness's [catalogue](../../guides/extensions.md) |
| **Serena hosting** | not pinned *and* harness is claude-code | `auto`, `host`, `local`, `off` |

The extensions step pre-checks entries that are either marked default or
[detected on your host](hostprobe.md). The LLM step branches into a cloud path (model +
auth) or a local path (URL + model picker) — see [LLM providers](../../guides/llm-providers.md).

On completion the wizard prints a summary and the **equivalent command** — the exact flags
that reproduce the selection, so you can skip the wizard next time.

## Related

- [Quick start](../../getting-started/quickstart.md) — the wizard in context.
- [The `.vb` file](../vb-file.md) — what the wizard writes.
- [LLM providers](../../guides/llm-providers.md) — the provider selection step in detail.
