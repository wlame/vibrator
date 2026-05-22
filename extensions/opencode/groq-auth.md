---
name: Groq provider auth
kind: tool
default: false
size_mb: 0
category: ai-integration
auth:
  env: GROQ_API_KEY
runtime_needs:
  third_party_api: Groq
  outbound_net: true
install: |
  # No install step — OpenCode picks up GROQ_API_KEY automatically
  # via the @ai-sdk/groq provider that ships with the binary.
  true
source: https://opencode.ai/docs/providers/
---

# Groq provider auth

Routes OpenCode through Groq's LPU inference cloud — known for
**extremely fast token generation** (often 500-1000+ tok/sec on
Llama / DeepSeek / Qwen / Kimi). Great for high-throughput interactive
loops where latency matters more than raw model quality.

## How it works

Set `GROQ_API_KEY` in the container environment. OpenCode's
`@ai-sdk/groq` provider auto-registers Groq-hosted models. Pick one
with `/model`:

```
/model groq/llama-3.3-70b-versatile
/model groq/deepseek-r1-distill-llama-70b
/model groq/qwen-2.5-coder-32b
```

## When to use

- Pair-programming style sessions where you want the agent to respond
  in <1s instead of 5-10s.
- Cheap iteration on plans, summaries, naming, refactor scaffolding.
- As a fallback when your primary provider hits rate limits.

The model menu is smaller and skews open-weights, so for hard
reasoning work you'll still want Opus / GPT-5 — but for the volume
work in any session, Groq is a real productivity bump.

## Why off by default

Niche — most users won't have a Groq account on hand. Drop the key in
and flip on when you want the speed.
