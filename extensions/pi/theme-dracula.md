---
name: Dracula theme for Pi
kind: tool
default: false
size_mb: 0
category: harness-specific
install: |
  # Drops a Pi theme file at ~/.pi/agent/themes/dracula.json.
  # Pi's theme system requires 51 specific tokens; we ship the canonical
  # Dracula palette mapped onto each one. Activate via /theme dracula
  # or "theme": "dracula" in settings.json.
  mkdir -p ~/.pi/agent/themes
  cat > ~/.pi/agent/themes/dracula.json <<'JSON'
  {
    "$schema": "https://pi.dev/schemas/theme.json",
    "name": "dracula",
    "vars": {
      "background":  "#282a36",
      "currentLine": "#44475a",
      "selection":   "#44475a",
      "foreground":  "#f8f8f2",
      "comment":     "#6272a4",
      "cyan":        "#8be9fd",
      "green":       "#50fa7b",
      "orange":      "#ffb86c",
      "pink":        "#ff79c6",
      "purple":      "#bd93f9",
      "red":         "#ff5555",
      "yellow":      "#f1fa8c"
    },
    "colors": {
      "ui.background":         "$background",
      "ui.foreground":         "$foreground",
      "ui.border":             "$comment",
      "ui.cursor":             "$pink",
      "ui.selection":          "$selection",
      "ui.statusbar.bg":       "$currentLine",
      "ui.statusbar.fg":       "$foreground",
      "ui.statusbar.accent":   "$purple",
      "footer.bg":             "$currentLine",
      "footer.fg":             "$foreground",
      "header.bg":             "$currentLine",
      "header.fg":             "$cyan",
      "markdown.heading":      "$purple",
      "markdown.bold":         "$pink",
      "markdown.italic":       "$yellow",
      "markdown.code":         "$green",
      "markdown.codeblock":    "$green",
      "markdown.link":         "$cyan",
      "markdown.list":         "$orange",
      "markdown.quote":        "$comment",
      "markdown.hr":           "$comment",
      "tool.read":             "$cyan",
      "tool.write":            "$green",
      "tool.edit":             "$orange",
      "tool.bash":             "$pink",
      "tool.search":           "$purple",
      "tool.error":            "$red",
      "tool.success":          "$green",
      "tool.diff.added":       "$green",
      "tool.diff.removed":     "$red",
      "tool.diff.context":     "$comment",
      "syntax.keyword":        "$pink",
      "syntax.string":         "$yellow",
      "syntax.number":         "$purple",
      "syntax.comment":        "$comment",
      "syntax.function":       "$green",
      "syntax.class":          "$cyan",
      "syntax.variable":       "$foreground",
      "syntax.type":           "$cyan",
      "syntax.operator":       "$pink",
      "syntax.punctuation":    "$foreground",
      "thinking.low":          "$comment",
      "thinking.medium":       "$purple",
      "thinking.high":         "$pink",
      "bash.prompt":           "$green",
      "bash.command":          "$foreground",
      "bash.output":           "$foreground",
      "bash.error":            "$red",
      "spinner":               "$purple",
      "model.label":           "$cyan",
      "user.message":          "$foreground",
      "assistant.message":     "$foreground",
      "system.message":        "$comment"
    }
  }
  JSON
  echo "Dracula theme installed. Activate with: /theme dracula"
source: https://draculatheme.com/
---

# Dracula theme for Pi

The Dracula palette mapped onto Pi's 51 required theme tokens. Drops a
JSON file at `~/.pi/agent/themes/dracula.json`. Activate with:

```
/theme dracula
```

Or set permanently in `~/.pi/agent/settings.json`:

```json
{ "theme": "dracula" }
```

Hot-reloads via `/reload` if you tweak the JSON.

## Palette

Standard Dracula:

- Background `#282a36`
- Current Line `#44475a`
- Foreground `#f8f8f2`
- Comment `#6272a4`
- Cyan `#8be9fd`
- Green `#50fa7b`
- Orange `#ffb86c`
- Pink `#ff79c6`
- Purple `#bd93f9`
- Red `#ff5555`
- Yellow `#f1fa8c`

## Color tokens covered

All 51 of Pi's required theme tokens are mapped:

- **UI**: background, foreground, border, cursor, selection, statusbar
- **Footer / Header**: bg, fg, accent
- **Markdown**: heading, bold, italic, code, codeblock, link, list,
  quote, hr
- **Tools**: read, write, edit, bash, search, error, success, diff
  added/removed/context
- **Syntax**: keyword, string, number, comment, function, class,
  variable, type, operator, punctuation
- **Thinking levels**: low / medium / high
- **Bash mode**: prompt, command, output, error
- **Misc**: spinner, model label, user/assistant/system messages

## Other themes worth installing

If you want something more polished out of the box, also check the
`@zenobius/pi-rose-pine` package — it ships Rose Pine, Rose Pine Moon,
and Rose Pine Dawn pre-mapped.

Default off; pick if you want Dracula across your tooling.
