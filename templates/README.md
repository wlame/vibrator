# Container templates

Static files embedded into the vibrator binary at build time (via
`//go:embed templates` in `embedded.go`) and copied into the docker
build context at image-build time (by
`internal/dockerfile.PrepareBuildContext`).

These files end up *inside* the container image via `COPY` directives
emitted by the dockerfile generator. They never run on the host.

## Layout

```
templates/
  shells/      Shell-specific rc files (bashrc, zshrc, config.fish)
  scripts/    Shell-agnostic helpers (welcome banner, entrypoint, exec wrapper)
```

## Conventions

- **Cross-shell parity**: every UX feature (PS1 prefix, welcome banner,
  history, aliases) should be implemented for **all three shells**. Per-
  shell mechanics differ (zsh PS1 vs fish `fish_prompt`) but the
  user-visible result must match.
- **POSIX-safe scripts**: anything in `scripts/` may be sourced or
  invoked by any of the three shells, so use POSIX `sh` syntax
  (avoid bash-isms like `[[`, `${VAR:0:N}`, arrays). Bash-isms belong
  in `shells/bashrc` only.
- **No host-specific paths**: scripts must use `$HOME`, `$USER`, etc. —
  don't hard-code `/home/wlame` or any other host value. Templates are
  shared across host users.
- **Editable as plain files**: contributors should be able to `vim
  templates/shells/zshrc` and see the change in the next build.
