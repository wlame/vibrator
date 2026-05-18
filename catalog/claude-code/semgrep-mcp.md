---
name: Semgrep MCP
kind: mcp
default: false
size_mb: 80
deps:
  features: [python, audit-toolkit]
install: |
  uv tool install semgrep-mcp
  claude mcp add semgrep \
    --scope user \
    --transport stdio \
    -- semgrep-mcp
source: https://github.com/semgrep/semgrep-mcp
---

# Semgrep MCP

AI-accessible SAST via Semgrep's 5000+ rule library. Exposes tools like
`security_check`, `scan`, AST access, and rule authoring. Multi-language
(Python, Go, JS/TS, Java, Ruby, …).

Pair with the `audit-toolkit` feature for the full production-readiness
audit workflow (see `/opt/audit/production-audit-prompt.md` in the image).

## When to prefer this over the CLI

- **MCP**: interactive analysis, "explain this finding", false-positive
  triage, rule authoring conversations.
- **CLI** (`semgrep scan --config=auto .`): scripted output, CI gates.

Both are available when this entry is enabled.
