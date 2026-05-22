---
name: pi-subagents
kind: subagent
default: true
size_mb: 3
category: harness-specific
deps:
  features: [node]
install: |
  # pi-subagents by tintinweb — the most full-featured of the five
  # competing Pi subagent extensions. Claude-Code-style hierarchical
  # subagents with parallel execution and live widgets.
  pi install npm:@tintinweb/pi-subagents
source: https://github.com/tintinweb/pi-subagents
---

# pi-subagents

Pi has no built-in subagent / task tool. `tintinweb/pi-subagents` is the
most Claude-Code-compatible community extension, and the one vibrator
defaults to.

## Capabilities

- **Parallel execution** (default concurrency 4)
- **Live widget** showing per-agent progress
- **Custom agent types** via `.pi/agents/*.md` (YAML frontmatter +
  markdown body — same shape as Anthropic's agent spec)
- **Mid-run steering** via the `steer_subagent` tool
- **Session resume** + **graceful turn limits**
- **Context inheritance forking** (child gets parent snapshot)
- **Persistent agent memory** scoped per project / local / user
- **Git worktree isolation** per subagent
- **Skill preloading** + **tool denylists per agent**
- **Cron / interval scheduling**
- **Fuzzy model selection** (per-agent `model:` frontmatter)

## Built-in agent types

| Type              | Tools                    | Model              | Purpose                  |
|-------------------|--------------------------|--------------------|--------------------------|
| `general-purpose` | All (parent twin)        | Parent             | Anything                 |
| `Explore`         | Read-only                | Haiku              | Cheap exploration        |
| `Plan`            | Read-only                | Parent             | Architecture planning    |

## Why this one (vs the alternatives)

Pi has at least five competing subagent extensions
(`mjakl/pi-subagent`, `baochunli/pi-collaborating-agents`,
`minghinmatthewlam/pi-subagents`, `@narumitw/pi-subagents`,
`richardgill/sub-pi`). `tintinweb` wins for vibrator because:

1. Closest to the Claude Code Task-tool mental model
2. Largest feature surface (worktrees, scheduling, denylists)
3. Active maintenance and largest user base
4. `.pi/agents/*.md` format is a near-drop-in for `.claude/agents/*.md`

Default on. If you prefer peer-to-peer agents that DM each other, swap
for `baochunli/pi-collaborating-agents`.
