# `vibrate runtime detect`

```bash
vibrate runtime detect [flags]
```

Inspects and prints the Docker runtime and socket path Vibrator will use. A read-only
diagnostic — it changes nothing. The full detection algorithm is documented in
[Runtime detection](../../lifecycle/runtime-detection.md).

## Output

```
Runtime: orbstack
Socket:  /Users/you/.orbstack/run/docker.sock
Source:  socket-scan
```

- **Runtime** — `docker-desktop`, `orbstack`, `colima`, `rancher-desktop`, `podman`,
  `native`, or `custom`.
- **Socket** — the absolute Unix socket path.
- **Source** — how it was found (`env-override`, `docker-host`, `docker-context`,
  `socket-scan`).

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--docker-socket=<path>` | — | Force this socket path (highest priority). Equivalent to `VIBRATOR_DOCKER_SOCKET`. |
| `--colima-profile=<name>` | `$COLIMA_PROFILE` or `default` | Colima VM profile to probe. |

## Examples

```bash
# What will vibrate use?
vibrate runtime detect

# Probe a specific Colima profile.
vibrate runtime detect --colima-profile=work

# Verify a manual override resolves.
vibrate runtime detect --docker-socket=/custom/path/docker.sock
```

## Related

- [Runtime detection](../../lifecycle/runtime-detection.md) — the detection order and the
  list of recognized runtimes.
- [Environment variables](../environment-variables.md) — `VIBRATOR_DOCKER_SOCKET`,
  `DOCKER_HOST`, `COLIMA_PROFILE`.
