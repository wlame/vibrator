# LLM providers

For **provider-agnostic harnesses** — [Codex](harnesses.md#codex),
[OpenCode](harnesses.md#opencode), and [Pi](harnesses.md#pi) — the
[wizard](../reference/commands/wizard.md) shows an LLM provider step, and your choice is
stored in [`.vb`](configuration.md) under `[llm]`. [Claude Code](harnesses.md#claude-code)
is Anthropic-only, so it skips this step and uses its
[auth env vars](authentication.md) instead.

## Supported providers

| Provider | Type | Credential | Default endpoint |
|----------|------|-----------|------------------|
| `anthropic` | cloud | API key | `https://api.anthropic.com` |
| `openai` | cloud | API key | `https://api.openai.com` |
| `ollama` | local | none | `http://host.docker.internal:11434` |
| `lmstudio` | local | none | `http://host.docker.internal:1234` |
| `openai-compat` | custom | API key | your URL |

The harness maps your provider/model/key into the environment variables it expects (for
Codex/Pi that's an OpenAI-compatible `OPENAI_API_KEY` + `OPENAI_BASE_URL` shape; OpenCode
uses per-provider keys).

## The `[llm]` block

```toml
[llm]
provider = "openai"
model    = "gpt-4o"

[llm.auth]
env = "OPENAI_API_KEY"     # name of a host env var (preferred)
# value = "sk-..."         # OR a pasted literal (plaintext in .vb — avoid if possible)
```

For local providers, `[llm.auth]` is omitted — no key is needed:

```toml
[llm]
provider = "ollama"
model    = "qwen3:32b"
# base_url defaults to http://host.docker.internal:11434
```

See the [`.vb` reference](../reference/vb-file.md#llm) for the full schema.

## Cloud providers

The wizard's cloud path asks for a **model** (preset list with a custom fallback) and an
**auth method**:

- **Env var (recommended)** — store the *name* of a host environment variable. The key
  stays in your shell and is forwarded at run time; it never touches `.vb`.
- **Paste a value** — stored as a literal in `.vb` (mode `0600`, gitignored). Convenient
  but less safe — prefer the env var path.

## Local providers (Ollama / LM Studio)

Local providers run on your host and are reached from the container via
`host.docker.internal`. Vibrator manages their lifecycle before launch:

- **Model enumeration** — the wizard lists models already downloaded on your host (or lets
  you type a custom name if the server is unreachable).
- **Ensure-running** — at launch, `vibrate` probes the provider; if it's down it tries to
  start it (`ollama serve`), waits for readiness, and pulls the requested model if missing.
  If it can't be reached or started, the launch aborts rather than running a container that
  would immediately fail.

| Provider | Host binary | Default URL |
|----------|-------------|-------------|
| Ollama | `ollama` | `http://host.docker.internal:11434` |
| LM Studio | `lms` | `http://host.docker.internal:1234` |

```bash
# Example: Codex against a local Ollama model.
vibrate --harness=codex   # then pick "ollama" + a model in the wizard
```

## OpenAI-compatible endpoints

Choose `openai-compat` to point at any OpenAI-compatible HTTP API — a self-hosted gateway,
a proxy, or a third-party service. You supply the base URL and a credential (env var or
literal).

## Related

- [Harnesses](harnesses.md) — which harnesses support the provider step.
- [Authentication](authentication.md) — how keys are forwarded into the container.
- [`.vb` schema](../reference/vb-file.md#llm) — the `[llm]` block in full.
