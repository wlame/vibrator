---
name: Build agent (built-in)
kind: subagent
default: true
size_mb: 0
install: |
  # Built-in primary agent — no install step required. Configurable via
  # .opencode/agents/build.md in the workspace, but the default behavior
  # ships with the opencode binary.
  true
source: https://opencode.ai/docs/agents/
---

# Build agent (built-in)

OpenCode's default primary agent for development work. Full tool access:
read, write, edit, bash, web search, MCP tools. The agent you talk to by
default when you launch `opencode`.

Switch agents during a session with the Tab key, or `@build` to invoke
explicitly.

## When to customize

Drop a `.opencode/agents/build.md` in your workspace to override the
system prompt, restrict tools, or change the default model. Format is
Markdown with YAML frontmatter:

```yaml
---
description: My customized build agent
tools:
  read: true
  write: true
  bash: false   # disable bash for safer interactive sessions
model: anthropic/claude-3.5-sonnet
---

Custom system prompt body...
```

## Why on by default

It's literally the default agent — disabling it would break opencode's
operability. The catalog entry exists so the wizard can show it as a
"baked-in" item rather than appearing as an empty harness.
