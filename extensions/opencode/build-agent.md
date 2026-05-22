---
name: Build agent (built-in)
kind: subagent
default: true
size_mb: 0
category: harness-specific
install: |
  # Built-in primary agent — no install step required. Configurable via
  # ~/.config/opencode/agent/build.md (or .opencode/agent/build.md in the
  # workspace), but the default behavior ships with the opencode binary.
  true
source: https://opencode.ai/docs/agents/
---

# Build agent (built-in)

OpenCode's default primary agent for development work. Full tool access:
`read`, `write`, `edit`, `bash`, `glob`, `grep`, web search, plus any
MCP tools you've enabled. This is the agent you talk to by default when
you launch `opencode`.

Switch agents during a session with the Tab key, or `@build` to invoke
explicitly inside a message.

## When to customize

Drop a `~/.config/opencode/agent/build.md` (global) or
`.opencode/agent/build.md` (per-project) to override the system prompt,
restrict tools, or change the default model. Format is Markdown with
YAML frontmatter:

```yaml
---
description: My customized build agent
mode: primary
model: anthropic/claude-opus-4-6
temperature: 0.2
tools:
  write: true
  edit: true
  bash: false   # disable bash for safer interactive sessions
---

Custom system prompt body...
```

## Why on by default

It's literally the default agent — disabling it would break opencode's
operability. The extension exists so the wizard can show it as a
"baked-in" item rather than appearing as an empty harness.
