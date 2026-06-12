# Runtime detection

Vibrator talks to Docker by shelling out to the `docker` CLI — there's no Docker SDK
dependency. To do that reliably across the many ways people run Docker on macOS and Linux,
it auto-detects the Docker socket. You can inspect what it found with:

```bash
vibrate runtime detect
```

```
Runtime: orbstack
Socket:  /Users/you/.orbstack/run/docker.sock
Source:  socket-scan
```

## Detection order

The socket is resolved by the first of these that succeeds:

1. **Explicit override** — the `--docker-socket=<path>` flag or the
   `VIBRATOR_DOCKER_SOCKET` environment variable.
2. **`$DOCKER_HOST`** — when it's a `unix://` socket (non-unix schemes like `tcp://` and
   `ssh://` are rejected).
3. **Active `docker context`** — the endpoint of `docker context inspect`.
4. **Well-known socket paths** — scanned in order (see below).
5. **Fallback** — `/var/run/docker.sock`.

## Recognized runtimes

| Runtime | Socket path probed |
|---------|--------------------|
| Docker Desktop | `~/.docker/run/docker.sock` |
| OrbStack | `~/.orbstack/run/docker.sock` |
| Colima | `~/.colima/<profile>/docker.sock` |
| Rancher Desktop | `~/.rd/docker.sock` |
| Podman | `~/.local/share/containers/podman/machine/podman.sock` |
| Native Linux | `/var/run/docker.sock` |

The runtime name is identified from the matched socket path. A custom path you supply
yourself is reported as `custom`.

## Colima profiles

Colima can run multiple VMs ("profiles"). Vibrator picks the profile from, in order, the
`--colima-profile=<name>` flag, the `COLIMA_PROFILE` environment variable, or `default`:

```bash
vibrate runtime detect --colima-profile=work
```

## VM-based runtimes and `--dind`

For VM-based runtimes (Colima, Rancher Desktop), the macOS-side socket is a proxy that
only listens on the host. When you use [`--dind`](../guides/docker-in-docker.md) to mount
the socket into a container, Vibrator mounts the daemon's own `/var/run/docker.sock` (which
the daemon, running inside the VM, resolves to its real socket) and the base image's
docker sudo wrapper handles group access.

## Overriding detection

If detection picks the wrong socket — unusual multi-runtime setups, remote daemons exposed
locally, etc. — force it:

```bash
# One-off:
vibrate runtime detect --docker-socket=/custom/path/docker.sock

# Persistent, for the whole session:
export VIBRATOR_DOCKER_SOCKET=/custom/path/docker.sock
```

## Related pages

- [`vibrate runtime detect`](../reference/commands/runtime.md) — the command reference.
- [Docker-in-Docker](../guides/docker-in-docker.md) — `--dind` and socket mounting.
- [Environment variables](../reference/environment-variables.md) — `VIBRATOR_DOCKER_SOCKET`,
  `DOCKER_HOST`, `COLIMA_PROFILE`.
- [Troubleshooting](../troubleshooting.md) — fixes for "no Docker runtime found" and DinD on
  VM-based runtimes.
