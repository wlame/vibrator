# The ECC bundle

[ECC](https://github.com/affaan-m/ECC) — "Everything Claude Code" (MIT) — is a
cross-harness bundle of subagents, skills, rules, and hooks. Vibrator ships it as a family
of opt-in [extensions](extensions.md), one per ECC profile, each named `ecc-*`.

When selected, an `ecc-*` extension's build-time install snippet shallow-fetches a
**pinned ECC commit** and runs ECC's own manifest-driven installer into the harness-native
config directory (`~/.claude`, `~/.codex`, or `~/.opencode`).

## Profiles

ECC profiles trade capability against agent-context cost. **None is on by default** — you
opt in consciously.

| Profile | What it is | claude-code | codex | opencode |
|---------|-----------|:---:|:---:|:---:|
| `ecc-minimal` | lightest, no hook runtime | ✓ | — | ✓ |
| `ecc-core` | lean baseline | ✓ | ✓ | ✓ |
| `ecc-developer` | default engineering preset **(recommended)** | ✓ | ✓ | ✓ |
| `ecc-security` | core + security module | ✓ | ✓ | ✓ |
| `ecc-research` | core + research/content | ✓ | ✓ | ✓ |
| `ecc-full` | everything (heaviest context) | ✓ | ✓ | ✓ |

```bash
vibrate --harness=claude-code --extensions=ecc-developer
vibrate extensions show ecc-developer        # full docs for any profile
```

!!! note "Harness coverage"
    - **Codex** has no `ecc-minimal` — with the hook runtime skipped, minimal and core
      resolve to the same install on Codex, so only `ecc-core` is offered.
    - **Pi** is not supported: ECC ships no `pi` adapter, so there are no `ecc-*` entries
      for the Pi harness.

## How it's installed

The install snippet (visible via `vibrate extensions show <id>`) does, roughly:

```bash
ECC_REF=<pinned-commit>                    # same SHA across all ecc-* entries
git init -q
git remote add origin https://github.com/affaan-m/ECC.git
git fetch -q --depth 1 origin "$ECC_REF"   # shallow-fetch exactly the pinned commit
git checkout -q FETCH_HEAD
npm install --no-audit --no-fund --omit=dev
node scripts/install-apply.js --target <harness> --profile <profile>
```

Because ECC's installer needs Node, every `ecc-*` entry declares `deps.features: [node]`,
which [auto-enables](profiles-and-features.md#how-resolution-works) the `node` feature.

## Pinning

Every `ecc-*` entry pins the **same** ECC commit for reproducibility and supply-chain
hygiene. Bumping it is a deliberate, single find-and-replace of `ECC_REF=` across all
`extensions/*/ecc-*.md` files — a consistency test fails the build if the pins ever drift
apart.

## Context cost

ECC profiles install a lot of agents, skills, and rules (`ecc-developer` is ~63 agents +
~249 skills; `ecc-full` is heavier still). That capability comes at the cost of agent
context budget, so pick the smallest profile that covers your workflow. The wizard shows an
"about" blurb for the focused entry to help you choose.

## Related

- [Extensions](extensions.md) — the catalogue mechanics ECC plugs into.
- [`vibrate extensions show`](../reference/commands/extensions.md#vibrate-extensions-show) —
  read any `ecc-*` entry's full docs and install snippet.
