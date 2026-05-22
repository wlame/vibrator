---
name: Dracula theme
kind: tool
default: false
size_mb: 0
category: harness-specific
runtime_needs:
  outbound_net: true
install: |
  mkdir -p "$HOME/.config/opencode/themes"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL https://raw.githubusercontent.com/dracula/opencode/main/dracula.json \
         -o "$HOME/.config/opencode/themes/dracula.json"
  else
    wget -qO "$HOME/.config/opencode/themes/dracula.json" \
         https://raw.githubusercontent.com/dracula/opencode/main/dracula.json
  fi
  # Set as active theme in tui.json (jq-merge, idempotent).
  mkdir -p "$HOME/.config/opencode"
  jq '. + {"$schema":"https://opencode.ai/tui.json","theme":"dracula"}' \
     "$HOME/.config/opencode/tui.json" 2>/dev/null \
     > "$HOME/.config/opencode/tui.json.tmp" \
     && mv "$HOME/.config/opencode/tui.json.tmp" "$HOME/.config/opencode/tui.json" \
     || echo '{"$schema":"https://opencode.ai/tui.json","theme":"dracula"}' > "$HOME/.config/opencode/tui.json"
source: https://github.com/dracula/opencode
---

# Dracula theme

The official Dracula theme port for OpenCode — the iconic dark
purple-on-near-black palette with eye-catching pink, cyan, and yellow
accents. Same palette as Dracula for VS Code, vim, iTerm, etc.

## Install

Drops `dracula.json` into `~/.config/opencode/themes/` and sets it as
the active theme in `~/.config/opencode/tui.json`. Switch at runtime
with `/theme` if you want to flip back.

## Why off by default

Themes are personal. OpenCode's built-in `opencode` theme is a sane
neutral default; users opt into specific palettes.
