# `.vb` schema

The complete schema of the per-workspace pin file. For the practical guide, see
[The `.vb` file](../guides/configuration.md).

`.vb` is TOML, written with mode `0600`, discovered by walking up to the git or filesystem
root. Every field is optional except `harness` (required to launch). Empty/zero fields are
omitted on save.

## Full example

```toml
# vibrator workspace pin (`.vb`) — auto-managed by `vibrate`.
# Plaintext prereq tokens may live in [prereqs.*] subtables — keep gitignored.

harness = "claude-code"
profile = "full"
shell   = "zsh"
with    = ["gh"]
no      = ["aider"]
extensions = ["context7", "ecc-developer"]
mounts  = ["/data/refs", "/work/lib:rw"]

[llm]
provider = "openai"
model    = "gpt-4o"

[llm.auth]
env = "OPENAI_API_KEY"

[prereqs.claude-mem-server-beta]
api_key    = "cmem_..."
team_id    = "..."
project_id = "..."

[env]
GITHUB_TOKEN = "$GITHUB_TOKEN"
REGION       = "us-east-1"

[integrations]
serena     = "auto"
claude-mem = "host"

[identity]
name  = "Ada Alias"
email = "ada+vibe@example.com"

[hooks]
acknowledged_missing = ["node"]
```

## Top-level fields

| TOML key | Type | Default | Notes |
|----------|------|---------|-------|
| `harness` | string | — | Required to launch. One of `claude-code`, `codex`, `opencode`, `pi` (see [Harnesses](../guides/harnesses.md)). |
| `profile` | string | `full` | One of `minimal`, `backend`, `frontend`, `full` (see [Profiles](profiles.md)). |
| `shell` | string | `zsh` | One of `bash`, `zsh`, `fish`. |
| `with` | list | — | [Feature](features.md) IDs to enable on top of the profile. |
| `no` | list | — | Feature IDs to disable. |
| `extensions` | list | — | [Extension](../guides/extensions.md) IDs (scoped to the harness). |
| `mounts` | list | — | Extra host folders bound into the container at the same absolute path. Each entry is `PATH` or `PATH:rw`; read-only is the default. Written by the `--mount` flag and re-applied on every run. Changing the set recreates the container (the mount set is part of the container identity, not the image fingerprint). For claude-code, mounted folders are also passed to the agent via `--add-dir`. |

## `[llm]` { #llm }

The LLM provider choice, for [provider-agnostic harnesses](../guides/llm-providers.md). Omit
the whole table for Claude Code (Anthropic-only).

| TOML key | Type | Default | Notes |
|----------|------|---------|-------|
| `provider` | string | — | `anthropic`, `openai`, `ollama`, `lmstudio`, `openai-compat`. |
| `model` | string | — | Model identifier in the provider's namespace (`gpt-4o`, `qwen3:32b`, …). |
| `base_url` | string | provider default | Custom endpoint. Empty = provider's canonical default. |

### `[llm.auth]`

The credential plan. Omit entirely for local providers (`ollama`, `lmstudio`). For cloud
and `openai-compat`, set **exactly one** of:

| TOML key | Type | Notes |
|----------|------|-------|
| `env` | string | Name of a host env var carrying the key (preferred — secret stays out of `.vb`). |
| `value` | string | Literal API key. Plaintext in `.vb` (`0600`, gitignored). |

## `[prereqs.<id>]` { #prereqs }

Cached host-side [prerequisite](../guides/integrations.md) tokens, keyed by prereq ID. The
schema inside each subtable is loose — each prereq's bootstrap decides what it stores.
Keys are sorted on save for stable diffs.

For `claude-mem-server-beta` (see [claude-mem](../integrations/claude-mem.md)):

| Key | Notes |
|-----|-------|
| `api_key` | minted project-scoped bearer token (`cmem_...`) |
| `team_id` | resolved team UUID |
| `project_id` | resolved project UUID |
| `actor_id` | `vibrator:<hostname>:<workspace-path>` |

!!! warning "Plaintext credentials"
    `[prereqs.*]` subtables hold secrets in plaintext. `.vb` is written `0600` and added to
    `.gitignore` — keep it out of version control.

## `[env]`

Host→container environment forwarding, applied at `docker run`:

| Value form | Behavior |
|------------|----------|
| `"$NAME"` | Resolved from the host environment **at run time** (not at pin-load time). |
| `"literal"` | Forwarded as-is. |

Keys are sorted on save. See the [precedence rules](../guides/authentication.md#precedence)
for how `[env]` interacts with other forwarded vars.

## `[identity]` { #identity }

An optional **privacy override** for the contact name/email the agent uses
inside the container. By default the agent can read your Anthropic-account email
(`oauthAccount.emailAddress` in `~/.claude.json`, seeded from the host) and reuse
it in git commits and outbound HTTP "contact" headers. Set an alias here and
vibrator forces it everywhere the agent looks:

| TOML key | Type | Notes |
|----------|------|-------|
| `name` | string | Git author/committer name and the rewritten `oauthAccount.displayName`. |
| `email` | string | Git author/committer email and the rewritten `oauthAccount.emailAddress` — the field that actually keeps the real account email off the wire. |

```toml
[identity]
name  = "Ada Alias"
email = "ada+vibe@example.com"
```

When set, vibrator:

- forwards `GIT_AUTHOR_*` / `GIT_COMMITTER_*` / `EMAIL` so commits use the alias
  regardless of any gitconfig (git prefers these over config);
- has the container entrypoint rewrite `oauthAccount.emailAddress` /
  `.displayName` in `~/.claude.json` and pin `git config --global user.*`;
- **suppresses the host `~/.gitconfig` mount** so the real email can't leak through it.

Authentication is unaffected — the OAuth token and account UUID still carry your
real login; only the human-readable contact info is swapped. Changing `[identity]`
on an existing workspace recreates the container from the existing image (no
rebuild), so the alias takes effect immediately rather than leaking from a stale
container.

## `[integrations]`

Per-integration [hosting mode](../guides/integrations.md#hosting-modes), keyed by
integration ID:

| Value | Meaning |
|-------|---------|
| `auto` *(default when key absent)* | Probe host → http, else stdio fallback. |
| `host` | http only; warn if unreachable, no fallback. |
| `local` | stdio only; never probe the host. |
| `off` | Disable the integration's MCP wiring. |

## `[hooks]` { #hooks }

Per-workspace hook preferences, written when you respond to a
[missing-tool hook prompt](../lifecycle/startup.md#missing-tool-hooks) at launch.

| TOML key | Type | Notes |
|----------|------|-------|
| `acknowledged_missing` | list | Feature IDs you chose **not** to install for hooks that need them. Vibrator stops re-prompting for these; the container guard keeps skipping the affected hooks. Installing the feature (adding it to `with`) closes the gap and clears the entry. |

## Discovery and lifecycle

- **Found** by walking up from `$PWD` to the git root (or filesystem root); first `.vb`
  wins. Its directory is the workspace root.
- **Written** by the [wizard](commands/wizard.md) (unless `--no-save`) and by
  [`vibrate prereqs bootstrap`](commands/prereqs.md) / inline launch bootstrap.
- **Defaults applied on save:** `profile` → `full`, `shell` → `zsh` when unset.

## Related pages

- [The `.vb` file](../guides/configuration.md) — practical guide.
