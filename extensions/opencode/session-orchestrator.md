---
name: OpenCode Workspace (session orchestrator)
kind: plugin
default: false
size_mb: 5
category: harness-specific
deps:
  features: [node]
install: |
  mkdir -p "$HOME/.config/opencode"
  jq '.plugin = ((.plugin // []) + ["opencode-workspace"] | unique)' \
     "$HOME/.config/opencode/config.json" 2>/dev/null \
     > "$HOME/.config/opencode/config.json.tmp" \
     && mv "$HOME/.config/opencode/config.json.tmp" "$HOME/.config/opencode/config.json" \
     || echo '{"$schema":"https://opencode.ai/config.json","plugin":["opencode-workspace"]}' > "$HOME/.config/opencode/config.json"
source: https://github.com/kdcokenny/opencode-workspace
---

# OpenCode Workspace (session orchestrator)

kdcokenny's bundled multi-agent orchestration harness for OpenCode —
ships **16 components** as a single install: 4 plugins (task
delegation, planning, notifications, git worktrees), pre-tuned
sub-agents, slash commands, and the glue that lets them coordinate.

The pitch: turn an OpenCode session into a small autonomous workspace
where the build agent can delegate sub-tasks (research, refactor,
test) to specialized agents running in their own contexts, then
reconcile the results back to the main thread.

## Why multi-agent?

Solo sessions hit a context wall on large tasks. Splitting work
across sub-sessions (each with its own narrow context) often beats
extending the main session's window. Workspace standardizes that
pattern.

## Alternatives

There are several competing approaches in the OpenCode ecosystem:

- **opencode-orchestrator** (agnusdei1207) — hub-and-spoke with
  work-stealing queues.
- **subtask2** (`@openspoon/subtask2`) — slash-command-based
  orchestration.
- **opencode-conductor** (NocturnLabs) — Context-Driven Development
  lifecycle: Context → Spec → Plan → Implement.
- **micode** (vtemian) — Brainstorm → Plan → Implement with
  AST-aware tools and git worktree isolation.

Workspace is the most batteries-included; the others are leaner if
you want to compose your own flow.

## Why off by default

Heavyweight install, opinionated workflow. Enable for long-running or
multi-phase projects.
