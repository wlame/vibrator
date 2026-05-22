---
name: Kubernetes MCP
kind: mcp
default: false
size_mb: 35
category: cloud-infrastructure
host_aliases: [kubernetes, k8s]
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  # containers/kubernetes-mcp-server is the Go-native implementation
  # that talks directly to the Kubernetes API server (no kubectl
  # shellout overhead). Distributed as an npm wrapper for ease of
  # install. --read-only is the safer default for first-time setups;
  # remove the flag explicitly once you've decided you want write
  # access from Claude sessions.
  claude mcp add kubernetes \
    --scope user \
    --transport stdio \
    -- npx -y kubernetes-mcp-server@latest --read-only
source: https://github.com/containers/kubernetes-mcp-server
---

# Kubernetes MCP

Kubernetes + OpenShift control plane access from Claude Code. List
pods / services / deployments, inspect resource state, read logs,
describe events, manage helm releases. The Go-native server talks
straight to the Kubernetes API — no kubectl shell-out, no helm CLI
dependency.

## Kubeconfig

The container needs your kubeconfig. Two patterns:

1. **Mount the host kubeconfig** — pass `--volume=$HOME/.kube:/root/.kube`
   to `vibrate` (this is the default vibrator behavior for files
   matching the host-mount allowlist; check `vibrate --help` for the
   exact flag).
2. **Bake a minimal kubeconfig into the image** — useful for read-only
   service accounts pinned to a specific cluster.

`KUBECONFIG` env var override works too; point it at any mounted path.

## Read-only by default

This extension installs with `--read-only`. The MCP server still
exposes the full read surface (get, list, describe, logs, events) but
refuses write verbs (create, apply, delete, patch). Drop the flag in
your install snippet if you want Claude to be able to mutate cluster
state — recommended only against dev/staging clusters, not prod.

## Why opt-in

Cluster credentials are sensitive, and the install needs a real
kubeconfig to be useful. Default off so the wizard doesn't surface
kube-related noise to non-platform users.

## Comparison with podman-mcp / docker-mcp

Different layer: Kubernetes MCP is orchestrator-level; the container
runtime MCPs are about local Docker / Podman daemons. Pick the one
that matches your day-to-day.
