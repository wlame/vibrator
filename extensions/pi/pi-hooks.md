---
name: pi-hooks
kind: plugin
default: false
size_mb: 2
category: harness-specific
deps:
  features: [node]
install: |
  # pi-hooks by prateekmedia — bundle of six extensions that together
  # approximate Claude Code's hook + permission DSL.
  pi install npm:pi-hooks
source: https://github.com/prateekmedia/pi-hooks
---

# pi-hooks

Bundle of six Pi extensions by `prateekmedia` that together act as the
closest Pi equivalent of Claude Code's hook + permission system.

## What's inside

| Extension     | What it does                                                      |
|---------------|-------------------------------------------------------------------|
| `checkpoint`  | Git-ref snapshot per turn. Restore files-only, conv-only, or both |
| `lsp`         | Auto-diagnostics at end of turn + manual `def/refs/hover/symbols` |
| `permission`  | 4-level access control (off / low / medium / high) via `/permission` |
| `ralph-loop`  | Subagent loop with configurable iterations, delays, pause/resume  |
| `repeat`      | Searchable interface to replay bash commands, writes, or edits    |
| `token-rate`  | Output tokens-per-second display in footer                        |

## Why a bundle

Each one is useful on its own, but together they give vibrator users
the four classic Claude Code productivity features:

1. Checkpoint / rollback (`checkpoint`)
2. In-editor diagnostics (`lsp`)
3. Confirmation gates for dangerous ops (`permission`)
4. Loop-until-done (`ralph-loop`)

## LSP setup

`lsp` does not install language servers — you do. Common installs:

```bash
# TypeScript / JS
npm install -g typescript typescript-language-server

# Python
pip install python-lsp-server==1.14.0 ruff==0.15.16 python-lsp-ruff==2.3.1

# Go
go install golang.org/x/tools/gopls@v0.22.0
```

`lsp` auto-detects which server to invoke from the file extension.

## Conflicts

If you also enable `pi-rewind-hook` (also in this catalogue), both will
create snapshots — pick one. `pi-rewind-hook` has a more elegant
single-ref-store design, but `pi-hooks/checkpoint` is bundled with the
rest and more discoverable.
