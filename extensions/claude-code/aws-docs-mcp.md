---
name: AWS Documentation MCP
kind: mcp
default: false
size_mb: 15
category: documentation
host_aliases: [aws-docs]
deps:
  features: [python]
runtime_needs:
  outbound_net: true
install: |
  # awslabs publishes the AWS Documentation MCP server as a uvx
  # package. No auth required — it only reads public AWS docs.
  claude mcp add aws-docs \
    --scope user \
    --transport stdio \
    -- uvx awslabs.aws-documentation-mcp-server@1.1.24
source: https://github.com/awslabs/mcp
---

# AWS Documentation MCP

Read-only access to the public AWS documentation corpus. Search,
retrieve, and follow cross-references inside docs.aws.amazon.com —
without spamming the live AWS API or burning context on giant HTML
pages.

## Why use it

Same value prop as `context7` for JS / Python libraries, scoped to AWS:
the model gets accurate, current API surfaces (IAM policy attributes,
service quotas, CloudFormation resource types, SDK method signatures)
instead of training-data approximations that drift quickly as services
evolve.

## When to enable

Anyone doing serious AWS work — infrastructure changes, IAM policy
authoring, service integration, debugging "why is my Lambda timing
out". Useful enough that pairing with `terraform-mcp` covers most
AWS-via-IaC workflows end to end.

## Adjacent extensions

awslabs publishes a family of AWS MCP servers (`aws-api-mcp-server`,
`aws-iac-mcp-server`, `aws-pricing-mcp-server`, `aws-support-mcp-server`).
This entry is just the docs server — it has no AWS credential
requirement and is safe to enable in any context. The credentialed
servers warrant their own extension entries.

## No auth

Reads public docs only — no AWS keys, no IAM role, no outbound traffic
beyond docs.aws.amazon.com.
