---
name: Gondolin (micro-VM sandbox)
kind: tool
default: false
size_mb: 350
category: security
deps:
  features: [node]
runtime_needs:
  local_only: true
install: |
  # Gondolin is published by the Pi team (earendil-works). It's a
  # JavaScript-orchestrated QEMU micro-VM with a first-class Pi
  # extension that mounts the workspace at /workspace. The package
  # auto-caches guest assets on first run, so the install step here
  # just primes the cache by invoking bash inside a throwaway guest.
  npm install -g @earendil-works/gondolin
  npx @earendil-works/gondolin bash --exit-immediately || true
  # Pi extension hook (auto-registered when the package is on PATH and
  # Pi sees it in node_modules).
  pi install npm:@earendil-works/gondolin
source: https://github.com/earendil-works/gondolin
---

# Gondolin (micro-VM sandbox)

The **Pi-team-blessed sandbox**. Built by the same earendil-works org
that ships Pi itself, Gondolin is a JavaScript-orchestrated QEMU
micro-VM that runs your agent's tool calls inside an isolated guest.

## What it gives you

- Linux micro-VM per session (QEMU default, KVM where available)
- JavaScript-defined egress policy (allow/deny by host, port, path)
- Secret injection without exposing values inside the guest
- Virtual filesystem mounts — workspace at `/workspace` by convention
- SSH bridge host ↔ guest for interactive debugging
- Snapshotting for fast restart

## Why prefer this over Docker

Docker shares the host kernel. Gondolin runs a real second kernel,
which closes a category of container-escape and side-channel risk
that's hard to mitigate inside a bare Docker image. For agents that
execute model-generated code, the extra isolation is usually worth the
~200 MB VM image cost.

## Platform support

- **ARM64**: best (Apple Silicon, AWS Graviton)
- **Linux x86_64**: CI'd, works fine
- **Windows / x86 macOS**: less well-tested; use Docker fallback

## Pi extension hooks

When installed as a Pi package, Gondolin registers:

- Bash routing — every `bash` tool call is forwarded into the guest
- Workspace bind-mount at `/workspace` so file ops still touch host
  files transparently
- An egress-deny indicator in the footer when network is restricted

## Alternative sandboxes

If you want kernel-enforced sandboxing without a full VM, consider
`nono` (Landlock on Linux, Seatbelt on macOS). For Docker-based, Pi
has `pi-in-a-box`, `pi-less-yolo`, and `pi-devcontainers`. Vibrator's
own container is already a sandbox of sorts — pick Gondolin only if
you specifically need the second-kernel boundary.

Default off because of the size and platform constraints. Enable when
security posture demands it.
