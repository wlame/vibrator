package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	gort "runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/dockerfile"
	"github.com/wlame/vibrator/internal/extensions"
	"github.com/wlame/vibrator/internal/harness"
	"github.com/wlame/vibrator/internal/integration"
	"github.com/wlame/vibrator/internal/localprovider"
	"github.com/wlame/vibrator/internal/prereq"
	"github.com/wlame/vibrator/internal/runtime"
	"github.com/wlame/vibrator/internal/workspace"
)

// buildImage generates the Dockerfile fresh and shells out to
// `docker build`. The Dockerfile is piped via stdin (-f -); the build
// context is a per-build tempdir populated by PrepareBuildContext
// (NOT the user's workspace — that mount happens at `docker run`
// time, not `docker build` time).
func buildImage(ctx context.Context, dc docker.Client,
	dfSpec dockerfile.Spec, imageTag string, opts Options,
) error {
	out, err := dockerfile.Generate(dfSpec)
	if err != nil {
		return fmt.Errorf("generate dockerfile: %w", err)
	}

	ctxDir, cleanup, err := dockerfile.PrepareBuildContext()
	if err != nil {
		return fmt.Errorf("prepare build context: %w", err)
	}
	defer cleanup()

	// Materialize the per-harness integrations manifest into the build
	// context. The dockerfile generator emits an unconditional COPY
	// for this file, so it must exist before `docker build`. See
	// internal/integration/manifest.go for the schema + buildcontext.go
	// for the writer's overall contract.
	if err := dockerfile.WriteIntegrationsManifest(ctxDir, dfSpec.Harness.ID()); err != nil {
		return fmt.Errorf("write integrations manifest: %w", err)
	}

	fmt.Fprintf(opts.Stderr, "→ Building image %s (no-cache=%v) ...\n", imageTag, opts.Rebuild)

	return dc.Build(ctx, docker.BuildSpec{
		DockerfileBytes: out,
		ContextDir:      ctxDir,
		Tag:             imageTag,
		NoCache:         opts.Rebuild,
		BuildArgs: map[string]string{
			"USERNAME": dfSpec.Username,
			"HOST_UID": fmt.Sprintf("%d", dfSpec.HostUID),
			"HOST_GID": fmt.Sprintf("%d", dfSpec.HostGID),
		},
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
	})
}

// runContainer translates a workspace + pin into a `docker run`
// invocation, mounts the workspace at the same absolute path, forwards
// auth + LLM env vars, and execs.
//
// `docker run` is INTERACTIVE here (-it) because the user is dropping
// into a shell session. When they exit, docker returns and we return
// normally.
//
// enabledExts is the list of resolved Extension entries (in the same
// order as pin.Extensions). Used to forward extension-declared
// `auth.env` host values into the container — without this plumbing,
// extensions like `codex-plugin-cc` that declare `auth.env:
// OPENAI_API_KEY` would silently fail at runtime because the host's
// env value never reaches the container.
func runContainer(ctx context.Context, dc docker.Client,
	imageTag, containerName, wsDir string,
	wsSpec workspace.Spec, pin config.Pin,
	enabledExts []*extensions.Entry, opts Options,
) error {
	wsHash := workspace.Fingerprint(wsSpec)

	envVars, err := buildContainerEnv(pin, enabledExts)
	if err != nil {
		return err
	}
	// Surface the workspace path to in-container scripts (welcome
	// banner, future entrypoint). Prepend so an explicit pin.Env or
	// auth-derived value can override.
	envVars = append([]docker.EnvVar{
		{Name: "WORKSPACE_PATH", Value: wsDir},
	}, envVars...)
	// claude-mem prereq → CLAUDE_MEM_* envs. Forwarded only when the
	// prereq has actually been bootstrapped for this workspace (the pin
	// carries the minted key + ids) AND the host has an admin config
	// (which carries the server URL + runtime mode). Without both, the
	// entrypoint's claude-mem block stays dormant.
	envVars = append(envVars, buildClaudeMemEnv(pin)...)

	labels := map[string]string{
		"vibrator.managed":   "true",
		"vibrator.harness":   pin.Harness,
		"vibrator.workspace": wsHash,
		"vibrator.path":      wsDir,
		// Records whether this container was created with the host docker
		// socket mounted (--dind). resolveAndLaunch reads it back to decide
		// whether a later invocation with a different --dind state must
		// recreate the container (the socket mount can't be added to a
		// live container).
		dindLabelKey: strconv.FormatBool(opts.DinD),
		// Fingerprint of the [identity] override this container was created
		// with (empty when none). Lets resolveAndLaunch recreate the
		// container when the alias changes — identity is injected at run
		// time, so a live container can't be retrofitted.
		identityLabelKey: identityFingerprint(pin),
	}

	// Workspace mount at the same absolute path on both sides — the
	// foundational mount that makes paths in error messages, stack
	// traces, and `pwd` match the user's host muscle memory.
	volumes := []docker.Volume{
		{Host: wsDir, Container: wsDir},
	}
	// Host config / auth / session state for the chosen harness. Each
	// harness declares its own mounts via Harness.HostMounts; the
	// orchestrator does the filesystem probing and conversion here. No
	// per-harness special-casing — adding host persistence to a new
	// harness is implementing the interface method, not editing app.
	if h, ok := harness.ByID(pin.Harness); ok {
		volumes = append(volumes, hostMountsToVolumes(h, defaultUsername(opts), wsDir, opts.Stderr)...)
	}
	// D6 + D7: optional/conditional mounts. Both auto-detect — no CLI
	// flag needed — and silently no-op when prerequisites are absent.
	volumes = append(volumes, buildOptionalMounts(defaultUsername(opts), pin, wsHash, opts.Stderr)...)
	// D9: GPG agent socket forwarding. Auto-detected via gpgconf — the
	// user opts in by configuring `extra-socket` in their gpg-agent.conf
	// (no extra-socket = no mount). Container-side C5 in entrypoint.sh
	// symlinks /gpg-agent-extra to wherever gpg expects.
	if v, ok := buildGPGAgentMount(); ok {
		volumes = append(volumes, v)
	}

	// D8: Docker-in-Docker. Opt-in via --dind because mounting the
	// host's docker socket is a security-sensitive choice (effectively
	// equivalent to host root once you can `docker run --privileged`).
	// When enabled, mount the auto-detected socket and pass the host
	// docker group's GID via --group-add so the unprivileged user can
	// actually use the socket without sudo.
	var dockerGroupAdd []string
	if opts.DinD {
		if v, gid, ok := buildDockerSocketMount(opts.Stderr); ok {
			volumes = append(volumes, v)
			if gid != "" {
				dockerGroupAdd = append(dockerGroupAdd, gid)
			}
		} else {
			fmt.Fprintf(opts.Stderr,
				"vibrate: warning: --dind requested but no docker socket found on host; continuing without DinD\n")
		}
	}

	fmt.Fprintf(opts.Stderr, "→ Creating container %s ...\n", containerName)

	// LoginMode: start detached with a long-running no-op CMD so the
	// entrypoint runs its full setup (config merge, rules, settings) before
	// we inject the auth login exec. The harness is launched separately via
	// execIntoContainer after the login step completes.
	if opts.LoginMode {
		return dc.Run(ctx, docker.RunSpec{
			Image:         imageTag,
			ContainerName: containerName,
			Hostname:      workspace.Hostname(wsDir),
			GroupAdd:      dockerGroupAdd,
			Detach:        true,
			Init:          true,
			ShmSize:       "2g",
			Volumes:       volumes,
			Env:           envVars,
			Labels:        labels,
			AddHosts:      []string{"host.docker.internal:host-gateway"},
			WorkingDir:    wsDir,
			Cmd:           []string{"sleep", "infinity"},
		})
	}

	// What to exec inside the new container: harness CLI (default) or
	// shell (vibrate shell). We pass an explicit Cmd to override the
	// image's CMD; the Dockerfile still bakes shell as CMD so a manual
	// `docker run <image>` from someone bypassing vibrate still gets a
	// sensible default.
	launchCmd, err := resolveLaunchCmd(pin, opts, nil)
	if err != nil {
		return err
	}

	return dc.Run(ctx, docker.RunSpec{
		Image:         imageTag,
		ContainerName: containerName,
		Hostname:      workspace.Hostname(wsDir),
		GroupAdd:      dockerGroupAdd,
		Interactive:   true,
		Cmd:           launchCmd,
		// F1: --init wires tini as PID 1 inside the container. Without
		// it, processes left behind by the user's shell (orphan node
		// servers, dead playwright children, MCP servers killed mid-
		// stream) accumulate as zombies — visible as <defunct> entries
		// in ps and a slow leak of process-table slots. tini reaps them
		// and forwards signals (Ctrl-C reaches the foreground shell
		// rather than being eaten by docker's default PID-1 shim).
		Init: true,
		// F2: docker's default /dev/shm is 64MB. Playwright / Chromium
		// crashes mid-run when shared memory fills up (the symptom is
		// a cryptic "Target.attachToBrowserTarget failed" on the first
		// page that allocates ~50MB of canvas). 2GB is comfortably
		// above any single-page workload.
		ShmSize: "2g",
		Volumes: volumes,
		Env:     envVars,
		Labels:  labels,
		// host network keeps host.docker.internal cheap and lets
		// in-container tools reach host services without --add-host.
		// We use bridge instead of host to keep Linux/macOS behavior
		// uniform; --add-host below patches in host.docker.internal.
		AddHosts: []string{"host.docker.internal:host-gateway"},
		// Land in the workspace, not the user's $HOME — the workspace
		// is mounted at the same absolute path on both sides, so this
		// mirrors what `cd <project>` on the host would put you in.
		WorkingDir: wsDir,
		Stdin:      opts.Stdin,
		Stdout:     opts.Stdout,
		Stderr:     opts.Stderr,
	})
}

// execIntoContainer runs the chosen launch target inside an already-
// running (or just-started) container. wsDir is the workspace path on
// the host (also the path inside the container, since vibrator mounts
// at the same absolute path) — used to set --workdir so re-entries
// land in the project, not the user's $HOME.
//
// The launch target (harness CLI vs shell) comes from opts.LaunchTarget.
// Both targets are wrapped with claude-exec so the session-start hooks
// (integration manifest probes, transport switching) fire on every
// re-entry, not just on first `docker run`.
func execIntoContainer(ctx context.Context, dc docker.Client,
	containerName, wsDir string, pin config.Pin, opts Options,
) error {
	cmd, err := resolveLaunchCmd(pin, opts, nil)
	if err != nil {
		return err
	}
	return dc.Exec(ctx, docker.ExecSpec{
		Container:   containerName,
		Interactive: true,
		WorkingDir:  wsDir,
		// WORKSPACE_PATH is set at original `docker run` time so it's
		// already in the container's env, but exec'd shells inherit
		// from the docker exec invocation, not from the run-time env.
		// Re-pass it here so the welcome banner shows the right path
		// on re-entry to an existing container.
		Env: []docker.EnvVar{
			{Name: "WORKSPACE_PATH", Value: wsDir},
		},
		Cmd:    cmd,
		Stdin:  opts.Stdin,
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
	})
}

// resolveLaunchCmd builds the argv that runs inside the container,
// based on the requested LaunchTarget:
//
//   - LaunchHarness (default): the harness's own CLI — claude, codex,
//     opencode, or pi — wrapped with claude-exec.
//   - LaunchShell:             the user's shell (pin.Shell), wrapped
//     with claude-exec.
//
// Both wrap with claude-exec so the integrations manifest is reprocessed
// on every session start — without it, host-side service changes (e.g.,
// starting the Serena host server between sessions) wouldn't get picked
// up by an exec'd harness.
//
// Returns an error only for the harness path, and only when the
// registered harness has no LaunchCommand (a programming bug — every
// harness must declare one).
func resolveLaunchCmd(pin config.Pin, opts Options, extraDirs []string) ([]string, error) {
	switch opts.LaunchTarget.effective() {
	case LaunchShell:
		shell := pin.Shell
		if shell == "" {
			shell = "zsh"
		}
		return []string{"/usr/local/bin/claude-exec", "/bin/" + shell}, nil

	case LaunchHarness:
		h, ok := harness.ByID(pin.Harness)
		if !ok {
			return nil, fmt.Errorf("harness %q not registered (build error?)", pin.Harness)
		}
		argv := h.LaunchCommand()
		if len(argv) == 0 {
			return nil, fmt.Errorf("harness %q declares no LaunchCommand", pin.Harness)
		}
		argv = append(argv, h.ExtraDirArgs(extraDirs)...)
		return append([]string{"/usr/local/bin/claude-exec"}, argv...), nil
	}

	// Unreachable — effective() normalizes "" to LaunchHarness.
	return nil, fmt.Errorf("unknown launch target %q", opts.LaunchTarget)
}

// hostMountsToVolumes resolves a harness's declarative HostMounts into
// concrete docker volumes. This is the one place that touches the
// filesystem on behalf of every harness: it expands each mount's
// host-relative path against the host home and its container-relative
// path against /home/<containerUser>, probes (or creates) the source per
// MountKind, and rejects any descriptor whose cleaned path escapes its
// home root.
//
// A harness with no host persistence returns no HostMounts and this
// yields no volumes — no special-casing anywhere.
func hostMountsToVolumes(h harness.Harness, containerUser, wsDir string, stderr io.Writer) []docker.Volume {
	var out []docker.Volume

	hostHome, err := os.UserHomeDir()
	if err != nil || hostHome == "" {
		return out
	}
	containerHome := "/home/" + containerUser

	for _, m := range h.HostMounts(harness.HostMountContext{WorkspaceDir: wsDir}) {
		hostPath, hostOK := joinUnderRoot(hostHome, m.HostRel)
		containerPath, ctrOK := joinUnderRoot(containerHome, m.ContainerRel)
		if !hostOK || !ctrOK {
			// A descriptor that escapes home is a programming bug in the
			// harness, not user input — skip it loudly rather than mount
			// something outside the intended root.
			fmt.Fprintf(stderr, "vibrate: warning: skipping host mount %q→%q for %s (path escapes home)\n",
				m.HostRel, m.ContainerRel, h.ID())
			continue
		}

		switch m.Kind {
		case harness.MountFileIfExists:
			if !isRegularFile(hostPath) {
				continue
			}
		case harness.MountDirIfExists:
			if !isDir(hostPath) {
				continue
			}
		case harness.MountDirEnsure:
			if !isDir(hostPath) {
				if err := os.MkdirAll(hostPath, 0o755); err != nil {
					// Non-fatal: a failed mkdir usually means odd host
					// perms, not a vibrator bug. Log and skip the mount.
					fmt.Fprintf(stderr, "vibrate: warning: couldn't create %s for session persistence: %v\n",
						hostPath, err)
					continue
				}
			}
		}

		out = append(out, docker.Volume{
			Host:      hostPath,
			Container: containerPath,
			ReadOnly:  m.ReadOnly,
		})
	}
	return out
}

// joinUnderRoot joins a forward-slash relative path onto an absolute
// root, returning (cleanedAbsolutePath, true) only when the result stays
// within root. A rel that climbs out via ".." yields ("", false). This
// is the guard that keeps a harness's HostMount descriptors from naming
// a path outside the user's (or container's) home dir.
func joinUnderRoot(root, rel string) (string, bool) {
	joined := filepath.Join(root, filepath.FromSlash(rel))
	cleaned := filepath.Clean(joined)
	if cleaned != root && !strings.HasPrefix(cleaned, root+string(os.PathSeparator)) {
		return "", false
	}
	return cleaned, true
}

// isRegularFile reports whether path exists and is a regular file (not a
// directory or socket). Same "any error = skip" semantics as isDir.
func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// buildOptionalMounts produces volumes that aren't tied to the claude
// state directory but are still useful when the corresponding host
// resource exists or the workspace's extensions ask for them:
//
//   - D6 (~/.aws → ~/.aws:ro): auto-mounts when the host has AWS
//     credentials. Read-only so a buggy container can't corrupt them.
//   - D7 (claude-mem per-workspace cache): only mounted when the
//     workspace's extensions include claude-mem. The cache lives at
//     ~/.cache/vibrator/claude-mem/<wsHash> on the host so each
//     workspace gets its own state — different workspaces don't share
//     vector embeddings, summaries, or temp files.
//
// Both are silent no-ops when their preconditions aren't met (no
// pin.Extensions claude-mem, no ~/.aws on host). `wsHash` scopes the
// claude-mem cache per workspace.
func buildOptionalMounts(containerUser string, pin config.Pin, wsHash string, stderr io.Writer) []docker.Volume {
	var out []docker.Volume

	hostHome, err := os.UserHomeDir()
	if err != nil || hostHome == "" {
		return out
	}
	containerHome := "/home/" + containerUser

	// D6: AWS credentials passthrough. Read-only — the container
	// inherits host creds but can't rotate or wipe them.
	if hostAws := filepath.Join(hostHome, ".aws"); isDir(hostAws) {
		out = append(out, docker.Volume{
			Host:      hostAws,
			Container: containerHome + "/.aws",
			ReadOnly:  true,
		})
	}

	// D6b: host global git config passthrough. Read-only — the container's
	// global git config IS the host's, so when the agent runs `git init`
	// and `git commit` in a fresh repo it commits with the user's real
	// name + email instead of a bogus container default. Only the standard
	// ~/.gitconfig is forwarded; git reads it for identity (user.name /
	// user.email) without needing to write to it.
	//
	// Suppressed when [identity] is set: the whole point of an alias is to
	// keep the real email off the wire, so we must NOT mount a host
	// gitconfig that carries it. Identity instead flows in via the
	// GIT_*/EMAIL env vars and the entrypoint's git-config write.
	identitySet := pin.Identity != nil && (pin.Identity.Email != "" || pin.Identity.Name != "")
	if hostGitconfig := filepath.Join(hostHome, ".gitconfig"); !identitySet && isRegularFile(hostGitconfig) {
		out = append(out, docker.Volume{
			Host:      hostGitconfig,
			Container: containerHome + "/.gitconfig",
			ReadOnly:  true,
		})
	}

	// D7: claude-mem per-workspace cache. Only when claude-mem is
	// actually selected for this workspace.
	if hasExtension(pin.Extensions, "claude-mem") {
		// ~/.cache/vibrator/claude-mem/<wsHash> on the host. Living
		// under ~/.cache (XDG base spec) means a `rm -rf ~/.cache` by
		// the user wipes vibrator state along with everything else —
		// the right behavior for a cache directory.
		hostCache := filepath.Join(hostHome, ".cache", "vibrator", "claude-mem", wsHash)
		if !isDir(hostCache) {
			if err := os.MkdirAll(hostCache, 0o755); err != nil {
				fmt.Fprintf(stderr, "vibrate: warning: couldn't create %s for claude-mem cache: %v\n",
					hostCache, err)
				return out
			}
		}
		out = append(out, docker.Volume{
			Host:      hostCache,
			Container: containerHome + "/.claude-mem/cache",
			ReadOnly:  false,
		})
	}

	return out
}

// hasExtension reports whether `id` is in the workspace's extensions
// list. Membership check — extensions IDs are case-sensitive and the
// list is small enough that a linear scan is fine.
func hasExtension(extensions []string, id string) bool {
	for _, e := range extensions {
		if e == id {
			return true
		}
	}
	return false
}

// socketGroupGID returns the owning group GID of a unix socket file,
// formatted as a decimal string for `--group-add`. Returns "" on any
// failure (stat error, unsupported syscall.Stat_t cast, etc.) — the
// caller treats empty as "couldn't determine, skip group-add".
//
// Implemented as a `syscall.Stat_t` cast which is darwin/linux only
// (the only platforms vibrator supports — Windows would need a
// different approach but isn't on the roadmap).
func socketGroupGID(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d", st.Gid)
}

// dockerSocketCandidates lists host paths to probe for a docker socket,
// in priority order. Earlier entries win. Covers the common
// Mac/Linux/desktop-VM setups; users with exotic configs can override
// via $DOCKER_HOST (handled first in buildDockerSocketMount).
//
//	Docker Desktop:   ~/.docker/run/docker.sock
//	OrbStack:         ~/.orbstack/run/docker.sock
//	Colima default:   ~/.colima/default/docker.sock
//	Rancher Desktop:  ~/.rd/docker.sock
//	Native Linux:     /var/run/docker.sock
//
// All are unix domain sockets; we stat the path to confirm it really
// IS a socket before mounting.
var dockerSocketCandidates = []string{
	"~/.docker/run/docker.sock",
	"~/.orbstack/run/docker.sock",
	"~/.colima/default/docker.sock",
	"~/.rd/docker.sock",
	"/var/run/docker.sock",
}

// buildDockerSocketMount discovers the host docker socket and returns
// a (mount, group GID, true) when one is reachable. The container side
// is always `/var/run/docker.sock` — that's what every docker CLI
// looks for by default, so the user doesn't need to set DOCKER_HOST
// inside the container.
//
// Returns ("", "", false) when no socket is found. Side-effect-free.
//
// The group GID lets the launch code add the container user to a
// group with the same GID as the socket's owning group on the host —
// the standard "let the unprivileged user talk to docker without
// sudo" pattern. Empty string means "couldn't determine, skip the
// group-add" (the user falls back to `sudo docker ...`).
func buildDockerSocketMount(stderr io.Writer) (docker.Volume, string, bool) {
	candidates := []string{}
	// $DOCKER_HOST takes top priority. Format is `unix:///path/to/sock`
	// so strip the prefix when present. Clean the path and require it
	// to be absolute — a relative or dot-segment path would make the
	// bind-mount source depend on the current working directory.
	if dh := os.Getenv("DOCKER_HOST"); strings.HasPrefix(dh, "unix://") {
		p := filepath.Clean(strings.TrimPrefix(dh, "unix://"))
		if filepath.IsAbs(p) {
			candidates = append(candidates, p)
		}
	}
	home, _ := os.UserHomeDir()
	for _, c := range dockerSocketCandidates {
		// Expand leading "~/" — Go's filepath functions don't do shell
		// expansion automatically, so we handle it here for the
		// `~/.docker/run/docker.sock` style entries.
		if strings.HasPrefix(c, "~/") && home != "" {
			c = filepath.Join(home, c[2:])
		}
		candidates = append(candidates, c)
	}

	for _, path := range candidates {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		// Confirm it's actually a socket, not a regular file or
		// directory that happens to share the name.
		if info.Mode()&os.ModeSocket == 0 {
			continue
		}
		gid := socketGroupGID(path)

		// For VM-based runtimes (Colima, Rancher Desktop), the macOS socket
		// path is a proxy socket that only listens on the macOS side. Containers
		// run inside the Linux VM and cannot connect to a socket that only exists
		// in the macOS address space. Use /var/run/docker.sock as the host path
		// instead — Docker daemon (inside the VM) resolves that path to its own
		// actual socket, so the bind mount lands the real socket in the container.
		// The base image's sudo wrapper for docker handles group access.
		mountHostPath := path
		if det, err := runtime.Detect(runtime.Options{}); err == nil {
			if det.Runtime == runtime.Colima || det.Runtime == runtime.RancherDesktop {
				mountHostPath = "/var/run/docker.sock"
				gid = "" // GID is irrelevant: docker uses a sudo wrapper
			}
		}

		return docker.Volume{
			Host:      mountHostPath,
			Container: "/var/run/docker.sock",
			ReadOnly:  false,
		}, gid, true
	}
	return docker.Volume{}, "", false
}

// buildGPGAgentMount probes the host for a forwardable gpg-agent socket
// and returns the bind mount when one is available.
//
// "Forwardable" here means the user has configured `extra-socket` in
// `~/.gnupg/gpg-agent.conf` — that's gpg's standard mechanism for
// exposing a socket safe to pass into a container or remote session.
// The path is reported by `gpgconf --list-dirs agent-extra-socket`.
//
// The CONTAINER side gets the socket at `/gpg-agent-extra` by
// convention. The entrypoint script (step C5) symlinks
// from there to wherever gpg-inside-container expects its socket — so
// `git commit -S`, `gpg --sign`, etc. all "just work" with the host
// key without that key ever leaving the host.
//
// Returns (_, false) when gpgconf isn't installed, when the user hasn't
// configured an extra-socket, or when the socket isn't actually present
// (gpg-agent not running). All three are normal "no GPG forwarding"
// states — never an error.
func buildGPGAgentMount() (docker.Volume, bool) {
	gpgconf, err := exec.LookPath("gpgconf")
	if err != nil {
		return docker.Volume{}, false
	}
	out, err := exec.Command(gpgconf, "--list-dirs", "agent-extra-socket").Output()
	if err != nil {
		return docker.Volume{}, false
	}
	socket := strings.TrimSpace(string(out))
	if socket == "" {
		return docker.Volume{}, false
	}
	// Stat to confirm the socket exists and IS a socket (gpgconf
	// reports the expected path even when the agent isn't running, so
	// we can't trust the output alone).
	info, err := os.Stat(socket)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return docker.Volume{}, false
	}
	return docker.Volume{
		Host:      socket,
		Container: "/gpg-agent-extra",
		ReadOnly:  false,
	}, true
}

// exists is a small file-presence helper. Treats every error as
// "doesn't exist" — for mount-construction, missing-vs-permission-
// denied is the same decision: skip.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isDir reports whether path is a directory. Same error semantics as
// exists — any stat failure means "skip this mount".
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// buildClaudeMemEnv produces the CLAUDE_MEM_* env vars that the
// container-side entrypoint (templates/scripts/entrypoint.sh step 8)
// uses to bootstrap claude-mem's runtime settings file and probe
// /healthz + /v1/events for auth status.
//
// Returns nil (no envs) when either the admin config is missing OR the
// pin has no cached bootstrap result — both are required because the
// admin config supplies the server URL while the bootstrap supplies the
// per-workspace API key. Without both we'd half-configure the plugin
// and confuse the user with cryptic 401s.
//
// The admin config's ServerURL is forwarded VERBATIM; it's already
// expected to be in container-shape (host.docker.internal:<port>) per
// the contract in prereq.ClaudeMemAdminConfig docs.
func buildClaudeMemEnv(pin config.Pin) []docker.EnvVar {
	cached, ok := pin.Prereqs[prereq.ClaudeMemPrereqID]
	if !ok || len(cached) == 0 {
		return nil
	}
	cfg, err := prereq.LoadClaudeMemAdminConfig()
	if err != nil || cfg == nil {
		return nil
	}
	if cfg.ServerURL == "" {
		return nil
	}

	envs := []docker.EnvVar{
		{Name: "CLAUDE_MEM_RUNTIME", Value: cfg.Runtime},
		{Name: "CLAUDE_MEM_SERVER_BETA_URL", Value: cfg.ServerURL},
	}
	// The bootstrap result keys match the field names the entrypoint
	// expects in env-var form (api_key → CLAUDE_MEM_SERVER_BETA_API_KEY,
	// etc.). We map them here rather than asking the entrypoint to do
	// case conversion in shell.
	if v := cached["api_key"]; v != "" {
		envs = append(envs, docker.EnvVar{Name: "CLAUDE_MEM_SERVER_BETA_API_KEY", Value: v})
	}
	if v := cached["team_id"]; v != "" {
		envs = append(envs, docker.EnvVar{Name: "CLAUDE_MEM_SERVER_BETA_TEAM_ID", Value: v})
	}
	if v := cached["project_id"]; v != "" {
		envs = append(envs, docker.EnvVar{Name: "CLAUDE_MEM_SERVER_BETA_PROJECT_ID", Value: v})
	}
	return envs
}

// claudeOAuthTokenFile is the conventional host path for storing a
// long-lived Claude OAuth token outside the shell
// environment. Vibrator reads it as a fallback when
// CLAUDE_CODE_OAUTH_TOKEN isn't already exported.
const claudeOAuthTokenFile = ".claude-docker-token"

// readOAuthTokenFile returns the trimmed contents of
// $HOME/.claude-docker-token, or "" on any error (missing file, perms,
// empty contents — all treated identically: "no token to forward").
//
// Whitespace is stripped because users often `echo "tok" > file`
// which adds a trailing newline that confuses Claude's auth.
func readOAuthTokenFile() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, claudeOAuthTokenFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// ─── --login machinery ───────────────────────────────────────────────────────

// entrypointReadyPath is the sentinel file the entrypoint touches just before
// exec'ing the user's command. waitForEntrypoint polls for it to avoid racing
// claude auth login against the config-merge steps (section 2 of entrypoint.sh).
const entrypointReadyPath = "/tmp/.vibrator-entrypoint-done"

// waitForEntrypoint polls the container until entrypoint.sh drops its
// readiness sentinel (or times out after ~5 s). Non-fatal on timeout —
// the login exec simply races the tail of the entrypoint, which is
// an unlikely and low-risk scenario in practice.
func waitForEntrypoint(ctx context.Context, dc docker.Client, containerName string) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		err := dc.Exec(ctx, docker.ExecSpec{
			Container: containerName,
			Cmd:       []string{"test", "-f", entrypointReadyPath},
			Stdout:    io.Discard,
			Stderr:    io.Discard,
		})
		if err == nil {
			return nil
		}
		// Sleep, but bail immediately if the caller cancels (Ctrl-C).
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out waiting for entrypoint ready signal in %s", containerName)
}

// claudeAuthURLMarker is the prefix claude auth login prints before the
// URL the user should open. We scan for this in the docker exec output.
const claudeAuthURLMarker = "If the browser didn't open, visit: "

// authURLWriter wraps an io.Writer and scans for the claude auth URL.
// When found, it opens it in the host browser (best-effort, non-blocking).
// All bytes pass through unchanged so the user still sees the full output.
type authURLWriter struct {
	w    io.Writer
	buf  []byte
	done bool
}

func (a *authURLWriter) Write(p []byte) (int, error) {
	n, err := a.w.Write(p)
	if a.done {
		return n, err
	}
	a.buf = append(a.buf, p...)
	if idx := bytes.Index(a.buf, []byte(claudeAuthURLMarker)); idx >= 0 {
		rest := a.buf[idx+len(claudeAuthURLMarker):]
		if end := bytes.IndexAny(rest, " \r\n\t"); end >= 0 {
			url := strings.TrimSpace(string(rest[:end]))
			if strings.HasPrefix(url, "https://") {
				a.done = true
				go openBrowser(url)
				a.buf = nil
			}
		}
	}
	// Prevent unbounded buffer growth while scanning.
	if len(a.buf) > 8192 {
		a.buf = a.buf[len(a.buf)-4096:]
	}
	return n, err
}

// openBrowser opens url in the host's default browser. Non-blocking and
// best-effort — a failure (no browser, wrong platform) is silently ignored
// because the URL is already visible in the terminal.
func openBrowser(url string) {
	var cmd string
	var args []string
	switch gort.GOOS {
	case "darwin":
		cmd, args = "open", []string{url}
	case "linux":
		cmd, args = "xdg-open", []string{url}
	case "windows":
		cmd, args = "cmd", []string{"/c", "start", url}
	default:
		return
	}
	_ = exec.Command(cmd, args...).Start()
}

// runLoginStep runs `claude auth login` interactively (stdin connected, no TTY
// so stdout stays a pipe Go can scan). It intercepts the OAuth URL and opens
// the host browser, then writes the resulting auth state back to the host's
// ~/.claude.json so future launches are pre-authenticated without --login.
//
// --login always runs the auth flow. We deliberately do NOT short-circuit when
// the host is already authenticated: passing --login is an explicit request to
// (re)authenticate — e.g. to switch accounts — so we honour it every time.
// Subsequent launches without --login still pick up the saved auth via the
// entrypoint's host→container config merge.
func runLoginStep(ctx context.Context, dc docker.Client, containerName, containerUser string, opts Options) error {
	fmt.Fprintln(opts.Stderr, "→ Running claude auth login (browser will open automatically)…")

	err := dc.Exec(ctx, docker.ExecSpec{
		Container:   containerName,
		Interactive: true,
		NoTTY:       true, // -i only; stdout is a pipe we can scan for the URL
		Cmd:         []string{"claude", "auth", "login"},
		Stdin:       opts.Stdin,
		Stdout:      &authURLWriter{w: opts.Stdout},
		Stderr:      opts.Stderr,
	})
	if err != nil {
		return fmt.Errorf("claude auth login: %w", err)
	}

	fmt.Fprintln(opts.Stderr, "→ Login complete — saving auth state to host ~/.claude.json…")
	if wbErr := writebackAuthToHost(ctx, dc, containerName, containerUser); wbErr != nil {
		// Non-fatal: auth still works for this session; it just won't persist.
		fmt.Fprintf(opts.Stderr, "⚠  auth writeback failed (auth works this session only): %v\n", wbErr)
	}
	return nil
}

// writebackAuthToHost reads the container's ~/.claude.json and merges the
// auth/onboarding fields back into the host's ~/.claude.json. This makes
// the login state persist across container recreations: on the next launch
// the entrypoint merges the host config into the container config (D1 mount),
// so no re-authentication is needed.
func writebackAuthToHost(ctx context.Context, dc docker.Client, containerName, containerUser string) error {
	hostHome, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	hostConfigPath := filepath.Join(hostHome, ".claude.json")

	// Read the container's ~/.claude.json.
	var containerBuf bytes.Buffer
	if err := dc.Exec(ctx, docker.ExecSpec{
		Container: containerName,
		Cmd:       []string{"cat", "/home/" + containerUser + "/.claude.json"},
		Stdout:    &containerBuf,
		Stderr:    io.Discard,
	}); err != nil {
		return fmt.Errorf("read container ~/.claude.json: %w", err)
	}

	var cConfig map[string]json.RawMessage
	if err := json.Unmarshal(containerBuf.Bytes(), &cConfig); err != nil {
		return fmt.Errorf("parse container ~/.claude.json: %w", err)
	}

	// Read (or initialise) the host config. A host file that exists but
	// fails to parse ABORTS the writeback: silently continuing would
	// rewrite ~/.claude.json with only the auth fields, erasing every
	// other setting the user has (theme, keybindings, MCP servers, ...).
	var hConfig map[string]json.RawMessage
	if data, err := os.ReadFile(hostConfigPath); err == nil {
		if uerr := json.Unmarshal(data, &hConfig); uerr != nil {
			return fmt.Errorf("host %s is not valid JSON — refusing writeback so it isn't overwritten: %w",
				hostConfigPath, uerr)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read host %s: %w", hostConfigPath, err)
	}
	if hConfig == nil {
		hConfig = make(map[string]json.RawMessage)
	}

	// Merge only the auth / onboarding fields — same set the entrypoint
	// extracts when it goes the other direction (host → container).
	authFields := []string{
		"oauthAccount",
		"userID",
		"hasCompletedOnboarding",
		"lastOnboardingVersion",
		"subscriptionNoticeCount",
		"hasAvailableSubscription",
		"s1mAccessCache",
	}
	changed := false
	for _, key := range authFields {
		if v, ok := cConfig[key]; ok && string(v) != "null" {
			hConfig[key] = v
			changed = true
		}
	}
	if !changed {
		return nil // nothing to write back
	}

	updated, err := json.MarshalIndent(hConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal host config: %w", err)
	}
	return os.WriteFile(hostConfigPath, updated, 0o600)
}

// buildContainerEnv produces the full set of env vars forwarded into
// the container at `docker run` time. Order of precedence (later wins):
//
//  1. Harness AuthEnvVars (host env values passed through)
//  2. Harness LLMEnvVars (computed from pin.LLM)
//  3. Extension auth.env vars (host env values passed through)
//  4. pin.Env overrides (literal or $NAME indirection from host)
//
// Extensions sit between LLM and pin.Env because:
//   - The harness's own auth is more fundamental than an extension's,
//     so harness AuthEnvVars take precedence when names collide.
//   - pin.Env is the user's explicit per-workspace override and wins
//     over everything else by design.
//
// enabledExts is the resolved list of extensions the user enabled for
// this build. May be nil — in which case the function behaves exactly
// like the pre-extensions implementation. Tests that don't care
// about extension auth pass nil and continue working.
func buildContainerEnv(pin config.Pin, enabledExts []*extensions.Entry) ([]docker.EnvVar, error) {
	h, ok := harness.ByID(pin.Harness)
	if !ok {
		return nil, fmt.Errorf("unknown harness %q", pin.Harness)
	}

	// Materialize into an ordered map so we can deduplicate by name
	// while preserving the precedence rule (later wins).
	final := map[string]string{}

	// 1. Auth env vars — forward host values verbatim. For the
	//    claude-code OAuth token specifically, fall back to a token file
	//    on the host (~/.claude-docker-token) when the env var is unset.
	//    This convention lets users keep the OAuth token in a file
	//    rather than in their shell rc.
	for _, name := range h.AuthEnvVars() {
		if v := os.Getenv(name); v != "" {
			final[name] = v
			continue
		}
		if name == "CLAUDE_CODE_OAUTH_TOKEN" {
			if tok := readOAuthTokenFile(); tok != "" {
				final[name] = tok
			}
		}
	}

	// 2. LLM-derived env vars from pin.LLM.
	if pin.LLM != nil {
		apiKey, err := resolveAPIKey(pin.LLM)
		if err != nil {
			return nil, fmt.Errorf("resolve LLM api key: %w", err)
		}
		for k, v := range h.LLMEnvVars(pin.LLM.Provider, pin.LLM.Model, pin.LLM.BaseURL, apiKey) {
			final[k] = v
		}
	}

	// 3. Extension auth.env vars — forward host values verbatim.
	//    Silent when unset: the wizard's [token: $X] badge is the
	//    user-facing prompt; failing the launch here would block
	//    users who chose an alternative auth path (OAuth, ambient
	//    workload identity, etc.).
	//
	//    Skip names the harness AuthEnvVars or LLMEnvVars already
	//    populated — those are more fundamental and shouldn't be
	//    silently overwritten by an extension's hint.
	for _, e := range enabledExts {
		if e.Auth == nil || e.Auth.Env == "" {
			continue
		}
		name := e.Auth.Env
		if _, already := final[name]; already {
			continue
		}
		if v := os.Getenv(name); v != "" {
			final[name] = v
		}
	}

	// 4. Identity override. When [identity] is set, force git's authoring
	//    identity via the GIT_*/EMAIL env vars (git prefers these over any
	//    config, so commits use the alias regardless of the mounted or
	//    in-container gitconfig) and hand the alias to the entrypoint via
	//    VIBRATOR_IDENTITY_* so it can also rewrite oauthAccount in
	//    ~/.claude.json. Emitted before pin.Env so a power user can still
	//    override an individual var explicitly.
	if id := pin.Identity; id != nil {
		if id.Email != "" {
			final["GIT_AUTHOR_EMAIL"] = id.Email
			final["GIT_COMMITTER_EMAIL"] = id.Email
			final["EMAIL"] = id.Email
			final["VIBRATOR_IDENTITY_EMAIL"] = id.Email
		}
		if id.Name != "" {
			final["GIT_AUTHOR_NAME"] = id.Name
			final["GIT_COMMITTER_NAME"] = id.Name
			final["VIBRATOR_IDENTITY_NAME"] = id.Name
		}
	}

	// 5. pin.Env overrides. Values of the form "$NAME" are resolved
	//    against the host's environment; literal values pass through.
	for k, v := range pin.Env {
		if strings.HasPrefix(v, "$") {
			final[k] = os.Getenv(strings.TrimPrefix(v, "$"))
		} else {
			final[k] = v
		}
	}

	// 6. Per-integration hosting mode. The container's claude-exec wrapper
	//    reads VIBRATOR_INTEGRATION_MODE_<UPPERID> to decide between the
	//    host server (http) and a container-local fallback (stdio). Only
	//    explicit choices are forwarded; an absent var means "auto" in the
	//    wrapper, so we don't need to emit the default.
	for id, mode := range pin.Integrations {
		final[integrationModeEnvVar(id)] = mode
	}

	// Convert to sorted []docker.EnvVar for stable output (matters in
	// tests and when debugging exact `docker run` arg lines).
	names := make([]string, 0, len(final))
	for n := range final {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]docker.EnvVar, 0, len(final))
	for _, n := range names {
		out = append(out, docker.EnvVar{Name: n, Value: final[n]})
	}
	return out, nil
}

// integrationModeEnvVar maps an integration id to the env var name that
// carries its hosting mode into the container. The id is upper-cased and
// any character that isn't a letter or digit becomes '_' so the result is
// always a valid shell identifier (e.g. "claude-mem" → ...MODE_CLAUDE_MEM).
func integrationModeEnvVar(id string) string {
	var b strings.Builder
	b.WriteString("VIBRATOR_INTEGRATION_MODE_")
	for _, r := range strings.ToUpper(id) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// resolveAPIKey extracts the credential the LLM provider expects.
// Precedence:
//
//  1. pin.LLM.Auth.Value — pasted-into-wizard literal.
//  2. $pin.LLM.Auth.Env — host environment variable name.
//  3. "" — only valid for local providers (ollama, lmstudio).
//
// Returns ("", nil) for local providers. Returns an error when a cloud
// provider has neither path populated.
func resolveAPIKey(spec *config.LLMSpec) (string, error) {
	switch spec.Provider {
	case "ollama", "lmstudio":
		return "", nil
	}
	if spec.Auth == nil {
		return "", fmt.Errorf("provider %q requires credentials but [llm.auth] is missing", spec.Provider)
	}
	if spec.Auth.Value != "" {
		return spec.Auth.Value, nil
	}
	if spec.Auth.Env != "" {
		v := os.Getenv(spec.Auth.Env)
		if v == "" {
			return "", fmt.Errorf("env var $%s is unset on the host", spec.Auth.Env)
		}
		return v, nil
	}
	return "", fmt.Errorf("provider %q has no credential configured", spec.Provider)
}

// runIntegrationReadiness evaluates every LaunchCheck declared by registered
// integrations. It replaces the old hard-fail runLaunchPrereqs with a
// user-friendly model:
//
//   - Each failing check prints a warning with a fix hint and the exact
//     command to run.
//   - Checks with FixNow set offer an inline "[y/N] Bootstrap now?" prompt
//     when stdin is a terminal. On confirmation the fix runs, the result is
//     merged into the pin, and the updated pin (plus a dirty flag) are
//     returned so the caller can persist it before launching.
//   - A failing check NEVER aborts the launch — the integration is simply
//     dormant. The user always reaches their container.
//
// Returns (pin, dirty, err). dirty is true when a FixNow ran successfully
// and the pin was updated; the caller should re-persist to .vb.
func runIntegrationReadiness(
	ctx context.Context,
	pin config.Pin,
	wsDir string,
	opts Options,
) (config.Pin, bool, error) {
	all := integration.All()
	if len(all) == 0 {
		return pin, false, nil
	}

	hostname, _ := os.Hostname()
	lc := integration.LaunchCheckContext{
		WsDir:        wsDir,
		ProjectName:  filepath.Base(wsDir),
		Hostname:     hostname,
		Extensions:   pin.Extensions,
		Prereqs:      pin.Prereqs,
		Integrations: pin.Integrations,
	}

	pinDirty := false

	for _, integ := range all {
		if len(integ.LaunchChecks) == 0 {
			continue
		}
		for _, check := range integ.LaunchChecks {
			probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			result := check.Check(probeCtx, lc)
			cancel()

			if result.OK {
				continue
			}

			// Print the warning block.
			fmt.Fprintf(opts.Stderr, "\n  ⚠  [%s] %s\n", integ.ID, result.Message)
			if result.Hint != "" {
				fmt.Fprintf(opts.Stderr, "     hint: %s\n", result.Hint)
			}
			if result.FixCmd != "" {
				fmt.Fprintf(opts.Stderr, "     run:  %s\n", result.FixCmd)
			}

			// Offer inline fix when available and stdin is a terminal.
			if result.FixNow != nil && isStdinTTY(opts.Stdin) {
				fmt.Fprintf(opts.Stderr, "     Bootstrap now? [y/N] ")
				var answer string
				_, _ = fmt.Fscanln(opts.Stdin, &answer)
				if strings.ToLower(strings.TrimSpace(answer)) == "y" {
					prereqID, res, fixErr := result.FixNow(ctx, lc)
					if fixErr != nil {
						fmt.Fprintf(opts.Stderr, "     ✗ bootstrap failed: %v\n", fixErr)
					} else if prereqID != "" {
						if pin.Prereqs == nil {
							pin.Prereqs = make(map[string]map[string]string)
						}
						pin.Prereqs[prereqID] = res
						lc.Prereqs = pin.Prereqs // keep context fresh for subsequent checks
						pinDirty = true
						fmt.Fprintf(opts.Stderr, "     ✓ bootstrap complete — key saved to .vb\n")
					}
				}
			}
		}
	}

	fmt.Fprintln(opts.Stderr) // blank line after any warnings
	return pin, pinDirty, nil
}

// isStdinTTY reports whether in is an *os.File pointing at a character
// device (a real terminal). Returns false for pipes, buffers, and nil.
func isStdinTTY(in io.Reader) bool {
	f, ok := in.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// ensureLLMProviderRunning launches the host-side local provider if the
// pin specifies one (Ollama / LM Studio). For cloud providers this is
// a no-op.
//
// The function returns an error if the local provider can't be
// reached AND can't be auto-started — abort the launch rather than
// running a container that will immediately fail.
func ensureLLMProviderRunning(ctx context.Context, pin config.Pin, stderr io.Writer) error {
	if pin.LLM == nil {
		return nil
	}
	p, ok := localprovider.ByID(pin.LLM.Provider)
	if !ok {
		// Not a local provider — nothing to start.
		return nil
	}
	url := pin.LLM.BaseURL
	if url == "" {
		url = p.DefaultURL()
	}
	fmt.Fprintf(stderr, "→ Ensuring %s is running at %s ...\n", p.Name(), url)

	startCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := p.EnsureRunning(startCtx, url, pin.LLM.Model); err != nil {
		return fmt.Errorf("local provider %s not reachable: %w", p.Name(), err)
	}
	fmt.Fprintf(stderr, "  ✓ %s reachable\n", p.Name())
	return nil
}
