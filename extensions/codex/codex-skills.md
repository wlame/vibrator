---
name: AGENTS.md Best Practices
kind: skill
default: true
size_mb: 0
category: harness-specific
install: |
  # Drops a SKILL.md under ~/.codex/skills/ so Codex auto-discovers it.
  # The skill is a short, opinionated reminder the agent reads at session
  # start in any repo that has an AGENTS.md.
  mkdir -p "$HOME/.codex/skills/agents-md-best-practices"
  cat > "$HOME/.codex/skills/agents-md-best-practices/SKILL.md" <<'EOF'
  ---
  name: agents-md-best-practices
  description: |
    Reminds the agent to read AGENTS.md (and AGENTS.override.md) at the
    repo root and any subdirectory you enter, and to honour the
    conventions declared there.
  trigger: implicit
  ---

  # AGENTS.md best practices

  Before editing files in this repository:

  1. Read `AGENTS.md` at the git root. If it doesn't exist, check parent
     directories up to the workspace root.
  2. Read any `AGENTS.override.md` next to an `AGENTS.md` — overrides win.
  3. Walk into the subdirectory you intend to edit. Read any
     directory-local `AGENTS.md` before touching files there.

  ## Honour what you find

  - **Code style** — match the existing patterns (indentation, naming,
    quoting). Don't introduce a new dialect.
  - **Build/test commands** — use the ones declared in AGENTS.md
    verbatim. If the file says "always run `make test`", do not
    substitute `pytest`.
  - **Sandbox boundaries** — paths called out as "do not touch" or
    "human review required" are exactly that.
  - **PR conventions** — commit message format, branch naming,
    reviewer assignments.

  ## When AGENTS.md is missing

  Either the repo hasn't adopted the convention, or this is a fresh
  workspace. Prompt the user before introducing one — many teams have
  preferences you can't intuit.

  ## When AGENTS.md contradicts your instinct

  AGENTS.md wins, every time, unless the user explicitly overrides it
  during the session.
  EOF
source: https://agents.md
host_aliases: [agents-md-best-practices]
---

# AGENTS.md Best Practices (skill)

Self-contained skill that gets dropped into `~/.codex/skills/` at image
bake. No upstream — the content lives in the install block.

## What it does

When Codex sees an `AGENTS.md` in the workspace, this skill nudges the
agent to:

- Read AGENTS.md (and `.override.md`) at root and at any subdirectory
  it enters
- Match the conventions declared there (style, build, test, PR rules)
- Treat AGENTS.md as the authoritative project memory unless the user
  explicitly says otherwise

## Why it's on by default

Codex already injects AGENTS.md content as a user message at session
start, but reminders embedded in a skill compound well with deep
sessions where context gets compacted. Zero install cost (a 1 KB file)
and zero runtime cost (it's text).

## Related

- The OpenAI [agents.md](https://agents.md) standard
- The `agents-md-template` extension in this catalogue, which scaffolds
  a starter AGENTS.md into the workspace
