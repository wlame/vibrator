# Features

Features are the build-time capability layers that compose into an image. Each carries its
own Dockerfile fragment (emitted in [Stage 2](../lifecycle/build.md#stage-2-features)) and
declares dependencies on other features. Enable/disable them with `--with` / `--no`, or via
a [profile](profiles.md). See [Profiles & features](../guides/profiles-and-features.md) for
resolution rules.

## Catalogue

Listed in registry order — which is also dependency order (dependencies before
dependents).

| ID | Name | Deps | ≈ Size | Installs |
|----|------|------|--------|----------|
| `python` | Python 3.13 | — | 100 MB | `uv` + a prebuilt CPython 3.13 (python-build-standalone); symlinked to `python3`. |
| `go` | Go toolchain | — | 200 MB | Go (v1.26.2 in the image; copied from the official `golang` image; multi-arch). |
| `node` | Node.js + Bun | — | 150 MB | Node 22 + Bun; npm/npx shims. Needed by most JS-based MCP servers. |
| `playwright` | Playwright + Chromium | `node` | 500 MB | Chromium system libs + Chromium browser + Playwright MCP. |
| `postgres-client` | Postgres client | — | 30 MB | `psql`, `pg_dump`, `pg_restore`. |
| `gh` | GitHub CLI | — | 20 MB | `gh` from the official apt repo. |
| `docker-cli` | Docker CLI | — | 40 MB | `docker` client (no daemon) + a sudo wrapper. Auto-added by [`--dind`](../guides/docker-in-docker.md). |
| `audit-toolkit` | Production audit toolkit | `python` | 400 MB | trivy, syft, grype, semgrep, gitleaks, trufflehog, osv-scanner, checkov, dockle, scc, lizard. |
| `codex-cli` | OpenAI Codex CLI | `node` | 30 MB | `@openai/codex` (used for cross-model code review). |
| `ralphex` | ralphex | — | 20 MB | Autonomous coding loop — runs plans task-by-task in fresh sessions. |
| `aider` | aider AI pair programming | `python` | 80 MB | `aider-chat` via `uv tool install`. Opt-in; in no default profile. |

!!! tip "aider is opt-in"
    `aider` is in **no** profile. It only enters a build when you explicitly request it with
    `--with=aider`.

## Dependency behavior

- Selecting a feature **auto-enables** its transitive dependencies (e.g. `playwright` pulls
  in `node`; `audit-toolkit` pulls in `python`).
- A feature also gets pulled in when a [harness](../guides/harnesses.md) requires it (Codex
  and Pi require `node`) or when a selected [extension](../guides/extensions.md) declares it
  in `deps.features`.
- If you `--no` a feature that something else still needs, it's **auto-re-enabled** — to
  truly drop it, also drop its dependent. See
  [the resolution subtlety](../guides/profiles-and-features.md#a-subtlety-deps-win-over-no).

!!! warning "`--no` can be silently overridden"
    Because dependencies win over `--no`, disabling a feature that a harness, extension, or
    another selected feature still requires has no effect — it stays in the image. Drop the
    dependent too if you really need it gone.

## Always-on base toolkit

Independent of features, every image includes a base layer:
`ca-certificates`, `curl`, `wget`, `git`, `gpg`, `openssh-client`, `sudo`, `vim`, `less`,
`tree`, `jq`, `sqlite3`, `dnsutils`, `unzip`, `xz-utils`, `build-essential`, `locales`,
your chosen shell, **ripgrep** (`rg`), **fd**, and **fzf**. See
[Stage 1](../lifecycle/build.md#stage-1-base).

## Related pages

- [Profiles](profiles.md) — which features each profile bundles.
- [Profiles & features](../guides/profiles-and-features.md) — using `--with`/`--no`.
- [What happens on build](../lifecycle/build.md#stage-2-features) — where fragments are
  emitted.
