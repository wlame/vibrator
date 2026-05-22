---
name: "Profile: DevOps (Kubernetes / Terraform)"
kind: plugin
default: false
size_mb: 240
category: harness-specific
deps:
  features: [node, go]
install: |
  # Suggested archetype — SRE / platform engineer running k8s, Terraform,
  # AWS, Helm. Heavy on permission gates because the blast radius of a
  # bad kubectl is enormous.
  set -e

  # 1. MCP foundation
  pi install npm:pi-mcp-adapter

  # 2. Cloud / infra MCPs
  mkdir -p ~/.pi/agent
  node - <<'JS'
  const fs = require('fs'), path = require('path');
  const cfg = path.join(process.env.HOME, '.pi/agent/mcp.json');
  const data = fs.existsSync(cfg) ? JSON.parse(fs.readFileSync(cfg, 'utf8')) : { mcpServers: {} };
  data.mcpServers ||= {};
  data.mcpServers.kubernetes = { command: 'npx', args: ['-y', 'kubernetes-mcp-server@latest'] };
  data.mcpServers.terraform = { command: 'npx', args: ['-y', 'terraform-mcp-server@latest'] };
  data.mcpServers.helm = { command: 'npx', args: ['-y', 'helm-mcp@latest'] };
  data.mcpServers['aws-docs'] = { command: 'npx', args: ['-y', '@awslabs/mcp-aws-docs@latest'] };
  fs.writeFileSync(cfg, JSON.stringify(data, null, 2));
  JS

  # 3. Permission gates — HARD-blocks destructive verbs
  pi install npm:pi-hooks
  pi install git:github.com/rytswd/pi-agent-extensions  # permission-gate
  # Suggested archetype: configure blocklist for kubectl delete, terraform destroy
  cat > ~/.pi/agent/permission-gate.config.json <<'JSON'
  {
    "blocklist": [
      "kubectl\\s+delete",
      "terraform\\s+destroy",
      "helm\\s+uninstall",
      "aws\\s+.*\\s+delete",
      "rm\\s+-rf\\s+/"
    ]
  }
  JSON

  # 4. SSH-remote for jump-host'd cluster ops
  pi install git:github.com/cv/pi-ssh-remote

  # 5. Background-jobs for kubectl logs -f, port-forwards
  # gob is brew-only on host; document the requirement
  echo "NOTE: install 'gob' on the host (brew install juanibiapina/taps/gob) for background job mgmt"

  # 6. Skills — commit, github, update-changelog
  pi install git:github.com/mitsuhiko/agent-stuff

  # 7. Notifications for long-running ops
  pi install npm:pi-notify

source: https://github.com/rytswd/pi-agent-extensions
---

# Profile: DevOps (Kubernetes / Terraform)

Pre-curated Pi stack for SREs and platform engineers. The defining
characteristic of devops work is **enormous blast radius** — one wrong
`kubectl delete` can take down production. This profile is paranoid by
default.

## What's installed

| Layer            | Package                                              |
|------------------|------------------------------------------------------|
| MCP bridge       | `pi-mcp-adapter`                                     |
| K8s (MCP)        | `kubernetes-mcp-server`                              |
| Terraform (MCP)  | `terraform-mcp-server`                               |
| Helm (MCP)       | `helm-mcp`                                           |
| AWS docs (MCP)   | `@awslabs/mcp-aws-docs`                              |
| Hooks            | `pi-hooks` + `rytswd/permission-gate`                |
| SSH bridge       | `cv/pi-ssh-remote`                                   |
| Skills           | `agent-stuff` (commit, github, update-changelog)     |
| Notifications    | `pi-notify` (OSC 777 + iTerm2 / Kitty)               |

## Hard-blocked verbs

`permission-gate.config.json` blocks (case-insensitive):

- `kubectl delete`
- `terraform destroy`
- `helm uninstall`
- `aws ... delete`
- `rm -rf /`

The agent can still **plan** these — but execution always requires
explicit user confirmation. Extend `~/.pi/agent/permission-gate.config.json`
to add your team's invariants.

## Why no `kubectl` MCP write tools

For the same blast-radius reason. The cluster MCP exposes get/list,
not delete/scale-to-0. If you need write ops, do them through a CLI
that `permission-gate` can intercept by name.

## SSH bridge

`cv/pi-ssh-remote` mounts a remote host's `cwd` via SSHFS and routes
bash through SSH. Pattern: run Pi locally, but operate on a jump host.

## Background jobs

`gob` (`brew install juanibiapina/taps/gob`) is the recommended
background-process manager for `kubectl logs -f`, port-forwards, etc.
Install on the host; the container reuses it via shared volumes when
configured.
