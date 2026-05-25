---
name: AGENTS.md Template
description: Drops an AGENTS.md scaffold into the workspace
kind: tool
default: true
size_mb: 0
category: harness-specific
install: |
  # Bakes a starter AGENTS.md template into the image at a stable path.
  # The vibrator entrypoint copies it into the workspace on first run if
  # no AGENTS.md exists there yet — never overwriting an existing file.
  mkdir -p /opt/vibrator/templates
  cat > /opt/vibrator/templates/AGENTS.md.tmpl <<'EOF'
  # AGENTS.md

  This file is read by Codex CLI (and other AGENTS.md-aware agents) at
  session start. Use it to declare conventions and constraints that the
  agent should respect inside this repository.

  ## Project context

  - **What this project is**: <one-line description>
  - **Primary language(s)**: <e.g. Go, Python, TypeScript>
  - **Where to start reading**: <e.g. cmd/foo/main.go, src/index.ts>

  ## Build and test

  - Install deps: `<command>`
  - Build: `<command>`
  - Run tests: `<command>`
  - Lint: `<command>`

  The agent should use these commands verbatim. Do not substitute a
  different package manager or test runner.

  ## Style

  - <indentation, line length, quoting rules>
  - <import ordering>
  - <naming conventions>

  ## Do not touch

  - `vendor/`, `node_modules/`, generated code under `gen/`
  - `<other paths>`

  ## PR conventions

  - Branch naming: `<pattern>`
  - Commit message format: `<format>`
  - Required reviewers: `<team handles>`

  ## Subdirectory overrides

  Place a directory-local `AGENTS.md` to override these rules for a
  specific subtree. Place `AGENTS.override.md` next to an `AGENTS.md`
  to override it without editing the original.
  EOF
source: https://agents.md
host_aliases: [agents-md-template]
---

# AGENTS.md Template (tool)

Scaffolds a starter `AGENTS.md` template into the workspace so new
projects pick up the cross-vendor convention with zero typing.

## How it works

The install step bakes the template at
`/opt/vibrator/templates/AGENTS.md.tmpl` inside the image. The vibrator
entrypoint runs a small first-launch hook that copies it to
`<workspace>/AGENTS.md` if and only if that file does not already
exist. Existing AGENTS.md is never touched.

## Using it

From inside a Codex session:

```text
> scaffold AGENTS.md
```

…or, from the shell:

```bash
cp /opt/vibrator/templates/AGENTS.md.tmpl ./AGENTS.md
```

Then edit the placeholders (`<command>`, `<path>`, etc.) to reflect the
project's actual conventions.

## Why on by default

- Zero size, zero runtime cost.
- The single highest-leverage way to make Codex stick to repo
  conventions across long sessions.
- Pairs with the `codex-skills` extension's "read AGENTS.md first"
  reminder.

## Related

- The cross-vendor [agents.md](https://agents.md) standard
- The `codex-skills` extension in this catalogue, which adds an
  always-on reminder to read AGENTS.md before editing
