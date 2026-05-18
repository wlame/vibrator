# vibrator (Go rewrite, in progress)

A single-binary CLI that runs AI coding agents (Claude Code, Codex, OpenCode,
Pi) in isolated Docker containers per workspace — with declarative profile
and catalog configuration via a `.vb` file at the project root.

> **Status:** rewrite-in-progress on branch `pivot`. The previous bash
> implementation is preserved under [`previous-implementation/`](./previous-implementation)
> for design reference.

## Quick start (when Phase 4 lands)

```bash
just build
sudo install build/vibrate /usr/local/bin/

cd ~/my-project
vibrate    # wizard → image build → container drops you into a shell
```

After the first run you'll have a `.vb` file pinning your choices. Subsequent
`vibrate` calls in the same workspace skip the wizard and jump straight in.

## Phase 1: Foundation (done)

- `go.mod` + cobra CLI scaffold under [`cmd/vibrate`](./cmd/vibrate) and
  [`internal/cli`](./internal/cli)
- [`internal/docker`](./internal/docker) — Client interface + CLI-backed impl +
  in-memory mock
- [`internal/runtime`](./internal/runtime) — auto-detect Docker socket across
  Docker Desktop, OrbStack, Colima, Rancher Desktop, Podman, native
- [`internal/workspace`](./internal/workspace) — variant fingerprint + image /
  container naming
- [`internal/config`](./internal/config) — `.vb` TOML loader/writer
- CI: `.github/workflows/ci.yml` runs vet + race-tests + lint + build smoke

## Architectural decisions

| Decision | Choice |
|---|---|
| Docker integration | Shell out to `docker` CLI |
| `.vb` format | TOML |
| Catalog format | Markdown + YAML frontmatter, one file per item |
| Harness extensibility | Built-in Go interface |
| TUI | charmbracelet/huh — wizard fills CLI-flag gaps only |
| Profiles | minimal / backend / frontend / full |

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
just tidy           # go mod tidy
just clean          # remove ./build/

VERSION=0.2.0 just build      # release build with embedded version
INTEGRATION=1 just integration  # actually run integration tests

just ci             # what CI runs: lint + test + build
just run runtime detect       # build then run the binary with args
just versions       # print just / go / vibrator versions for bug reports
```

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
