# FAQ

Quick answers to common questions. Each links to the fuller explanation.

## Getting started

??? question "What is Vibrator, in one sentence?"
    A single Go binary (`vibrate`) that builds a tailored Docker image per project and runs
    an AI coding agent — Claude Code, Codex, OpenCode, or Pi — inside it, configured by a
    `.vb` file. See the [overview](index.md).

??? question "What's the difference between `vibrate` and `vb`?"
    Nothing — `vb` is a symlink to `vibrate`, installed alongside it. Use either.

??? question "Do I need Docker installed?"
    To *run* a workspace, yes — any Docker-compatible runtime (Docker Desktop, OrbStack,
    Colima, Rancher Desktop, Podman, native). To merely *build* the binary, no. See
    [Installation](getting-started/installation.md).

??? question "How do I start?"
    `cd` into a project and run `vibrate`. The [wizard](reference/commands/wizard.md) asks a
    few questions, the image builds, and the agent drops you in. See the
    [Quick start](getting-started/quickstart.md).

## Daily use

??? question "Why was the first run slow and the next one instant?"
    The first run does a `docker build` for your [variant](getting-started/concepts.md#variant).
    Once an image exists, `vibrate` reuses the container (or starts it) and just `exec`s in.
    See [Lifecycle](lifecycle/index.md#where-the-decision-is-made).

??? question "How do I change my setup after the first run?"
    [`vibrate reconfigure`](reference/commands/wizard.md#vibrate-reconfigure) re-runs the
    wizard and rebuilds, **preserving your credentials and `[env]`**. Or edit
    [`.vb`](guides/configuration.md) by hand.

??? question "How do I get a shell instead of the agent?"
    [`vibrate shell`](reference/commands/launch.md#vibrate-shell). Same build/run logic, it
    just execs your shell at the end.

??? question "How do I update the agent without rebuilding everything?"
    [`vibrate update`](reference/commands/update.md) upgrades just the harness CLI in place.
    For a full rebuild from the Dockerfile, use `vibrate --rebuild`.

??? question "How do I skip the wizard?"
    Pass everything as flags and add `--no-wizard`:
    `vibrate --no-wizard --harness=claude-code --profile=full`. See
    [the launch flags](reference/commands/launch.md).

??? question "How do I try a config once without saving it?"
    Add `--no-save` — the resolved config isn't written to `.vb`.

## Configuration

??? question "Where does Vibrator look for `.vb`?"
    It walks up from your current directory to the git root (or filesystem root); the first
    `.vb` wins, and its directory becomes the workspace root. See
    [The `.vb` file](guides/configuration.md#how-its-found).

??? question "Is `.vb` safe to commit?"
    No — it can hold plaintext credentials (minted keys, pasted API keys). It's written
    `0600` and auto-added to `.gitignore`. Treat it like a dotenv file.

??? question "What's a profile vs. a feature?"
    A [profile](reference/profiles.md) is a named bundle of
    [features](reference/features.md); features are the build-time layers (Python, Node,
    Playwright, …). Tune a profile with `--with`/`--no`. See
    [Profiles & features](guides/profiles-and-features.md).

??? question "I removed a feature with `--no` but it's still there. Why?"
    Something else needs it. `--no=node --with=playwright` keeps `node` because Playwright
    depends on it. Drop the dependent too. See
    [the resolution subtlety](guides/profiles-and-features.md#a-subtlety-deps-win-over-no).

??? question "Why do `--profile=full` and the default produce the same image?"
    Profile is just a label for a feature bundle; it's deliberately excluded from the
    [fingerprint](reference/naming-and-labels.md#the-fingerprint). Identical resolved
    features → identical image.

## Authentication

??? question "The banner says `auth: not configured`. What now?"
    Export the relevant key on the host before running (e.g. `export ANTHROPIC_API_KEY=...`),
    or for Claude Code run `vibrate --login` for the browser OAuth flow. See
    [Authentication](guides/authentication.md).

??? question "What does `vibrate --login` do?"
    Runs `claude auth login` in the container, opens the OAuth URL in your host browser, and
    writes the auth back to your host so future runs are pre-authenticated. See
    [`--login`](guides/authentication.md#vibrate-login).

??? question "Can I sign git commits inside the container?"
    Yes — if you've configured a gpg-agent `extra-socket`, it's auto-forwarded so
    `git commit -S` uses your host key without it leaving the host. See
    [GPG forwarding](guides/authentication.md#gpg-agent-forwarding).

??? question "Are my AWS credentials available?"
    If `~/.aws` exists on the host, it's mounted read-only. See
    [AWS credentials](guides/authentication.md#aws-credentials).

## Models & extensions

??? question "Can I use a local model (Ollama / LM Studio)?"
    Yes, for Codex/OpenCode/Pi. Pick it in the wizard; Vibrator ensures the provider is
    running and pulls the model before launch. See [LLM providers](guides/llm-providers.md).

??? question "How do I add an MCP server / skill / plugin?"
    Select an [extension](guides/extensions.md) by ID:
    `--extensions=context7,github-mcp`. Browse with `vibrate extensions list <harness>`.

??? question "What is ECC?"
    [Everything Claude Code](guides/ecc.md) — an opt-in bundle of agents, skills, rules, and
    hooks, shipped as `ecc-*` extensions. Try `--extensions=ecc-developer`.

## Integrations

??? question "What are integrations vs. extensions?"
    [Extensions](guides/extensions.md) are installed *into the image*. [Integrations](guides/integrations.md)
    connect the container to a *host-side service* (Serena, claude-mem) with automatic
    transport fallback.

??? question "What's the difference between `auto`, `host`, `local`, and `off`?"
    They're [hosting modes](guides/integrations.md#hosting-modes) for an integration:
    probe-then-fall-back, host-only, container-only, or disabled.

## Troubleshooting

??? question "`vibrate` can't find my Docker socket."
    Run [`vibrate runtime detect`](reference/commands/runtime.md). Override with
    `--docker-socket=...` or `VIBRATOR_DOCKER_SOCKET`. See
    [Runtime detection](lifecycle/runtime-detection.md).

??? question "I get `node: not found` (or python) errors from hooks."
    A hook in your `~/.claude/settings.json` shells out to a tool that isn't installed in the
    image — common with the `minimal` profile. `vibrate` detects this at launch and offers to
    **install the tool** (adds `--with=node`, rebuilds) or **disable those hooks**, and the
    container also auto-skips any hook whose tool isn't on `PATH`. See
    [Missing-tool hooks](lifecycle/startup.md#missing-tool-hooks).

??? question "How do I see what the container setup is doing?"
    Set `VIBRATOR_VERBOSE=1` to print `[vibrator] ...` diagnostics from the entrypoint and
    `claude-exec`. Silence the banner with `VIBRATOR_NO_BANNER=1`.

??? question "How do I clean up old images and containers?"
    [`vibrate variants list`](reference/commands/variants.md#vibrate-variants-list) to see
    them, [`vibrate variants prune`](reference/commands/variants.md#vibrate-variants-prune)
    to remove stopped containers and unused images.

??? question "The container can't reach a service on my host."
    Use `host.docker.internal` (Vibrator adds the host-gateway mapping automatically). For
    host integration servers, check they bind `0.0.0.0` and your
    [hosting mode](guides/integrations.md#hosting-modes).

??? question "Can the agent run `docker` itself?"
    Yes, with [`--dind`](guides/docker-in-docker.md) — but it grants host-root-equivalent
    access, so only use it for trusted workspaces.

## Still stuck?

- Read the [Lifecycle](lifecycle/index.md) pages — most "why did it do that?" answers live
  there.
- Open an issue on [GitHub](https://github.com/wlame/vibrator).
