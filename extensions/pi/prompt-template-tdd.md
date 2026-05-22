---
name: TDD prompt template
kind: skill
default: false
size_mb: 0
category: testing
install: |
  # Drops a TDD-focused prompt template under Pi's prompts directory.
  # Pi reads from ~/.pi/agent/prompts/ and turns each file into a slash
  # command — so this becomes /tdd <args>.
  mkdir -p ~/.pi/agent/prompts
  cat > ~/.pi/agent/prompts/tdd.md <<'TEMPLATE'
  ---
  description: Test-driven development workflow — RED → GREEN → REFACTOR
  argument-hint: "<feature description>"
  ---
  # TDD workflow for: $@

  Implement "$@" using strict test-driven development:

  ## 1. RED — write failing tests first

  - Identify the smallest meaningful behavioural slice
  - Write a test that captures the desired behaviour
  - Run the test and confirm it FAILS (compile error counts as failure,
    but only if the failure is for the right reason)
  - Show the failing output before continuing

  ## 2. GREEN — minimal code to pass

  - Write the smallest possible implementation that makes the test pass
  - Resist over-engineering — solve only the immediate failure
  - Run the test suite — confirm GREEN
  - If a test you didn't intend to break now fails, STOP and reconsider

  ## 3. REFACTOR — clean up

  - With tests green, improve naming, extract helpers, remove dupes
  - After each change run the full test suite
  - Refactor scope is limited to what the current test exercises

  ## 4. COMMIT

  - One commit per RED-GREEN-REFACTOR cycle
  - Conventional commit format: feat / fix / refactor / test
  - The commit message describes the BEHAVIOUR, not the mechanics

  ## Rules

  - NEVER write production code without a failing test
  - NEVER write more test code than needed to fail
  - NEVER write more production code than needed to pass
  - If you find yourself wanting to skip a step, the step is the point

  Start with step 1 — the first failing test for "$@".
  TEMPLATE
source: https://github.com/fgladisch/pi-skills
---

# TDD prompt template

Drops a TDD-focused prompt template at `~/.pi/agent/prompts/tdd.md`.
Pi turns every file in its `prompts/` directory into a slash command,
so this becomes:

```
/tdd implement a rate limiter that allows N requests per minute per IP
```

The template enforces a strict RED → GREEN → REFACTOR cycle with a
prompt that's explicit about what counts as "passing" and what counts
as "moving on too fast".

## Why a template, not a skill

Two formats serve different purposes:

- **Skills** (e.g. `fgladisch/pi-skills/test-driven-development`) get
  auto-injected into the system prompt; the model decides when to use
  them.
- **Prompt templates** (this) get invoked explicitly with `/tdd <args>`
  and front-load the rules so the model can't "skip the RED phase
  because it seems obvious".

Use the skill when you want TDD-shaped behaviour subtly woven into a
session. Use this template when you want to start a session in a
locked-in TDD mode and you don't trust the model to remember.

## Pair with

- `fgladisch/pi-skills/verification-before-completion` — runs a check
  after the agent claims a feature is done
- `pi-hooks/lsp` — fails the turn loudly if tests don't compile
- `pi-rewind-hook` — undo the cycle if you decide to redesign

Default off; enable when shipping a TDD-first workspace.
