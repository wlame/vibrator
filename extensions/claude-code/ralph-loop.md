---
name: Ralph Loop
kind: plugin
default: false
size_mb: 2
category: harness-specific
install: |
  # ralph-loop is in the official Anthropic marketplace (added late
  # 2025). Two-step install — register the marketplace if it isn't
  # already, then install the plugin from it.
  claude plugin marketplace add anthropics/claude-plugins-official 2>/dev/null || true
  claude plugin install ralph-loop@claude-plugins-official
source: https://github.com/frankbria/ralph-claude-code
---

# Ralph Loop

Frank Bria's "iterative self-referential AI development loop" plugin.
Named after Ralph Wiggum — the agent keeps trying the same task over
and over, seeing prior attempts each time, until either the task is
done or an exit condition trips.

The loop runs **inside** the session (not as an external bash wrapper
calling `claude` in a loop), which is safer: stays under the active
permission model, doesn't spawn detached processes, no rogue tool calls
after you close the laptop.

## Commands

After install:

- `/ralph-loop:ralph-loop` — start a loop in the current session with
  a task brief
- `/ralph-loop:cancel-ralph` — abort an active loop
- `/ralph-loop:help` — explains the exit-detection heuristics

## When to use it

Long-running iterative grinds: massive refactors, "fix every TS error",
"add tests for every untested file", multi-step migrations. Anywhere
the work is conceptually one task but requires dozens of edit-test
cycles to finish.

## Why opt-in (and experimental)

Loops are fundamentally riskier than single-pass tool calls — if exit
detection misfires, you could burn a non-trivial amount of context /
API budget before noticing. Read the README's safety section before
turning Ralph loose on anything that touches money or production
state.

The exit-detection logic is the secret sauce. It checks for
quiescence (no new tool calls), completion claims in the chat, and
explicit stop signals from skill rules. None of those are perfect —
review the loop's progress occasionally.
