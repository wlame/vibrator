---
name: Tokyo Night theme
kind: tool
default: false
size_mb: 0
category: harness-specific
install: |
  # Tokyo Night ships as a built-in OpenCode theme — no download needed.
  # We just point tui.json at it.
  mkdir -p "$HOME/.config/opencode"
  jq '. + {"$schema":"https://opencode.ai/tui.json","theme":"tokyonight"}' \
     "$HOME/.config/opencode/tui.json" 2>/dev/null \
     > "$HOME/.config/opencode/tui.json.tmp" \
     && mv "$HOME/.config/opencode/tui.json.tmp" "$HOME/.config/opencode/tui.json" \
     || echo '{"$schema":"https://opencode.ai/tui.json","theme":"tokyonight"}' > "$HOME/.config/opencode/tui.json"
source: https://opencode.ai/docs/themes/
---

# Tokyo Night theme

Tokyo Night — the popular cool-blue-and-purple dark scheme inspired by
Tokyo's downtown lights. Originally from the Neovim community
(`folke/tokyonight.nvim`), now a ubiquitous palette across editors.

OpenCode ships Tokyo Night as a **built-in theme**, so this extension
is a config-only install — it just flips the active theme to
`tokyonight` in `tui.json`. No theme file to download.

## Variants

If you want the lucent / translucent variant, install
`RazoBeckett/opencode-lucent-tokyonight-theme` instead — same palette
with terminal-transparency tweaks.

## Switch at runtime

```
/theme tokyonight
```

The list of built-in themes you can flip to also includes `opencode`,
`system`, `everforest`, `ayu`, `catppuccin`, `catppuccin-macchiato`,
`gruvbox`, `kanagawa`, `nord`, `matrix`, `one-dark`.

## Why off by default

OpenCode's default `opencode` theme is the neutral starting point —
users opt into specific palettes per taste.
