# About Vibrator

## What Vibrator is

Vibrator is a single-binary CLI that runs AI coding agents — [Claude Code](guides/harnesses.md#claude-code),
[Codex](guides/harnesses.md#codex), [OpenCode](guides/harnesses.md#opencode), and
[Pi](guides/harnesses.md#pi) — inside an isolated Docker container, one per workspace, with
declarative profile and extension configuration captured in a `.vb` file.

You install it with `just install`, invoke it as `vibrate` (alias `vb`), and run it from
any project directory:

```bash
cd ~/my-project
vibrate          # wizard fills the gaps, image builds once, agent drops you in
```

The first run launches a [wizard](getting-started/quickstart.md), writes a `.vb` TOML pin,
builds a tailored image once, and execs the agent's own CLI inside the container. Every
subsequent `vibrate` in that workspace reuses the container and re-enters instantly.

## Why it exists

AI coding agents want a clean, complete, reproducible toolchain — and they want to run
commands, install packages, and write files freely. Running them directly on your host
forces an uncomfortable trade-off:

- **Host pollution.** The agent installs language runtimes, CLIs, and global packages onto
  your machine, mixing its needs with yours.
- **No reproducibility.** "Works on my machine" creeps back in. Two projects, or two
  people, get subtly different environments depending on what's already installed.
- **No isolation.** An agent with shell access to your host can touch anything you can —
  not just the project you pointed it at.

The usual fix is to hand-roll a Dockerfile per project, but that means owning and
maintaining build files, credential forwarding, Docker networking, and runtime detection
yourself — for every repo. Vibrator turns that recurring chore into one declarative `.vb`
file and a single command.

!!! tip "The core idea is per-workspace isolation"
    Each project directory gets its own image and container, keyed by a content
    [fingerprint](reference/naming-and-labels.md). Two projects never share state, and the
    agent can't reach anything outside the mounted workspace.

## Design philosophy

Vibrator's design follows a few deliberate principles. The
[Architecture reference](reference/architecture.md) lists every decision; the themes are:

**Isolation per workspace.** A container is cheap and disposable; your host is not. Every
workspace gets its own image and container so the agent has a full, writable toolchain that
is sealed off from the rest of your machine. The workspace is mounted at the *same absolute
path* it has on the host, so git, editors, debuggers, and stack traces all see identical
paths inside and out.

**Declarative configuration.** The `.vb` file is [TOML](reference/vb-file.md) precisely
because it's meant to be read and hand-edited. It pins the harness, profile,
[features](reference/features.md), extensions, and (for provider-agnostic harnesses) the LLM
provider. Configuration is data, not a script.

**Determinism.** A given resolved spec produces a byte-identical Dockerfile, and variant
identity is a SHA-256 fingerprint over `harness;shell;features;extensions;user` (profile is
deliberately excluded — it's just a label for a feature bundle). Identical resolved specs
reuse one image, so builds are cacheable, inspectable, and reproducible.

**Batteries included, but opt-in.** [Profiles](reference/profiles.md) bundle whole
toolchains — the default `full` profile carries Python, Go, Node, Playwright, the GitHub
CLI, a Postgres client, and an audit toolkit — while [extensions](guides/extensions.md) (MCP
servers, skills, subagents, and bundles like [ECC](guides/ecc.md)) and host [integrations](integrations/index.md)
([Serena](integrations/serena.md), [claude-mem](integrations/claude-mem.md)) stay opt-in.
You start with a sensible bundle and add only what a given project needs.

**Runtime-agnostic, by shelling out.** Vibrator talks to Docker by invoking the `docker`
CLI rather than linking a Docker SDK. That keeps it free of import churn and lets it work
with any Docker-compatible runtime — [Docker Desktop, OrbStack, Colima, Rancher Desktop,
Podman, or native Linux](lifecycle/runtime-detection.md), all auto-detected.

**A small, typed surface.** Harnesses are a built-in Go interface, not a plugin system, so
adding one is a reviewed PR with a compile-time guarantee rather than runtime plugin
loading. The extensions catalogue, by contrast, is Markdown with YAML frontmatter — so the
catalogue is browsable without any tooling and "add a plugin" is a docs change.

## How it compares

Vibrator occupies a specific niche. It's worth being clear about where it helps and where a
different tool fits better.

| | Vibrator | Agent on the host | Hand-rolled Dockerfile | Devcontainers |
|---|---|---|---|---|
| Per-workspace isolation | ✅ Automatic | ❌ None | ⚠️ If you build it | ✅ Yes |
| Reproducible toolchain | ✅ Deterministic spec | ❌ Host-dependent | ⚠️ You maintain it | ✅ Yes |
| Zero per-repo Docker files | ✅ One `.vb` | — | ❌ Per-repo Dockerfile | ⚠️ `.devcontainer/` per repo |
| Built for AI agents | ✅ Harness-aware | ⚠️ Manual | ❌ DIY | ❌ Editor-oriented |
| Credential forwarding | ✅ Built-in | — | ❌ DIY | ⚠️ Partial |
| Runtime auto-detection | ✅ Built-in | — | ❌ DIY | ⚠️ Editor-tied |

**vs. running agents directly on the host.** The host approach has the least overhead and
the most risk: no build step, but no isolation, no reproducibility, and the agent shares
your machine's full surface. Vibrator trades a one-time per-spec build for a sealed,
repeatable environment.

**vs. a hand-rolled Dockerfile per project.** This gives you isolation, but you own every
moving part — the build file, credential mounts, Docker networking, runtime quirks — and
you repeat that work for each repo. Vibrator generates the Dockerfile from a declarative
spec and handles the plumbing. If you need a bespoke, long-lived image with logic Vibrator
doesn't model, a hand-rolled Dockerfile remains the more flexible choice.

**vs. devcontainers.** Devcontainers solve a related problem — reproducible dev environments
— but are oriented around an editor/IDE session and a per-repo `.devcontainer/` config.
Vibrator is CLI-first and agent-aware: it knows about harnesses, forwards host credentials,
wires up host integrations, and auto-detects the Docker runtime, none of which a
devcontainer does out of the box. If your goal is an IDE remote-development session rather
than running a coding agent, devcontainers are the better fit.

!!! warning "Docker-in-Docker is powerful and dangerous"
    The agent can run `docker` itself via [`--dind`](guides/docker-in-docker.md), but that
    grants host-root-equivalent access. Only enable it for trusted workspaces.

## Project status

Vibrator began as a bash implementation; the current Go rewrite (Go ≥ 1.26, a single static
binary) is **feature-complete**. It's released under the **MIT** license from
[github.com/wlame/vibrator](https://github.com/wlame/vibrator).

Releases are **tag-driven**: pushing a version tag runs [goreleaser](https://goreleaser.com/),
which builds cross-platform binaries with checksums and auto-generates the changelog.

If you're coming from the old bash version, [`vibrate migrate-pin`](reference/commands/migrate-pin.md)
converts a legacy `.vb.env` file into the current TOML `.vb` format.

!!! tip "New here?"
    Start with the [Getting started](getting-started/index.md) guide, then read
    [Core concepts](getting-started/concepts.md) for the vocabulary.

## Related pages

- [Getting started](getting-started/index.md) — install, first run, and the mental model.
- [Core concepts](getting-started/concepts.md) — workspace, variant, harness, profile,
  feature, extension, integration.
- [Architecture](reference/architecture.md) — the internal module map and every design
  decision.
- [Troubleshooting](troubleshooting.md) — concrete fixes for the failures you're likely to hit.
