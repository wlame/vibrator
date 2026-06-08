---
name: Science Skills (Google DeepMind)
description: 37 science-research skills — genomics, proteins, chemistry, drug & literature databases
kind: skill
default: false
size_mb: 4
category: niche
deps:
  features: [python]
runtime_needs:
  outbound_net: true
install: |
  # Science Skills — google-deepmind/science-skills (Apache-2.0). A bundle of 37
  # standard agent skills (SKILL.md + uv/Python scripts) for scientific research.
  # NOT tied to Google Antigravity — plain agent-skill format, copied into
  # OpenCode's skill dir. See claude-code/science-skills for the full overview.
  #
  # Pinned to a reviewed commit for reproducibility; bump deliberately.
  SCI_REF=c633b1ef0a1b5f1127bfd3192d34577a859811b2

  # Shallow-fetch EXACTLY the pinned commit (survives upstream advancing).
  mkdir -p /tmp/sci "$HOME/.opencode/skills"
  cd /tmp/sci
  git init -q
  git remote add origin https://github.com/google-deepmind/science-skills.git
  git fetch -q --depth 1 origin "$SCI_REF"
  git checkout -q FETCH_HEAD

  # Copy each self-contained skill dir into OpenCode's skill dir as a set
  # (incl. the shared `uv` + scienceskillscommon helpers they depend on).
  cp -r skills/* "$HOME/.opencode/skills/"

  cd /
  rm -rf /tmp/sci
source: https://github.com/google-deepmind/science-skills
---

# Science Skills (Google DeepMind) — OpenCode

The Google DeepMind [Science Skills](https://github.com/google-deepmind/science-skills)
bundle for the OpenCode harness — copied into `~/.opencode/skills/`. See
`claude-code/science-skills` for the full overview, task examples, and the list
of all 37 skills.

## In one line

37 agent skills that let the model query authoritative scientific databases
(structural biology, genomics/variants, proteins/pathways, chemistry/drugs) and
search the scientific literature (PubMed, arXiv, bioRxiv, EuropePMC, OpenAlex) —
via rate-limited Python (`uv`) scripts rather than training-data guesses. These
are standard `SKILL.md` skills, **not** Antigravity-specific.

## Prerequisites & notes

- **`uv` / Python** — provided by the `python` feature; the bundled `uv` skill
  self-heals if missing.
- **Outbound network** — skills call public APIs at runtime.
- **Optional API keys** — most need none; **AlphaGenome** and **OpenAlex** require
  a key, **ClinVar** benefits from one (higher rate limits). Provide as env vars.
- **Licensing** — Apache-2.0; individual data sources carry their own terms.

## Source

<https://github.com/google-deepmind/science-skills>
