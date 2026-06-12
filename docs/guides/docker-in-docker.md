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

The `docker` client binary (not a daemon) is **always baked into the base image** of every
variant and profile, alongside the other always-on tools — there's no feature to install. So
`--dind` is a purely **run-time** decision; it does not change the image at all:

1. **Mounts the host Docker socket** at `/var/run/docker.sock` (the path every `docker` CLI
   looks for by default, so no `DOCKER_HOST` needed inside). Without `--dind` the `docker`
   client is still present, it just has no socket to talk to — commands fail to connect,
   which is expected.
2. **Adds the container user to the socket's group** (`--group-add`) so `docker` works
   without `sudo` — where the runtime allows it.

Because the image content is identical with or without `--dind`, **toggling it never
rebuilds the image**. Running `vibrate --dind` on a workspace whose container was created
without it simply **recreates the container** from the existing image (fast — seconds) with
the socket mounted. Dropping `--dind` again likewise just recreates the container.

The socket is discovered the same way as [runtime detection](../lifecycle/runtime-detection.md):
`$DOCKER_HOST` first, then the well-known per-runtime paths.

## VM-based runtimes

On Colima and Rancher Desktop the macOS-side socket is a host-only proxy that containers
(running inside the Linux VM) can't reach. For these, Vibrator mounts the daemon's own
`/var/run/docker.sock` instead, and the base image's docker **sudo wrapper** handles
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

The `docker` client is already in every image, so there's nothing to bake in — just pass
`--dind` on the runs where you actually need socket access. Since toggling the flag only
recreates the container (never rebuilds), there's no cost to enabling it only when needed.

## Related pages

- [Runtime detection](../lifecycle/runtime-detection.md) — how the socket is found.
- [`vibrate` flags](../reference/commands/launch.md#options) — `--dind`.
