# `vibrate hostprobe`

```bash
vibrate hostprobe
```

Scans your host for installed harness configs and plugins, and shows how those raw host-side
identifiers map to entries in the [extensions catalogue](../../guides/extensions.md). This
is the same scan the [wizard](wizard.md) uses to **pre-check** extensions you already use on
the host, so it's a useful diagnostic for "why is the wizard suggesting this?"

Takes no flags.

## What it reports

For each harness, `hostprobe` prints:

| Field | Source |
|-------|--------|
| **Home dir** | the harness's config directory (e.g. `~/.claude`) |
| **Installed** | whether that directory exists |
| **Plugins / skills (raw)** | plugin IDs found in the harness's config |
| **MCP servers (raw)** | MCP server names found |
| **Registered marketplaces** | marketplace IDs (Claude Code) |
| **Extension matches** | which catalogue entries those raw IDs map to (these get pre-checked in the wizard) |
| **Notes** | best-effort diagnostics (parse warnings, detection method) |

For Claude Code specifically it reads `~/.claude/plugins/installed_plugins.json` (modern
manifest), `~/.claude/settings.json` (`enabledPlugins`), `~/.claude.json` (`mcpServers`),
and `~/.claude/plugins/known_marketplaces.json`. Parse errors are reported as notes, never
fatal.

## Example

```bash
vibrate hostprobe
```

Use it to confirm an extension you've installed on the host is recognized by Vibrator's
catalogue (and will be offered in the wizard), or to debug a mapping that isn't happening.

## Related pages

- [Extensions guide](../../guides/extensions.md) — the catalogue and `host_aliases`
  matching.
- [`vibrate extensions list`](extensions.md) — the catalogue itself.
- [`vibrate wizard`](wizard.md) — where the matches get pre-checked.
