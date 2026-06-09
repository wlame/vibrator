---
name: Terraform MCP
kind: mcp
default: false
size_mb: 30
category: cloud-infrastructure
host_aliases: [terraform]
deps:
  features: [go]
runtime_needs:
  outbound_net: true
install: |
  # HashiCorp's official Terraform MCP server, written in Go. Built
  # from source so we get a static binary on the image's PATH; the
  # Go toolchain is already cached when the `go` feature is on, so
  # this adds about 30 MB after the binary is stripped.
  GOBIN=/usr/local/bin go install github.com/hashicorp/terraform-mcp-server/cmd/terraform-mcp-server@v0.5.2
  claude mcp add terraform \
    --scope user \
    --transport stdio \
    -- terraform-mcp-server stdio
source: https://github.com/hashicorp/terraform-mcp-server
---

# Terraform MCP

HashiCorp's official Terraform MCP server. Query the Terraform Registry
in real time for module signatures, resource attribute schemas, and
provider documentation. Pairs naturally with HCP Terraform and
Terraform Enterprise workspaces for plan / apply orchestration.

## Why use it

Without this MCP, Claude often hallucinates Terraform resource
arguments based on stale training-data snapshots — provider schemas
churn quickly. With it, the model checks the live Registry before
proposing config, which materially improves first-pass correctness on
infra changes.

## When to enable

Active Terraform / OpenTofu work. Skip if your infra-as-code stack is
something else (CDK, Pulumi, raw kubernetes manifests). For AWS
specifically, layer `aws-docs-mcp` alongside this — they're
complementary.

## HCP Terraform integration

Set `TFC_TOKEN` (or `TFE_TOKEN` for Terraform Enterprise) in your env
if you want the server to read workspace state, runs, and
configurations from your HCP org. Without a token, the server only
serves Registry data — still useful, just narrower.
