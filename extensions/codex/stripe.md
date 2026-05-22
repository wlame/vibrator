---
name: Stripe MCP
kind: mcp
default: false
size_mb: 2
category: project-management
deps:
  features: [node]
auth:
  env: STRIPE_SECRET_KEY
runtime_needs:
  third_party_api: Stripe
  outbound_net: true
install: |
  # Stripe's official MCP server ships as @stripe/mcp on npm and reads
  # the API key as a CLI flag. We pass the env var through at launch
  # rather than baking the secret into config.toml.
  codex mcp add stripe -- npx -y @stripe/mcp --tools=all --api-key="$STRIPE_SECRET_KEY"
source: https://github.com/stripe/agent-toolkit
host_aliases: [stripe]
---

# Stripe MCP

Stripe's official MCP server. Create customers, products, payment
links, invoices; read charge/balance/payout data; manage subscriptions.
The `--tools=all` flag enables the full surface; narrow to specific
tools (e.g. `--tools=customers.read,products.create`) for tighter
sessions.

## Auth

Use a **Restricted API Key** (RAK), not your live secret key. Create
one at
[dashboard.stripe.com/apikeys](https://dashboard.stripe.com/apikeys)
and scope permissions to exactly what the agent needs (e.g.
`Customers: Read`, `Products: Write`).

Set `STRIPE_SECRET_KEY` on the host. Strongly prefer the **test mode**
key (`sk_test_...`) for any interactive session — switch to live only
for explicit, audited operations.

## Workflow examples

- "Find the customer with email foo@example.com and list their last 10
  charges"
- "Create a one-off product for $99 with a payment link"
- "Show all subscriptions cancelled in the last 24 hours"

## Risk note

Tool permissions are gated by the RAK, but a too-broad RAK is the most
common foot-gun. Audit the key's scopes before enabling Stripe MCP in
any session that the agent runs unattended.
