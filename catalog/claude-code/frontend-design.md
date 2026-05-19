---
name: frontend-design
kind: plugin
default: false
size_mb: 2
install: |
  # Anthropic's official marketplace; registered as short name
  # `claude-plugins-official`. Idempotent across cached layers.
  claude plugin marketplace add anthropics/claude-plugins-official 2>/dev/null || true
  claude plugin install frontend-design@claude-plugins-official
source: https://github.com/anthropics/claude-plugins-official
---

# frontend-design (Anthropic official)

Guides Claude through four design dimensions — **purpose, tone, constraints,
differentiation** — before writing a single line of CSS. Pushes for
unexpected font pairings, asymmetric layouts, scroll-triggered animations,
layered visual depth.

Output feels intentional and branded, not templated. Avoids the generic AI
aesthetic that the same prompt would produce without scaffolding.

## When to enable

Only useful for frontend work — pair with the `frontend` profile or with
`--with=playwright` for verification. Default = off in vibrator because
backend-focused users don't need it.

## Comparison with raw `claude` for frontend

Without this skill, Claude tends to produce "looks AI-generated" interfaces:
generic font stacks, symmetric layouts, predictable hover states. With it,
the output asks for and respects creative constraints.
