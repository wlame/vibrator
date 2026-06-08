# Extensions

An **extension** is something installed on top of the [harness](harnesses.md) at build time
— an MCP server, a slash-command skill, a subagent, a runtime tool, or a whole bundle like
[ECC](ecc.md). The catalogue is curated and **data-driven**: one Markdown file per item,
with YAML frontmatter, organized per harness under `extensions/<harness>/`. Adding or
updating an extension is a docs-style change, not Go code.

## Browsing the catalogue

```bash
vibrate extensions list                 # counts per harness
vibrate extensions list claude-code     # entries for one harness
vibrate extensions show context7        # full docs for one entry
```

See [`vibrate extensions`](../reference/commands/extensions.md) for the command details.

## Selecting extensions

By ID, comma-separated, scoped to your chosen harness:

```bash
vibrate --harness=claude-code --extensions=context7,ecc-developer
```

Or tick them in the [wizard](../reference/commands/wizard.md) — which pre-checks entries
that are marked default or that you already use on the host (detected via
[`hostprobe`](../reference/commands/hostprobe.md)). Selections persist in
[`.vb`](configuration.md) as `extensions`.

!!! note "Extensions are per-harness"
    An extension ID is resolved as `<harness>/<id>`. Selecting an ID that doesn't exist for
    your harness is an error. Run `vibrate extensions list <harness>` to see what's
    available.

## What an entry looks like

Each entry is Markdown with frontmatter. A simple MCP-server example:

```yaml
---
name: context7
description: Up-to-date library docs MCP — fetches API references on demand
kind: mcp
default: true
size_mb: 1
install: |
  claude mcp add context7 \
    --scope user \
    --transport http \
    https://mcp.context7.com/mcp
source: https://github.com/upstash/context7
---

# context7
...prose describing the extension...
```

### Frontmatter fields

| Field | Meaning |
|-------|---------|
| `name` | display label *(required)* |
| `kind` | `plugin`, `skill`, `mcp`, `subagent`, or `tool` *(required)* |
| `source` | upstream URL *(required)* |
| `description` | short summary shown in the wizard |
| `default` | pre-check in the wizard |
| `size_mb` | approximate image-size impact |
| `category` | one of the catalogue [categories](#categories) |
| `deps.features` | [features](../reference/features.md) the install needs (folded into resolution) |
| `deps.extensions` | other extension IDs this one needs |
| `prereq` | a host-side [prerequisite](../reference/commands/prereqs.md) ID to verify |
| `auth.env` | env var carrying a credential, forwarded from the host |
| `install` | the shell snippet run at build time |
| `host_aliases` | alternate IDs for [hostprobe](../reference/commands/hostprobe.md) matching |
| `runtime_needs` | declares network/host-service needs (e.g. `outbound_net`, `self_hosted`) |

### Kinds

| Kind | What it is |
|------|-----------|
| `plugin` | a complete pluggable package (command + skill + MCP) |
| `skill` | a slash-command-triggered behavior |
| `mcp` | a Model Context Protocol server |
| `subagent` | a dispatchable specialized agent |
| `tool` | a CLI/runtime tool |

### Categories

`code-intelligence`, `memory`, `documentation`, `web-browser`, `version-control`,
`project-management`, `communication`, `cloud-infrastructure`, `databases`, `design-ui`,
`testing`, `security`, `ai-integration`, `dev-tools`, `observability`, `harness-specific`,
`niche`.

## How extensions affect the build and run

- **Build:** each selected entry's `install` snippet runs in
  [Stage 4](../lifecycle/build.md#stage-4-extensions). Its `deps.features` are merged into
  [feature resolution](profiles-and-features.md#how-resolution-works), so a node-dependent
  MCP pulls in the `node` feature automatically.
- **Run:** if an entry declares `auth.env`, that host env var is
  [forwarded](../lifecycle/startup.md#forwarded-environment) into the container so the
  extension can authenticate.
- **Fingerprint:** selected extensions are part of the
  [variant fingerprint](../reference/naming-and-labels.md) — changing them builds a new
  image.

## Catalogue highlights

The catalogue spans all four harnesses and includes, among many others:

- **MCP servers** — `context7`, `github-mcp`, `linear-mcp`, `notion-mcp`, `playwright-mcp`,
  `postgres-mcp`, `sentry-mcp`, `kubernetes-mcp`, `terraform-mcp`, `filesystem-mcp`,
  `sqlite-mcp`, `fetch`, `sequential-thinking`, `chrome-devtools`.
- **Skills / agents** — `docs-writer-skill`, `code-review`, `code-reviewer-agent`,
  `pm-agent`, `superpowers`, `superclaude`.
- **Integration plugins** — `serena`, `claude-mem`, `codex-plugin-cc`, `cc-thingz`.
- **The [ECC bundle](ecc.md)** — the `ecc-*` family.

Run `vibrate extensions list <harness>` for the authoritative, current list.

## Related pages

- [The ECC bundle](ecc.md) — the big opt-in bundle.
- [`vibrate extensions`](../reference/commands/extensions.md) — command reference.
- [Profiles & features](profiles-and-features.md) — how `deps.features` feed resolution.
