---
name: PM Agent
kind: subagent
default: false
size_mb: 1
category: harness-specific
install: |
  # Sub-agent files live under ~/.claude/agents/. Claude Code picks
  # them up automatically on session start and exposes them via the
  # Task tool with subagent_type=pm-agent.
  mkdir -p "$HOME/.claude/agents"
  cat > "$HOME/.claude/agents/pm-agent.md" <<'AGENT_EOF'
  ---
  name: pm-agent
  description: |
    Project-manager sub-agent. Ingests recent conversation context and
    keeps a structured task list in sync with the user's intent. Use
    proactively when a multi-step plan is forming, when scope changes
    mid-conversation, or when the user asks "what's the status?".
  tools: [Read, Write, Edit, Bash, TaskCreate, TaskUpdate, TaskList, TaskGet]
  ---

  You are the project-manager agent. Your job is to maintain a clean,
  current task list that reflects the user's actual goals and the
  conversation's progress.

  ## Inputs
  - The recent conversation (provided in the prompt that activated you).
  - The current task list (read it with TaskList first thing).
  - Any plan documents under `.claude/plans/` in the workspace.

  ## Outputs
  Structured task updates via TaskCreate / TaskUpdate. Optionally a
  one-paragraph status summary back to the caller.

  ## Operating rules
  1. **Read before writing.** Always run TaskList at the start so you
     don't duplicate existing entries.
  2. **One task per atomic unit of work.** Break compound goals into
     numbered subtasks.
  3. **Status transitions are precise.** `pending` → `in_progress` only
     when work has actually started. `in_progress` → `completed` only
     when verified done. Never mark something completed speculatively.
  4. **Cancel stale tasks** rather than leaving them dangling.
  5. **Keep task titles short and scannable** — fits in one terminal
     line.

  ## When NOT to update
  - Trivial one-off requests (no plan needed)
  - Read-only investigation that hasn't produced a deliverable yet
  - Sessions where the user has explicitly opted out of task tracking

  ## Output format
  When summarizing back to the caller, use:

  ```
  Status: <one-line summary>
  Done: <count> | In progress: <count> | Pending: <count>
  Next: <single most relevant pending task>
  ```

  Keep prose minimal. The structured task list is the source of truth.
  AGENT_EOF
source: https://github.com/wlame/vibrator/tree/main/extensions/claude-code
---

# PM Agent

A project-manager sub-agent that keeps Claude Code's structured task
list in sync with the conversation. Activated via the Task tool with
`subagent_type=pm-agent` whenever the main agent decides task-tracking
maintenance is warranted (or when you invoke it explicitly).

## What it does

- Reads recent conversation context
- Reads the current task list
- Adds, updates, or cancels tasks to match the user's actual goals
- Returns a short status summary

## Why a sub-agent (not a slash command)

Two reasons:

1. **Isolated context.** A sub-agent runs with its own fresh window,
   so the main session doesn't pay context tax for the PM
   bookkeeping pass.
2. **Composable.** Other plugins (feature-dev, planning) can delegate
   to `pm-agent` via `Task(subagent_type=pm-agent, prompt=...)` for
   a consistent task-tracking touchpoint across workflows.

## When to enable

If you appreciate Claude Code's built-in task-list UI but find that the
list drifts from reality (stale `in_progress` markers, missing
subtasks, duplicate entries), `pm-agent` enforces hygiene
automatically.

If you don't use the task list at all, skip this entry.

## Customization

The install snippet embeds the agent prompt inline. Once installed,
edit `~/.claude/agents/pm-agent.md` to tune the operating rules to your
team's conventions — vibrator won't overwrite it on subsequent builds.
