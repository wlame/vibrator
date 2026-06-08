# Authentication

The container needs your credentials, but Vibrator never bakes them into the image —
they're **forwarded at run time** from your host environment, files, or sockets. This guide
covers every way credentials reach the container.

## Harness auth env vars

Each [harness](harnesses.md) declares the env vars it uses for auth. At `docker run`,
Vibrator forwards those that are **set on your host**:

| Harness | Forwarded if set |
|---------|------------------|
| Claude Code | `CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY` |
| Codex | `OPENAI_API_KEY` |
| OpenCode | `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `GROQ_API_KEY`, `OPENROUTER_API_KEY`, `DEEPSEEK_API_KEY` |
| Pi | `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `GROQ_API_KEY`, `OPENROUTER_API_KEY`, `XAI_API_KEY`, `DEEPSEEK_API_KEY`, `HF_TOKEN` |

So the simplest path is to export the key in your shell before running `vibrate`:

```bash
export ANTHROPIC_API_KEY=sk-...
vibrate
```

The [welcome banner](../lifecycle/startup.md#shell-rc-and-the-welcome-banner) shows which
credential it detected (`auth: Anthropic API key`), or `not configured` if none is set.

## Claude Code OAuth

Claude Code can authenticate via a browser OAuth flow instead of an API key. Two paths:

### `vibrate --login`

```bash
vibrate --login
```

This:

1. Starts the container detached so the [entrypoint](../lifecycle/startup.md#2-the-entrypoint-entrypointsh)
   finishes its setup.
2. Execs `claude auth login`, intercepts the printed OAuth URL, and **opens it in your host
   browser** automatically.
3. Writes the resulting auth state back to your host `~/.claude.json`.
4. Execs the harness.

Because the auth is saved to the host, subsequent launches *without* `--login` are
pre-authenticated — the [entrypoint merges](../lifecycle/startup.md#2-the-entrypoint-entrypointsh)
host config into the container on every start. `--login` always runs the flow when passed,
so use it to re-authenticate or switch accounts.

### Host config merge

If you've already authenticated Claude Code on your host (`~/.claude.json` has
`oauthAccount`), Vibrator mounts that config read-only and the entrypoint merges the
OAuth/onboarding state into the container — so you're authenticated without `--login` or an
API key.

### Token file fallback

For Claude Code, if `CLAUDE_CODE_OAUTH_TOKEN` isn't exported, Vibrator falls back to reading
`~/.claude-docker-token` on your host (whitespace-trimmed). Handy for keeping a long-lived
token in a file instead of your shell rc.

## Extension and LLM credentials

- **[Extensions](extensions.md)** that declare `auth.env` get that host env var
  [forwarded](../lifecycle/startup.md#forwarded-environment) — e.g. an extension needing
  `OPENAI_API_KEY` picks it up from your host automatically.
- **LLM providers** for Codex/OpenCode/Pi resolve their key from
  [`[llm.auth]`](llm-providers.md#cloud-providers) — either a host env var name (preferred)
  or a pasted literal.

## `[env]` forwarding

For anything else, forward arbitrary variables via `.vb`:

```toml
[env]
GITHUB_TOKEN = "$GITHUB_TOKEN"   # $NAME → resolved from host at run time
REGION       = "us-east-1"       # literal value
```

## GPG agent forwarding

If you've configured an `extra-socket` in `~/.gnupg/gpg-agent.conf`, Vibrator
[auto-mounts](../lifecycle/startup.md#mounts) it at `/gpg-agent-extra` and the entrypoint
symlinks it where gpg expects. Then `git commit -S` and `gpg --sign` inside the container
use your **host private key** — the key itself never crosses the boundary. No flag needed;
it's automatic when the extra-socket exists.

## AWS credentials

If `~/.aws` exists on your host, it's mounted **read-only** into the container so AWS CLI /
SDK calls work. Read-only means a buggy container can't rotate or wipe your credentials.
Automatic — no flag.

## Precedence

When env var names collide, later sources win:
**harness auth → LLM-derived → extension `auth.env` → `[env]` overrides**. So your explicit
`[env]` value always takes precedence, and the harness's own auth beats an extension's hint.

## Related pages

- [What happens on start](../lifecycle/startup.md#forwarded-environment) — the full
  forwarding order.
- [LLM providers](llm-providers.md) — provider credentials for Codex/OpenCode/Pi.
- [Environment variables](../reference/environment-variables.md) — every forwarded var.
