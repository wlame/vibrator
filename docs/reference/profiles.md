# Profiles

A profile is a named starting bundle of [features](features.md). Pick one with
`--profile=<id>` (default `full`) or in the [wizard](commands/wizard.md). See
[Profiles & features](../guides/profiles-and-features.md) for how resolution works.

The four profiles, smallest to largest:

## `minimal`

> Just the always-on base toolkit (jq, rg, fd, vim, curl, ssh, git). No language runtimes.

- **Features:** *(none)*
- **Approx. size:** ~150 MB
- **Good for:** CI runs, tight resource environments, "I just want a shell with rg + jq +
  vim in a box".

## `backend`

> Backend dev — adds Python, Go, GitHub CLI, Postgres client, ralphex. No browser, no audit
> toolkit.

- **Features:** `python`, `go`, `gh`, `postgres-client`, `ralphex`
- **Approx. size:** ~600 MB

## `frontend`

> Frontend dev — adds Node.js + Bun + Playwright (Chromium). No Python, no Go, no audit
> toolkit.

- **Features:** `node`, `playwright`, `gh`, `ralphex`
- **Approx. size:** ~1 GB

## `full` *(default)*

> Everything except aider — backend + frontend + audit toolkit + Codex CLI. Default when
> the wizard is skipped.

- **Features:** `python`, `go`, `node`, `playwright`, `gh`, `postgres-client`,
  `audit-toolkit`, `codex-cli`, `ralphex`
- **Approx. size:** ~2 GB

## Notes

- The **base toolkit** (jq, ripgrep, fd, fzf, vim, git, curl, ssh, …) is present in every
  profile — it's part of [Stage 1](../lifecycle/build.md#stage-1-base), not a feature.
- `aider` is in no default profile — it's opt-in via `--with=aider`.
- Sizes are informational estimates only.
- Profile is **not** part of the [variant fingerprint](naming-and-labels.md) — only the
  resolved feature set is. `--profile=full` and the implicit default build the same image.

## Related pages

- [Features](features.md) — what each listed feature installs.
- [Profiles & features](../guides/profiles-and-features.md) — resolution and `--with`/`--no`.
