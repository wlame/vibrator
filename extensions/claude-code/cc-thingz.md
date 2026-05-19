---
name: cc-thingz (umputun bundle)
kind: plugin
default: false
size_mb: 5
install: |
  # umputun/cc-thingz is a multi-skill marketplace. Installing it
  # registers seven plugins at once: brainstorm, review, planning,
  # release-tools, thinking-tools, skill-eval, workflow.
  mkdir -p "$HOME/.claude/plugins/marketplaces"
  git clone --depth 1 https://github.com/umputun/cc-thingz.git \
    "$HOME/.claude/plugins/marketplaces/umputun-cc-thingz"
  # The vibrate harness scaffolding wires the marketplace into Claude's
  # registry — see install-cc-thingz pattern (Phase 3, derived from
  # previous-implementation/templates/install-cc-thingz.sh).
source: https://github.com/umputun/cc-thingz
---

# cc-thingz (umputun)

A curated bundle of seven Claude Code plugins by [umputun]:

| Plugin | Surface | Purpose |
|---|---|---|
| **brainstorm** | `/brainstorm` skill | Turn ideas into designs via Socratic dialogue before any implementation. |
| **planning** | `/planning:make`, `/planning:exec` | Phased implementation plans + subagent execution loop. |
| **review** | `/review:pr`, `/review:git-review`, `/review:writing-style` | PR review, interactive git-diff annotation, technical writing style. |
| **release-tools** | `/last-tag`, `/new` (release) | Semantic versioning + auto-release notes. |
| **thinking-tools** | `/thinking-tools:ask-codex`, `dialectic`, `root-cause-investigator` | Cross-model second opinions + 5-Why investigation. |
| **skill-eval** | (skill QA tooling) | Evaluate other skills against a corpus. |
| **workflow** | `/clarify`, `/wrong`, `/learn`, `/md-copy`, `/txt-copy` | Conversational meta-tools — pivot mid-session, capture learnings. |

[umputun]: https://github.com/umputun

## When to enable

If you want richer workflow tooling without picking individual plugins.
Heavy bundle — disable individual sub-plugins from `~/.claude/settings.json`
post-install if you only want some.

## Conflict with individual plugins

If you also install some of these as standalone marketplace entries, the
last install wins. The vibrator install runs ordered (alphabetical by
marketplace ID), so cc-thingz comes after Anthropic official — meaning
upstream conflicts surface there.
