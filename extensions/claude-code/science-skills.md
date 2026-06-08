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
  # standard agent skills (SKILL.md + uv/Python scripts) for scientific research:
  # querying public genomics/proteomics/chemistry databases and searching the
  # scientific literature. NOT tied to Google Antigravity — these are the plain
  # agent-skill format, so Claude Code discovers them natively.
  #
  # Pinned to a reviewed commit for reproducibility; bump deliberately.
  SCI_REF=c633b1ef0a1b5f1127bfd3192d34577a859811b2

  # Shallow-fetch EXACTLY the pinned commit (survives upstream advancing).
  mkdir -p /tmp/sci "$HOME/.claude/skills"
  cd /tmp/sci
  git init -q
  git remote add origin https://github.com/google-deepmind/science-skills.git
  git fetch -q --depth 1 origin "$SCI_REF"
  git checkout -q FETCH_HEAD

  # Copy each skill directory into the harness skill dir. Claude Code discovers
  # ~/.claude/skills/<name>/SKILL.md. The skills are self-contained (scripts/,
  # references/) and cross-reference each other, so copy them as a set — including
  # the shared `uv` and `scienceskillscommon` helper skills they depend on.
  cp -r skills/* "$HOME/.claude/skills/"

  cd /
  rm -rf /tmp/sci
source: https://github.com/google-deepmind/science-skills
---

# Science Skills (Google DeepMind)

A bundle of **37 agent skills for scientific research** from Google DeepMind.
Each skill wraps a public science API behind rate-limited Python (`uv`) scripts
so the agent can query authoritative biological, chemical, and literature data
instead of guessing from training memory. These are the standard agent-skill
format (`SKILL.md`) — **not** dependent on Google Antigravity; Claude Code
discovers them natively under `~/.claude/skills/`.

## What it's for — example tasks

- "Find and download the experimental 3D structure of human hemoglobin."
- "What's known about this variant in ClinVar / gnomAD? Is it pathogenic?"
- "Search ChEMBL/PubChem for compounds active against this target."
- "Pull the UniProt record and InterPro domains for this protein."
- "Predict the regulatory effect of a non-coding variant (AlphaGenome)."
- "Search recent bioRxiv/PubMed/arXiv literature on a topic and summarise."

## What's included (37 skills)

| Area | Skills |
|---|---|
| Structural biology | PDB, AlphaFold DB, Foldseek, PyMOL, protein MSA, sequence-similarity search |
| Genomics & variants | AlphaGenome, ClinVar, dbSNP, gnomAD, GTEx, ENCODE cCREs, UCSC conservation/TFBS, UniBind, JASPAR, Ensembl |
| Proteins & pathways | UniProt, InterPro, Human Protein Atlas, STRING, Reactome, QuickGO, EMBL-EBI OLS, NCBI sequence fetch |
| Chemistry & drugs | ChEMBL, PubChem, Open Targets, openFDA, ClinicalTrials.gov |
| Literature search | PubMed, arXiv, bioRxiv, EuropePMC, OpenAlex |
| Helpers | `uv` (env setup), `scienceskillscommon`, `workflow_skill_creator` |

## Prerequisites & notes

- **`uv` / Python** — the scripts run via `uv`, which the `python` feature already
  installs in the image; the bundled `uv` skill self-heals if it's ever missing.
- **Outbound network** — skills call public API endpoints at runtime (and `uv`
  fetches Python deps on first use).
- **Optional API keys** — most skills need none. A few do or benefit from one:
  **AlphaGenome** and **OpenAlex** require a key; **ClinVar** is optional (higher
  rate limits). Provide them as env vars (e.g. `ALPHAGENOME_API_KEY`); the skill
  prompts and guides you the first time.
- **Licensing** — Apache-2.0 bundle; individual data sources carry their own usage
  terms (several skills prompt you to acknowledge them on first use).

## Source

<https://github.com/google-deepmind/science-skills>
