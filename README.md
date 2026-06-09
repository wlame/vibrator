# vibrator

A single-binary CLI that runs AI coding agents (Claude Code, Codex, OpenCode,
Pi) in isolated Docker containers per workspace — with declarative profile
and extension configuration via a `.vb` file at the project root.

> **Status:** Go rewrite is feature-complete on branch `pivot`. The previous
> bash implementation lives under
> [`previous-implementation/`](./previous-implementation) for design
> reference. Use [`vibrate migrate-pin`](#migrating-from-the-bash-version) to
> convert old workspaces.

## Quick start

```bash
# Install: builds the binary, places it on $PATH with a short `vb` alias,
# and registers shell-completion for $SHELL (bash, zsh, or fish).
just install                       # → /usr/local/bin (uses sudo if needed)
# or:
just install ~/.local/bin          # → user-local, no sudo

# First run — wizard fills the gaps, image builds, container drops you in:
cd ~/my-project
vibrate          # or `vb` — same thing

# Subsequent runs reuse the container (jump in instantly):
vibrate

# Skip the wizard entirely:
vibrate --no-wizard --harness=claude-code --profile=full --shell=zsh

# To remove:
just uninstall                     # symmetric — same arg as install
```

The first run writes a `.vb` file capturing your choices. It's added to
`.gitignore` automatically.

## Commands

| Command | Purpose |
|---|---|
| `vibrate` *(bare)* | Resolve `.vb` + flags, run wizard for unset fields, build/run/exec |
| `vibrate run` | Explicit form of the bare invocation (same flags) |
| `vibrate wizard` | Run the wizard standalone — preview without building |
| `vibrate build` | Build the image without running a container |
| `vibrate build-dockerfile` | Emit the generated Dockerfile to a file / stdout |
| `vibrate extensions list <harness>` | List extension entries available for a harness |
| `vibrate extensions show <id>` | Show a extension entry's frontmatter + body |
| `vibrate hostprobe` | Show host-detected plugins + which extension entries map |
| `vibrate prereqs status` | Probe host stacks (claude-mem server etc.) |
| `vibrate prereqs bootstrap <id>` | Run host-side setup (e.g., mint a claude-mem key) |
| `vibrate variants list` | List managed images + containers for this host |
| `vibrate variants prune` | Remove stopped containers / unused images |
| `vibrate runtime detect` | Auto-detect Docker socket path |
| `vibrate migrate-pin` | Convert old bash `.vb.env` → new TOML `.vb` |

Common flags (apply to `vibrate` and `vibrate run`):

```
--harness=<id>          claude-code | codex | opencode | pi (required)
--profile=<id>          minimal | backend | frontend | full  (default: full)
--shell=<id>            bash | zsh | fish                    (default: zsh)
--with=feature,...      Enable extra features beyond the profile
--no=feature,...        Disable features from the profile
--extensions=id,...        Extension IDs to install per the chosen harness
--no-wizard             Skip the wizard entirely; require all fields via flags or .vb
--no-save               Don't write the wizard's result to .vb
--rebuild               Force a fresh `docker build` even when an image exists
```

## Migrating from the bash version

If you have a workspace with a bash-era `.vb.env`, run:

```bash
vibrate migrate-pin --dry-run    # preview the conversion
vibrate migrate-pin              # write .vb, archive .vb.env to .vb.env.bak
```

The migrator maps known keys (HARNESS, PROFILE, WITH/NO, CLAUDE_MEM_SERVER_BETA_*)
to the new TOML structure and preserves unknown keys under `[env]` so nothing
is lost.

## What's inside

- **`cmd/vibrate`** — thin main that wires the cobra root command.
- **`internal/cli`** — every subcommand lives in its own file (run, build,
  build-dockerfile, extensions, wizard, hostprobe, prereqs, variants, runtime,
  migrate).
- **`internal/app`** — the orchestrator. Decision tree: load pin → flag
  overrides → wizard → validate → save → resolve specs → prereq probes →
  local-LLM startup → build/run/exec.
- **`internal/wizard`** — charmbracelet/huh-based form chain with adaptive
  gating (skips steps already supplied via flags).
- **`internal/harness`** — `Harness` interface; one subpackage per built-in
  (claudecode, codex, opencode, pi). Each declares its install snippet,
  required features, auth env vars, and LLM env-var mapping.
- **`internal/feature`** — base features (python, node, go, postgres-client,
  playwright, ...) with a topological resolver.
- **`internal/profile`** — minimal / backend / frontend / full bundles on top
  of `internal/feature`.
- **`internal/extensions`** — markdown + YAML frontmatter loader for the curated
  per-harness inventory under [`extensions/`](./extensions).
- **`internal/dockerfile`** — deterministic Dockerfile generator with
  golden-file tests for representative specs.
- **`internal/docker`** — Client interface over the `docker` CLI; production
  + mock implementations.
- **`internal/runtime`** — auto-detect across Docker Desktop, OrbStack,
  Colima, Rancher Desktop, Podman, native.
- **`internal/workspace`** — variant fingerprint + image/container naming.
- **`internal/config`** — `.vb` TOML loader/writer with `Pin` struct.
- **`internal/hostprobe`** — scan host for installed plugins (used by the
  wizard to pre-check extension entries).
- **`internal/prereq`** — host-side prereq types (HTTP / command / file
  probes) plus the claude-mem postgres bootstrap.
- **`internal/localprovider`** — Ollama / LM Studio lifecycle (enumerate
  models, ensure-running before container launch).
- **`internal/migrate`** — bash `.vb.env` → TOML `.vb` converter.

## ECC bundle (Everything Claude Code)

[ECC](https://github.com/affaan-m/ECC) ("Everything Claude Code", MIT) is a
cross-harness bundle of subagents, skills, rules, and hooks. vibrator ships it as
a family of opt-in `ecc-*` extensions — one per ECC profile — that, at image-build
time, shallow-fetch a pinned ECC commit and run ECC's own manifest-driven
installer into the harness-native config dir (`~/.claude`, `~/.codex`,
`~/.opencode`).

Profiles trade capability against agent-context cost — none is on by default, so
you opt in consciously (the wizard shows an "about" blurb for the focused entry):

| Profile | What it is | claude-code | codex | opencode |
|---|---|:--:|:--:|:--:|
| `ecc-minimal`   | lightest, no hook runtime | ✓ | — | ✓ |
| `ecc-core`      | lean baseline | ✓ | ✓ | ✓ |
| `ecc-developer` | default engineering preset (**recommended**) | ✓ | ✓ | ✓ |
| `ecc-security`  | core + security module | ✓ | ✓ | ✓ |
| `ecc-research`  | core + research/content | ✓ | ✓ | ✓ |
| `ecc-full`      | everything (heaviest context) | ✓ | ✓ | ✓ |

```bash
vibrate --harness=claude-code --extensions=ecc-developer
vibrate extensions show ecc-developer        # full docs for any profile
```

Notes:

- **codex** has no `ecc-minimal` — with the hook runtime skipped, minimal and
  core resolve to the same install on codex, so only `ecc-core` is offered.
- **pi** is not supported: ECC ships no `pi` adapter, so there are no `ecc-*`
  entries for the pi harness. (If upstream adds one, drop the files under
  `extensions/pi/` following the same pattern.)
- **Pinning:** every `ecc-*` entry pins the same ECC commit for reproducibility.
  Bump it deliberately with a single find-and-replace of `ECC_REF=` across
  `extensions/*/ecc-*.md`; `embedded_ecc_test.go` fails if the pins drift apart.

## Architectural decisions

| Decision | Choice |
|---|---|
| Docker integration | Shell out to `docker` CLI (no SDK dep) |
| `.vb` format | TOML (BurntSushi/toml) |
| Extensions format | Markdown + YAML frontmatter, one file per item |
| Harness extensibility | Built-in Go interface; PR to add a new one |
| Wizard library | charmbracelet/huh — gated step-by-step |
| Profiles | minimal / backend / frontend / full (default: full) |
| Variant identity | SHA-256 prefix over the spec's canonical form |
| Container reuse | Single-path workspace mount, label-driven discovery |
| Release | Manual GitHub UI release → CI builds + attaches per-platform binaries + checksums |

---

## Development

### Tooling requirements

| Tool | Why | Install |
|---|---|---|
| **Go** ≥ 1.26 | Compile the binary | https://go.dev/dl/ |
| **[`just`](https://just.systems/)** | Task runner — replaces `make` | see below |
| **Docker** | Only needed for integration tests | https://www.docker.com/ |
| `golangci-lint` *(optional)* | Stricter linting beyond `go vet` | https://golangci-lint.run/ |

### Installing `just`

`just` is a small Rust-based command runner — think Make but simpler, no
phony recipes, no implicit rules, cross-platform. The project's task
automation lives in [`justfile`](./justfile). Install once and you can run
`just` from anywhere in the repo.

```bash
# Pick one for your platform:
brew install just                              # macOS / Linux (Homebrew)
sudo apt install just                          # Debian 13+ / Ubuntu 24.10+
sudo dnf install just                          # Fedora
sudo pacman -S just                            # Arch Linux
winget install --id Casey.Just --exact         # Windows (winget)
scoop install just                             # Windows (Scoop)
cargo install just                             # Anywhere with a Rust toolchain

# Or grab the latest prebuilt binary:
curl -fsSL https://just.systems/install.sh \
  | bash -s -- --to ~/.local/bin
```

Verify with `just --version`. Optionally enable shell completion:

```bash
just --completions zsh  > "${fpath[1]}/_just"            # zsh
just --completions bash > /etc/bash_completion.d/just    # bash
just --completions fish > ~/.config/fish/completions/just.fish
```

### Daily commands

From the repo root:

```bash
just                # list every available recipe (default action)
just test           # run unit tests with -race
just test-cover     # tests + write coverage.out
just build          # produce ./build/vibrate
just lint           # go vet (+ golangci-lint if installed)
just integration    # real-docker tests (skipped unless INTEGRATION=1)
just install        # build + place binary on $PATH + `vb` alias + shell completion
just uninstall      # remove the binary, alias, and completion
just tidy           # go mod tidy
just clean          # remove ./build/ and ./dist/

just dist                     # build release binaries + checksums → ./dist/

VERSION=0.2.0 just build      # release build with embedded version
INTEGRATION=1 just integration  # actually run integration tests

just ci             # what CI runs: lint + test + build
just run runtime detect       # build then run the binary with args
just versions       # print just / go / vibrator versions for bug reports
```

### Releasing

Releases are created **manually in the GitHub UI**, and CI attaches the
binaries:

1. Go to **Releases → Draft a new release**.
2. Create (or choose) a tag like `v0.3.0`, write the release notes, and click
   **Publish release**.
3. Publishing fires [`.github/workflows/release.yml`](./.github/workflows/release.yml),
   which builds the `vibrate` binary for `linux × {amd64, arm64}` and
   `darwin × {amd64, arm64}` and uploads them — plus `checksums.txt` — as
   assets on that release. Your notes are left untouched; CI only adds the
   binaries. Re-publishing (or re-running the job) replaces the assets in place.

Assets are named `vibrate_<version>_<os>_<arch>` (e.g.
`vibrate_0.3.0_darwin_arm64`), with the version baked into the binary so
`vibrate --version` matches the tag.

Verify the exact artifacts locally first with `VERSION=0.3.0 just dist` — same
build, targets, and checksums as CI, written to `./dist/`, but nothing is
tagged, pushed, or uploaded.

> **Windows is not built.** vibrate uses Unix-only syscalls (`syscall.Setsid`,
> `syscall.Stat_t`) and does not cross-compile to `windows/*`; Windows users run
> it through WSL2 with the `linux/amd64` binary.

### Useful `just` flags

```bash
just --list                # list recipes (same as `just` bare)
just --show <recipe>       # print a recipe's source
just --evaluate            # show resolved variable values
just --choose              # interactive picker (requires fzf)
just --fmt --check         # verify the justfile is canonically formatted
just --completions <shell> # emit shell completion script
```

### Why `just` instead of Make?

- **No phony declarations.** `just` is a command runner, not a build system.
  It doesn't compare file timestamps; it just runs commands. Go's own build
  cache handles incremental work for us, so we don't need Make's targets.
- **Cross-platform.** Recipes behave the same on Linux/macOS/Windows. No
  BSD-vs-GNU `sed -i` workarounds (which the old bash Makefile had).
- **Static error reporting.** Syntax errors and undefined recipes are caught
  before anything runs.
- **`just --list`** is self-documenting — every recipe with a `#` comment
  above it shows up with that description.

## License

MIT — see [LICENSE](./LICENSE).
