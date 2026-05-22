---
name: pi-skills (Mario's pack)
kind: plugin
default: false
size_mb: 2
category: harness-specific
deps:
  features: [node]
install: |
  # pi-skills by badlogic (Mario Zechner, Pi's creator). The semi-
  # official Pi skill pack — Mario's curated stack of search, browser,
  # and Google API helpers.
  pi install git:github.com/badlogic/pi-skills
source: https://github.com/badlogic/pi-skills
---

# pi-skills (Mario's pack)

Skill bundle by `badlogic` (Mario Zechner) — Pi's creator — and as
close to an "official skill pack" as Pi gets. Eight skills, mostly
focused on web access and Google integration.

## Skills included

| Skill                | What it does                                            |
|----------------------|---------------------------------------------------------|
| `brave-search`       | Web search via Brave API                                |
| `browser-tools`      | Interactive browser automation via Chrome DevTools     |
| `gccli`              | Google Calendar CLI — events, availability             |
| `gdcli`              | Google Drive CLI — file mgmt, sharing                  |
| `gmcli`              | Gmail CLI — email, drafts, labels                      |
| `transcribe`         | Speech-to-text via Groq Whisper                         |
| `vscode`             | VS Code integration for diffs + file comparison        |
| `youtube-transcript` | Fetch YouTube video transcripts                         |

## Runtime needs

Some skills need external CLIs Mario also publishes:

```bash
# Required for gmcli / gccli / gdcli
npm install -g @mariozechner/gmcli @mariozechner/gccli @mariozechner/gdcli

# Required for transcribe
export GROQ_API_KEY=...

# Required for brave-search
export BRAVE_API_KEY=...
```

`browser-tools` needs a local Chrome and Node.js — generally already
present in the vibrator base image.

## Differs from Anthropic's skill spec

Pi's skill spec allows the `name` frontmatter field to differ from the
parent directory name. That means **the same skills work across Pi,
Claude Code, Codex, Amp, and Droid** if you point them at a shared
`.agents/skills/` folder. Mario's pack ships in Pi's preferred shape
but is compatible.

Default off — these add real surface area and most users only want a
subset. Pick this when you want Mario's "everyday stack" preinstalled.
