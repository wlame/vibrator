# Harnesses

A **harness** is the AI coding agent that runs inside the container. Vibrator ships four
built-ins. You pick one with `--harness=<id>` or in the [wizard](../reference/commands/wizard.md);
it's stored as `harness` in [`.vb`](configuration.md).

Each harness declares five things that drive the rest of the tool:

- **Install** — the Dockerfile fragment that puts its binary in the image
  ([Stage 3](../lifecycle/build.md#stage-3-harness)).
- **Auth env vars** — host environment variables forwarded for authentication.
- **Host config dir** — where its settings live on your host (mounted for persistence).
- **Required features** — base [features](../reference/features.md) it needs.
- **LLM provider support** — whether the [LLM step](llm-providers.md) applies.

## At a glance

| Harness | ID | Launches | Required features | LLM provider step | Host config |
|---------|----|----------|-----|:---:|---|
| Claude Code | `claude-code` | `claude` | — | ✗ | `~/.claude` |
| OpenAI Codex | `codex` | `codex` | `node` | ✓ | `~/.codex` |
| OpenCode | `opencode` | `opencode` | — | ✓ | `~/.config/opencode` |
| Pi | `pi` | `pi` | `node` | ✓ | `~/.pi` |

`vibrate` (bare) launches the harness's CLI; [`vibrate shell`](../reference/commands/launch.md#vibrate-shell)
launches your shell instead. Update the agent in place with
[`vibrate update`](../reference/commands/update.md).

---

## Claude Code { #claude-code }

Anthropic's Claude Code. Installed via the official `claude.ai/install.sh` script and
symlinked into `/usr/local/bin/claude`.

- **Auth env vars:** `CLAUDE_CODE_OAUTH_TOKEN` (preferred), `ANTHROPIC_API_KEY` (fallback).
  See [Authentication](authentication.md), including `vibrate --login` for the OAuth flow.
- **Host config:** `~/.claude` — extensively mounted so onboarding state, rules, settings,
  hooks, and session history carry over (see [What happens on start](../lifecycle/startup.md#mounts)).
- **LLM provider step:** not shown — Claude Code is Anthropic-only, so the auth env vars
  suffice.
- **Update:** `claude update`.

Claude Code is also the harness with the richest [integration](integrations.md) support
([Serena](../integrations/serena.md), [claude-mem](../integrations/claude-mem.md)) and the
[ECC bundle](ecc.md).

---

## Codex { #codex }

OpenAI Codex. Installed with `npm install -g @openai/codex`, so it requires the `node`
feature (auto-enabled).

- **Auth env vars:** `OPENAI_API_KEY`.
- **Host config:** `~/.codex`.
- **LLM provider step:** shown. Codex maps your provider into an OpenAI-compatible shape
  (`OPENAI_API_KEY` + `OPENAI_BASE_URL`), so you can point it at OpenAI, a local
  [Ollama/LM Studio](llm-providers.md) server, or any OpenAI-compatible endpoint.
- **Update:** `npm install -g @openai/codex@latest`.

---

## OpenCode { #opencode }

SST's OpenCode. Installed as a self-contained prebuilt binary downloaded from GitHub
Releases (architecture-aware, pinned to `0.5.0`), so it needs no base feature.

- **Auth env vars:** `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`,
  `GROQ_API_KEY`, `OPENROUTER_API_KEY`, `DEEPSEEK_API_KEY`.
- **Host config:** `~/.config/opencode`.
- **LLM provider step:** shown. OpenCode is multi-provider; pick the one you want and the
  matching key is forwarded.
- **Update:** `opencode upgrade`.

---

## Pi { #pi }

`@mariozechner/pi-coding-agent`. Installed with `npm install -g`, so it requires the `node`
feature.

- **Auth env vars:** `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`,
  `GROQ_API_KEY`, `OPENROUTER_API_KEY`, `XAI_API_KEY`, `DEEPSEEK_API_KEY`, `HF_TOKEN`.
- **Host config:** `~/.pi`.
- **LLM provider step:** shown (OpenAI-compatible mapping).
- **Update:** `npm install -g @mariozechner/pi-coding-agent@latest`.

!!! note "ECC and Pi"
    The [ECC bundle](ecc.md) ships no `pi` adapter, so there are no `ecc-*` extensions for
    the Pi harness.

---

## Adding a harness

Harnesses are built-in Go types — adding one is a code change (a small struct implementing
the `Harness` interface plus a registry entry), submitted as a PR. Day-to-day "add another
plugin/MCP", by contrast, is data-driven via the [extensions catalogue](extensions.md) and
needs no Go code. See [Architecture](../reference/architecture.md).

## Related pages

- [Authentication](authentication.md) — getting your keys/tokens into the container.
- [LLM providers](llm-providers.md) — the provider step for Codex/OpenCode/Pi.
- [`vibrate update`](../reference/commands/update.md) — upgrading the agent CLI.
