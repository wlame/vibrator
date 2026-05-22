---
name: CodeRabbit Review
kind: plugin
default: false
size_mb: 25
category: testing
deps:
  features: [node]
auth:
  env: CODERABBIT_API_KEY
runtime_needs:
  third_party_api: "CodeRabbit"
  outbound_net: true
install: |
  # The CodeRabbit plugin requires the coderabbit CLI on PATH first —
  # the plugin shells out to it for the actual review pass. The
  # official install script writes to /usr/local/bin so it survives
  # the container's user switch.
  curl -fsSL https://cli.coderabbit.ai/install.sh | sh
  # Refresh the marketplace cache so the plugin is discoverable, then
  # install it from the Anthropic official marketplace. The cache
  # update is the step most one-liner guides skip — without it the
  # plugin name resolves to "not found".
  claude plugin marketplace add anthropics/claude-plugins-official 2>/dev/null || true
  claude plugin marketplace update 2>/dev/null || true
  claude plugin install coderabbit@claude-plugins-official
source: https://github.com/coderabbitai/claude-plugin
---

# CodeRabbit Review

External code-review validation via CodeRabbit's hosted analyzer.
Registers a `/coderabbit:review` slash command (and an autonomously
triggered review skill) that runs the working diff through 40+
analyzers — security patterns, dead code, style consistency,
performance smells — then surfaces structured findings in the chat.

## How it differs from Anthropic's `code-review`

Anthropic's `code-review` plugin is an LLM-only review pass. CodeRabbit
is **LLM + static analyzers**: it actually runs SAST tooling against
the diff and folds the findings into the review. Stronger signal,
slower, costs money beyond the free tier.

The two compose well — chain them: `code-review` for the high-level
"is this the right approach" pass, then `coderabbit-review` for the
"what bugs did the analyzers find" pass before merge.

## Auth

Requires the `coderabbit` CLI to be authenticated. After first
`vibrate`, run `coderabbit login` inside the container — it'll open
the OAuth flow in your browser. The credentials persist in
`$HOME/.config/coderabbit/` and survive between sessions.

For non-interactive setups, set `CODERABBIT_API_KEY` directly.

## Why opt-in

Needs a CodeRabbit account, an interactive auth step, and outbound
network. None of that is reasonable to force on every user.

## Free-tier limits

CodeRabbit's free tier covers reasonable individual usage but caps
weekly review volume. Paid tiers unlock team workspaces and richer
analyzer coverage. See <https://coderabbit.ai/pricing>.
