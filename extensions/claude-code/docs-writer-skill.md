---
name: Docs Writer Skill
kind: skill
default: false
size_mb: 1
category: documentation
install: |
  # Skills live under ~/.claude/skills/<name>/SKILL.md. Claude Code
  # activates them by description match — invoking with
  # /docs-writer or by intent ("write me a README for this package").
  mkdir -p "$HOME/.claude/skills/docs-writer"
  cat > "$HOME/.claude/skills/docs-writer/SKILL.md" <<'SKILL_EOF'
  ---
  name: docs-writer
  description: |
    Generates README, CHANGELOG, and API-reference documentation from
    code. Use when the user asks to write or update docs, add a
    README, generate API docs, or compile release notes from commits.
  ---

  You are a technical documentation writer. Generate clear, accurate,
  user-focused docs from source code — not generic templates.

  ## Modes of operation

  ### README mode
  Triggered when: user asks for a README, getting-started, or project
  overview.

  Structure:
  1. **Tagline** — one sentence, what the project does and for whom.
  2. **Why it exists** — one paragraph, the problem it solves. Avoid
     "X is a library for Y" framings.
  3. **Install** — copy-pasteable, in the project's package manager
     idiom (read the lockfile / manifest before assuming).
  4. **Quick start** — minimal working example, in real code, not
     pseudo-code.
  5. **Common workflows** — 2-4 representative recipes.
  6. **Configuration / options** — only if non-trivial.
  7. **Links** — docs, issues, contributing.

  Don't pad with sections that don't apply. A 60-line README that's
  accurate beats a 400-line README full of boilerplate.

  ### CHANGELOG mode
  Triggered when: user asks for release notes, changelog, or "what
  changed since version X".

  Source: `git log` between tags / refs.

  Format: Keep a Changelog (https://keepachangelog.com/) style —
  Added / Changed / Deprecated / Removed / Fixed / Security headings.
  Group commits by intent, not chronologically. Drop trivia (typo
  fixes, dependency bumps unless security-relevant) into a "Misc"
  trailer.

  Date format: `YYYY-MM-DD` from the tag commit, not "today".

  ### API reference mode
  Triggered when: user asks for API docs, function reference, or
  module documentation.

  For each public symbol:
  - One-line summary
  - Signature (real, from the source — don't reformat or rename)
  - Parameters (name, type, semantics)
  - Return value (type, semantics)
  - Throws / errors (if applicable)
  - Example, copy-pasteable
  - Cross-links to related symbols

  Order: depth-first by module structure, alphabetical within a module.

  ## Operating rules

  - **Read the code first, then write.** Never invent function names
    or signatures.
  - **Use the project's existing conventions.** If there's already a
    README, match its tone. If there's a docs/ directory, write into
    that style.
  - **Avoid AI tells.** No "In conclusion", no "It's worth noting", no
    "Let's dive in". No emoji unless the project already uses them.
  - **Concrete over abstract.** "Returns the user's email" beats
    "Returns the relevant string identifier".
  - **Verify examples compile.** If you can run them inside the
    session, do.
  SKILL_EOF
source: https://github.com/wlame/vibrator/tree/main/extensions/claude-code
---

# Docs Writer Skill

A skill for generating documentation from code — READMEs, CHANGELOGs,
and API references. Activates either by slash invocation
(`/docs-writer`) or by intent match when the user asks for docs in
the natural-language sense.

## What it produces

- **READMEs** — tagline, problem-statement, copy-pasteable install +
  quick-start, common workflows. Not the bloated template you usually
  get when you ask an LLM for a README.
- **CHANGELOGs** — Keep-a-Changelog format, grouped by intent (Added /
  Changed / Fixed / Security), date-stamped from the actual git tag.
- **API references** — per-symbol entries with real signatures pulled
  from source, parameter docs, return semantics, examples.

## Why a skill (not a plugin)

Single concern, no marketplace dependency. Fits the skill model
exactly: a focused prompt that activates by description match and
disappears when it's not needed.

## When to enable

If you write a lot of project documentation and want consistent
output. If your project already has a strong docs culture
(established style guide, existing patterns), use this skill as
scaffolding and edit the result — don't expect first-pass perfection
on a complex API.

## Customization

The skill prompt embeds at `~/.claude/skills/docs-writer/SKILL.md`.
Fork it to your house style — common edits:

- Replace the README structure with your org's required sections
- Add framework-specific cross-link patterns (`@see Module#foo`)
- Pin the CHANGELOG style to semver or to your release-notes template
