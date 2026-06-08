# Installation

Vibrator is a single Go binary. You install it by building from source with the
project's [`just`](https://just.systems/) task runner, which also places a short `vb`
alias on your `$PATH` and registers shell completion for your shell.

## Requirements

| Tool | Why | Notes |
|------|-----|-------|
| **Go** ≥ 1.26 | Compiles the binary | <https://go.dev/dl/> |
| **[`just`](https://just.systems/)** | Task runner used for install/build | See [installing `just`](#installing-just) |
| **Docker** | Runs the containers at runtime | Any Docker-compatible runtime — see [Runtime detection](../lifecycle/runtime-detection.md) |

!!! note "Docker isn't needed to build the binary"
    You only need Docker to actually *run* a workspace (and to run the integration test
    suite). Building and installing `vibrate` itself needs only Go and `just`.

## Install with `just`

From the repository root:

```bash
# Install: builds the binary, places it on $PATH with a short `vb` alias,
# and registers shell completion for $SHELL (bash, zsh, or fish).
just install                       # → /usr/local/bin (uses sudo if needed)

# Or install user-locally, no sudo:
just install ~/.local/bin
```

This recipe:

1. Builds `./build/vibrate`.
2. Copies it to the destination directory (default `/usr/local/bin`).
3. Creates a `vb` symlink alongside it — `vb` and `vibrate` are interchangeable.
4. Generates and installs shell completion for your current `$SHELL`.

To remove everything again, use the symmetric recipe with the same argument:

```bash
just uninstall                     # removes the binary, alias, and completion
just uninstall ~/.local/bin        # if you installed user-locally
```

## Installing `just`

`just` is a small Rust-based command runner — think Make, but simpler. Install it once:

=== "macOS / Linux"

    ```bash
    brew install just                       # Homebrew
    cargo install just                      # any Rust toolchain
    curl -fsSL https://just.systems/install.sh | bash -s -- --to ~/.local/bin
    ```

=== "Debian / Ubuntu / Fedora / Arch"

    ```bash
    sudo apt install just                   # Debian 13+ / Ubuntu 24.10+
    sudo dnf install just                   # Fedora
    sudo pacman -S just                     # Arch
    ```

=== "Windows"

    ```powershell
    winget install --id Casey.Just --exact  # winget
    scoop install just                      # Scoop
    ```

Verify with `just --version`.

## Verify the install

```bash
vibrate --version        # prints the embedded version (or "dev")
vibrate --help           # lists every subcommand
vb runtime detect        # confirms vibrate can find your Docker socket
```

If `vibrate runtime detect` prints a runtime and socket path, you're ready to go. If it
can't find a socket, see [Runtime detection](../lifecycle/runtime-detection.md).

## Building manually

If you'd rather not use `just`, you can build and install by hand:

```bash
# Build with an embedded version string.
VERSION=0.2.0 just build        # → ./build/vibrate
# or with plain Go:
go build -o build/vibrate ./cmd/vibrate

# Put it on your PATH yourself.
cp build/vibrate /usr/local/bin/vibrate
ln -sf /usr/local/bin/vibrate /usr/local/bin/vb
```

Shell completion can be generated on demand — `vibrate` is built with
[Cobra](https://github.com/spf13/cobra), which provides a `completion` command:

```bash
vibrate completion zsh  > "${fpath[1]}/_vibrate"
vibrate completion bash > /etc/bash_completion.d/vibrate
vibrate completion fish > ~/.config/fish/completions/vibrate.fish
```

## Next steps

- [Quick start](quickstart.md) — your first `vibrate` run.
- [Core concepts](concepts.md) — the mental model.
