# Docker-in-Docker

Sometimes the agent needs to run `docker` itself — build images, run test containers,
inspect the daemon. The `--dind` flag wires the container up to drive your **host** Docker
daemon.

```bash
vibrate --dind
```

!!! warning "This grants host-root-equivalent access"
    Mounting the host's Docker socket lets anything in the container run any `docker`
    command — including `docker run --privileged`. That's effectively root on your host.
    `--dind` is opt-in for exactly this reason. Only use it for workspaces you trust.

## What `--dind` does

1. **Auto-installs the `docker-cli` feature** — the container gets a `docker` client binary
   (not a daemon). You can still override with `--no=docker-cli` if you supply your own.
2. **Mounts the host Docker socket** at `/var/run/docker.sock` (the path every `docker` CLI
   looks for by default, so no `DOCKER_HOST` needed inside).
3. **Adds the container user to the socket's group** (`--group-add`) so `docker` works
   without `sudo` — where the runtime allows it.

The socket is discovered the same way as [runtime detection](../lifecycle/runtime-detection.md):
`$DOCKER_HOST` first, then the well-known per-runtime paths.

## VM-based runtimes

On Colima and Rancher Desktop the macOS-side socket is a host-only proxy that containers
(running inside the Linux VM) can't reach. For these, Vibrator mounts the daemon's own
`/var/run/docker.sock` instead, and the `docker-cli` feature's **sudo wrapper** handles
group access — `docker` in the container is a tiny shim that runs `sudo /usr/bin/docker`,
and the container user has passwordless sudo inside the sandbox.

## Verifying

```bash
vibrate --dind shell
# inside the container:
docker ps          # lists the host's containers
docker version     # client + server (the host daemon)
```

## Persisting it

To make a workspace always use DinD, the cleanest path is the auto-installed feature plus
the flag each run. You can also pin the client binary by adding the feature explicitly:

```bash
vibrate --with=docker-cli   # bake the client in
# then run with --dind when you actually need socket access
```

## Related pages

- [Runtime detection](../lifecycle/runtime-detection.md) — how the socket is found.
- [`docker-cli` feature](../reference/features.md) — the client binary + sudo wrapper.
- [`vibrate` flags](../reference/commands/launch.md#options) — `--dind`.
