---
name: Plan agent (built-in)
kind: subagent
default: true
size_mb: 0
install: |
  true
source: https://opencode.ai/docs/agents/
---

# Plan agent (built-in)

OpenCode's restricted primary agent — denies most edit tools. Useful for
analysis and architectural planning sessions where you want the model to
think out loud without touching files.

Activate during a session with Tab (cycles through primary agents) or
`@plan` mention.

## Tool restrictions

Plan has read-only access by default:

- ✓ read, grep, glob, web search, MCP read-only tools
- ✗ write, edit, multi-edit, bash (mostly)

## Customizing

Same `.opencode/agents/plan.md` override pattern as the build agent.
Useful when you want to allow some specific edits in plan mode (e.g.,
"plan mode but `git checkout` is fine").

Default = on (same logic as build-agent — it ships with opencode).
