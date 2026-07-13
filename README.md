# vibrator

Run AI coding agents — **Claude Code**, Codex, OpenCode, or Pi — in an isolated
Docker container per project. One Go binary (`vibrate`, alias `vb`), a declarative
`.vb` file that pins your setup, and a wizard that fills the gaps.

📖 **[Full documentation →](https://wlame.github.io/vibrator/)**

> Claude Code is the primary, fully-exercised harness. Codex, OpenCode, and Pi are
> wired in and usable, but **experimental** — expect rough edges and please file issues.

## Features

- **One binary, zero host clutter** — the only thing you install on the host is Docker; everything else runs in the container.
- **Isolated per project** — a throwaway container per workspace, so your host stays clean and projects never share state.
- **Declarative `.vb`** — harness, profile, extensions, mounts, and LLM provider in one TOML file. Auto-gitignored, replayed on every run.
- **Wizard fills the gaps** — anything you don't pass as a flag, it asks; fully scriptable with `--no-wizard`.
- **Runtime auto-detection** — Docker Desktop, OrbStack, Colima, Rancher Desktop, Podman, or native Docker.
- **Profiles & features** — `minimal` / `backend` / `frontend` / `full`, fine-tuned with `--with` / `--no`.
- **Curated extensions** — MCP servers, skills, and agents (Serena, Context7, Playwright, claude-mem, the ECC bundle) added with `--extensions`.
- **Cloud or local LLMs** — Anthropic, OpenAI, any OpenAI-compatible endpoint, or a local Ollama / LM Studio model.

## Install

### Download a release binary (quickest)

Grab the asset for your OS/arch from the **[latest release](https://github.com/wlame/vibrator/releases/latest)**
— `vibrate_<version>_<os>_<arch>`, built for Linux and macOS on amd64/arm64 — then:

```bash
chmod +x vibrate_*_*_*
sudo mv vibrate_*_*_* /usr/local/bin/vibrate
vibrate --version
```

> Windows isn't built natively — run the `linux/amd64` binary under WSL2.

### Build from source

Needs [Go](https://go.dev/dl/) and [just](https://just.systems). `just install`
compiles the binary, puts it on your `$PATH`, adds the `vb` alias, and registers
shell completion.

```bash
git clone https://github.com/wlame/vibrator.git
cd vibrator
just install                 # → /usr/local/bin (sudo); or: just install ~/.local/bin
```

## Quick start

```bash
cd ~/my-project
vibrate          # or `vb` — wizard fills the gaps, builds the image, drops you in
```

The first run writes a `.vb` file (auto-gitignored) capturing your choices; every
run after that reuses the container and jumps straight in.

```bash
vibrate --harness=codex --profile=backend   # pick an agent + stack
vibrate --extensions=ecc-developer          # add a curated bundle
vibrate shell                               # a plain shell instead of the agent
vibrate reconfigure                         # re-run the wizard, keep credentials
```

See the **[CLI reference](https://wlame.github.io/vibrator/reference/commands/)**
for every command and flag.

## Development

```bash
just              # list all recipes
just build        # build ./build/vibrate (native platform)
just build-all    # cross-compile every supported platform
just test         # unit tests with the race detector
just ci           # what CI runs: lint + test + build
```

Releases are drafted in the GitHub UI; CI builds the per-platform binaries and
attaches them. The [documentation](https://wlame.github.io/vibrator/) has the full
development and release guide, plus an architecture overview.

## License

MIT — see [LICENSE](./LICENSE).
