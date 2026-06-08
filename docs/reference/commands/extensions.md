# `vibrate extensions`

Browse the curated [extensions catalogue](../../guides/extensions.md) — the per-harness
inventory of MCP servers, skills, subagents, and bundles you can install into an image.

```bash
vibrate extensions <list|show> [args]
```

---

## `vibrate extensions list` { #vibrate-extensions-list }

```bash
vibrate extensions list [HARNESS]
```

With **no argument**, lists each harness and how many extension entries it has. With a
**harness** argument, lists that harness's entries in a table:

```bash
vibrate extensions list claude-code
```

| Column | Meaning |
|--------|---------|
| `ID` | the extension ID you pass to `--extensions` |
| `KIND` | `plugin`, `skill`, `mcp`, `subagent`, or `tool` |
| `DEFAULT` | ✓ if pre-selected in the wizard |
| `SIZE` | approximate image-size impact (MB), when known |
| `NAME` | display label |

---

## `vibrate extensions show` { #vibrate-extensions-show }

```bash
vibrate extensions show ID
```

Prints an entry's full YAML frontmatter and Markdown body — what it is, why it's useful,
its dependencies, auth requirements, and install snippet.

The `ID` can be:

- **Qualified** — `claude-code/context7` (unambiguous).
- **Bare** — `context7` (searched across all harnesses; errors if ambiguous).

### Examples

```bash
# Counts per harness.
vibrate extensions list

# Everything available for Codex.
vibrate extensions list codex

# Full docs for one entry.
vibrate extensions show ecc-developer
vibrate extensions show claude-code/serena
```

## Related

- [Extensions guide](../../guides/extensions.md) — how the catalogue works and how to select
  entries.
- [The ECC bundle](../../guides/ecc.md) — the `ecc-*` family.
- [`vibrate hostprobe`](hostprobe.md) — which of your host plugins map to catalogue entries.
