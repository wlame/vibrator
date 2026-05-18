# vibrator (Go rewrite, in progress)

A single-binary CLI that runs AI coding agents (Claude Code, Codex, OpenCode,
Pi) in isolated Docker containers per workspace — with declarative profile
and catalog configuration via a `.vb` file at the project root.

> **Status:** rewrite-in-progress on branch `pivot`. The previous bash
> implementation is preserved under [`previous-implementation/`](./previous-implementation)
> for design reference. See
> [docs/plans](https://github.com/wlame/vibrator) for the current plan.

## Quick start (when Phase 4 lands)

```bash
make build
sudo install build/vibrate /usr/local/bin/

cd ~/my-project
vibrate    # wizard → image build → container drops you into a shell
```

After the first run you'll have a `.vb` file pinning your choices. Subsequent
`vibrate` calls in the same workspace skip the wizard and jump straight in.

## Phase 1: Foundation (done)

- `go.mod` + cobra CLI scaffold under [`cmd/vibrate`](./cmd/vibrate) and
  [`internal/cli`](./internal/cli)
- [`internal/docker`](./internal/docker) — Client interface + CLI-backed impl +
  in-memory mock
- [`internal/runtime`](./internal/runtime) — auto-detect Docker socket across
  Docker Desktop, OrbStack, Colima, Rancher Desktop, Podman, native
- [`internal/workspace`](./internal/workspace) — variant fingerprint + image /
  container naming
- [`internal/config`](./internal/config) — `.vb` TOML loader/writer
- CI: `.github/workflows/ci.yml` runs `go vet`, `go test -race`, build, smoke

## Architectural decisions

See `docs/plans/i-want-to-pivod-functional-tower.md` (in `~/.claude/plans/` on
the author's machine; will move into-repo at `docs/plans/` later).

| Decision | Choice |
|---|---|
| Docker integration | Shell out to `docker` CLI |
| `.vb` format | TOML |
| Catalog format | Markdown + YAML frontmatter, one file per item |
| Harness extensibility | Built-in Go interface |
| TUI | charmbracelet/huh — wizard fills CLI-flag gaps only |
| Profiles | minimal / backend / frontend / full |

## Development

```bash
make test          # unit tests (no docker needed)
make lint          # go vet + golangci-lint (if installed)
make build         # writes ./build/vibrate
make integration   # real-docker tests (requires INTEGRATION=1)
```

## License

MIT — see [LICENSE](./LICENSE).
