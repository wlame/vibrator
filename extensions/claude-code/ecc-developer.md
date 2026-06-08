---
name: ECC — Everything Claude Code (developer)
description: ECC developer profile — 63 agents + ~249 skills + rules/hooks into ~/.claude (heavy, opt-in)
kind: plugin
default: false
size_mb: 6
category: harness-specific
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  # Everything Claude Code (ECC) — affaan-m/ECC. A cross-harness bundle of
  # subagents, skills, rules, and hooks installed into ~/.claude via ECC's own
  # manifest-driven installer. The "developer" profile is ECC's default
  # engineering preset (rules + agents + commands + hooks + language/db/
  # orchestration modules).
  #
  # Pinned to a reviewed commit for reproducibility + supply-chain hygiene.
  # Bump deliberately (single find-and-replace across the ecc-* entries) —
  # upstream is a fast-moving release-candidate line. See
  # .claude/plans/ecc-integration-plan.md.
  ECC_REF=99baa8250096f2d295583572399a5c9aba2ce312

  # Shallow-fetch EXACTLY the pinned commit. init+fetch+checkout (rather than
  # `clone --depth 1`) so the pin keeps working after upstream advances past
  # this SHA — GitHub serves any reachable commit to a depth-1 fetch.
  mkdir -p /tmp/ecc
  cd /tmp/ecc
  git init -q
  git remote add origin https://github.com/affaan-m/ECC.git
  git fetch -q --depth 1 origin "$ECC_REF"
  git checkout -q FETCH_HEAD

  # ECC's installer needs only three small runtime deps (ajv, @iarna/toml,
  # sql.js). --omit=dev keeps it lean; the claude target never needs the
  # TypeScript build step (that's opencode-only).
  npm install --no-audit --no-fund --omit=dev --loglevel=error

  # Drive ECC's own installer. --target claude writes into ~/.claude with
  # ECC-managed content namespaced under rules/ecc/ and skills/ecc/ (so it
  # never clobbers user content) and records an install-state.json.
  node scripts/install-apply.js --target claude --profile developer

  # Drop the clone + node_modules from the image layer — only ~/.claude
  # content (~6 MB) needs to persist.
  cd /
  rm -rf /tmp/ecc
source: https://github.com/affaan-m/ECC
---

# ECC — Everything Claude Code (developer profile)

[ECC](https://github.com/affaan-m/ECC) ("Everything Claude Code", MIT) is a
cross-harness bundle of agent capabilities, installed into `~/.claude` by ECC's
own manifest-driven installer. The **developer** profile is ECC's default
engineering preset.

## What gets installed

Into `~/.claude/` (ECC content namespaced under `rules/ecc/`, `skills/ecc/`):

| Component | Count | What it is |
|---|---|---|
| `agents/`   | 63  | Specialised subagents (planner, code-reviewer, security-reviewer, per-language reviewers, build-error-resolver, e2e-runner, …) |
| `skills/`   | ~136 (developer subset) | Workflow definitions — the canonical surface (`/tdd`, `/refactor-clean`, research-ops, …) |
| `commands/` | 79  | Legacy slash-command shims (ECC is migrating these into skills) |
| `rules/`    | 115 | `common/` always-on principles + per-language packs |
| `hooks/`    | 4   | Event automations (PreToolUse/PostToolUse/Stop/…) |

Approx. **5.5 MB** installed (the developer profile). `core` is lighter (~4 MB),
`full` heavier (~7.4 MB, ~249 skills) — see the sibling `ecc-*` entries.

## Why opt-in (read before enabling)

ECC is **powerful but heavy on agent context** — the developer profile lands
~136 skills + 63 agents in `~/.claude`, all discoverable by Claude Code. That is
a lot of surface for the model to consider. Enable it when you want ECC's
opinionated, security-first, research-first workflow scaffolding; skip it for a
lean session. It also overlaps conceptually with `superpowers` / `superclaude` —
pick one workflow framework rather than stacking several.

## Profiles

This entry installs ECC's **developer** profile. Other profiles are available as
separate extensions so you consciously pick the context cost you want:

- `ecc-minimal` — rules + agents + commands, no hook runtime (lightest)
- `ecc-core` — lean baseline (rules + agents + commands + hooks + platform + quality)
- **`ecc-developer`** — default engineering preset *(this entry)*
- `ecc-security` — core + security agents/guidance
- `ecc-research` — core + research/content workflows
- `ecc-full` — everything (~249 skills; heaviest context)

## Reproducibility

Pinned to commit `99baa82` of `affaan-m/ECC`. The install shallow-fetches exactly
that commit, runs `npm install --omit=dev`, then `install-apply.js --target
claude --profile developer`. Bump the pin deliberately across all `ecc-*` entries.

## Source

<https://github.com/affaan-m/ECC>
