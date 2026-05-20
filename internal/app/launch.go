package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/wlame/vibrator/internal/extensions"
	vibrator "github.com/wlame/vibrator"
	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/dockerfile"
	"github.com/wlame/vibrator/internal/harness"
	"github.com/wlame/vibrator/internal/localprovider"
	"github.com/wlame/vibrator/internal/prereq"
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
func runContainer(ctx context.Context, dc docker.Client,
	imageTag, containerName, wsDir string,
	wsSpec workspace.Spec, pin config.Pin, opts Options,
) error {
	wsHash := workspace.Fingerprint(wsSpec)

	envVars, err := buildContainerEnv(pin)
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
	}

	// Workspace mount at the same absolute path on both sides — the
	// foundational mount that makes paths in error messages, stack
	// traces, and `pwd` match the user's host muscle memory.
	volumes := []docker.Volume{
		{Host: wsDir, Container: wsDir},
	}
	// Host claude config / settings / rules / session state. The
	// container-side entrypoint script reads these to seed the in-
	// container ~/.claude/ on first run.
	if pin.Harness == "claude-code" {
		volumes = append(volumes, buildClaudeHostMounts(defaultUsername(opts), opts.Stderr)...)
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

	return dc.Run(ctx, docker.RunSpec{
		Image:         imageTag,
		ContainerName: containerName,
		Hostname:      workspace.Hostname(wsDir),
		GroupAdd:      dockerGroupAdd,
		Interactive:   true,
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
		// page that allocates ~50MB of canvas). 2GB matches the bash
		// impl and is comfortably above any single-page workload.
		ShmSize:     "2g",
		Volumes:     volumes,
		Env:         envVars,
		Labels:      labels,
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

// execIntoContainer runs an interactive shell inside an already-running
// (or just-started) container. wsDir is the workspace path on the host
// (also the path inside the container, since vibrator mounts at the
// same absolute path) — used to set --workdir so re-entries land in
// the project, not the user's $HOME.
func execIntoContainer(ctx context.Context, dc docker.Client,
	containerName, wsDir string, pin config.Pin, opts Options,
) error {
	shell := pin.Shell
	if shell == "" {
		shell = "zsh"
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
		// Wrap the shell with claude-exec so the live Serena MCP probe
		// (C6) re-runs on every entry. claude-exec exec's its $@ at
		// the end, so the shell becomes the wrapper's replacement
		// process — signals route correctly, no extra PID layer.
		Cmd:    []string{"/usr/local/bin/claude-exec", "/bin/" + shell},
		Stdin:  opts.Stdin,
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
	})
}

// claudeSessionPersistDirs are the per-CC subdirectories that hold
// in-progress conversations, file history, etc. Bind-mounting them
// means a container-side claude session shows up in the host's claude
// history list (and vice versa) — the same "session continuity" the
// bash impl gave users by default.
//
// Trade-off: shared mutable state. Concurrent host + container claude
// runs could race on the same JSON files. The bash impl shipped this
// default-on for ~12 months without major complaints, so we follow.
var claudeSessionPersistDirs = []string{
	"projects",
	"file-history",
	"sessions",
	"tasks",
	"paste-cache",
}

// buildClaudeHostMounts produces the volume list that wires host
// ~/.claude state into the container. All mounts are conditional on
// the host source existing — a fresh host with no claude install
// gets no extra mounts and the entrypoint gracefully no-ops.
//
// Session-persist dirs (D5) are auto-created on the host if missing
// so the container can write to them on first run (matching the bash
// impl's `mkdir -p` before mount).
//
// `containerUser` is the unprivileged user the image was built for;
// container paths land under /home/<user>/.claude/...
func buildClaudeHostMounts(containerUser string, stderr io.Writer) []docker.Volume {
	var out []docker.Volume

	hostHome, err := os.UserHomeDir()
	if err != nil || hostHome == "" {
		return out
	}
	containerHome := "/home/" + containerUser
	containerClaude := containerHome + "/.claude"
	hostClaude := filepath.Join(hostHome, ".claude")

	// D1: ~/.claude.json → ~/.claude.host.json:ro
	// Entrypoint extracts OAuth + onboarding fields from this. Read-only
	// because the container should NEVER modify the host's master config.
	if exists(filepath.Join(hostHome, ".claude.json")) {
		out = append(out, docker.Volume{
			Host:      filepath.Join(hostHome, ".claude.json"),
			Container: containerHome + "/.claude.host.json",
			ReadOnly:  true,
		})
	}

	// D2: ~/.claude/settings.json → ~/.claude/settings.host.json:ro
	// Entrypoint copies this with macOS-path rewrite and re-merges
	// baked plugin hooks. Read-only for the same reason as D1.
	if exists(filepath.Join(hostClaude, "settings.json")) {
		out = append(out, docker.Volume{
			Host:      filepath.Join(hostClaude, "settings.json"),
			Container: containerClaude + "/settings.host.json",
			ReadOnly:  true,
		})
	}

	// D3: ~/.claude/rules → ~/.claude/rules-host:ro
	// Entrypoint copies *.md from rules-host into the container's rules
	// dir on every entry, so editing rules on host takes effect next run.
	if isDir(filepath.Join(hostClaude, "rules")) {
		out = append(out, docker.Volume{
			Host:      filepath.Join(hostClaude, "rules"),
			Container: containerClaude + "/rules-host",
			ReadOnly:  true,
		})
	}

	// D4: ~/.claude/hooks → ~/.claude/hooks (rw)
	// Hook scripts the user wrote on host. Writable so the user can
	// edit hooks inside the container and changes persist back to host.
	if isDir(filepath.Join(hostClaude, "hooks")) {
		out = append(out, docker.Volume{
			Host:      filepath.Join(hostClaude, "hooks"),
			Container: containerClaude + "/hooks",
			ReadOnly:  false,
		})
	}

	// D5: session persistence dirs (rw).
	// Auto-create on host if missing so first-time `vibrate` users
	// still get their container session persisted somewhere.
	for _, name := range claudeSessionPersistDirs {
		hostPath := filepath.Join(hostClaude, name)
		if !isDir(hostPath) {
			if err := os.MkdirAll(hostPath, 0o755); err != nil {
				// Non-fatal — log and skip this mount. A failed mkdir
				// here usually means host perms are weird, not a
				// vibrator bug; user can `mkdir -p` themselves.
				fmt.Fprintf(stderr, "vibrate: warning: couldn't create %s for session persistence: %v\n",
					hostPath, err)
				continue
			}
		}
		out = append(out, docker.Volume{
			Host:      hostPath,
			Container: containerClaude + "/" + name,
			ReadOnly:  false,
		})
	}

	return out
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
	// so strip the prefix when present.
	if dh := os.Getenv("DOCKER_HOST"); strings.HasPrefix(dh, "unix://") {
		candidates = append(candidates, strings.TrimPrefix(dh, "unix://"))
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
		// Resolve the GID. On Linux this is the docker group; on
		// macOS Docker Desktop the socket is owned by the user's
		// staff group (the container daemon runs in a VM but the
		// socket is just a forwarded pipe). Either way, granting the
		// container user that GID lets it `read+write` the socket.
		gid := socketGroupGID(path)
		if gid == "" {
			fmt.Fprintf(stderr,
				"vibrate: --dind: found socket %s but couldn't determine its group GID; container user may need `sudo` for docker commands\n",
				path)
		}
		return docker.Volume{
			Host:      path,
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
// The CONTAINER side gets the socket at `/gpg-agent-extra` (matches the
// bash impl's convention). The entrypoint script (step C5) symlinks
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

// claudeOAuthTokenFile is the conventional host path the bash impl
// used for storing a long-lived Claude OAuth token outside the shell
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

// buildContainerEnv produces the full set of env vars forwarded into
// the container at `docker run` time. Order of precedence:
//
//  1. Harness AuthEnvVars (host env values passed through)
//  2. Harness LLMEnvVars (computed from pin.LLM)
//  3. pin.Env overrides (literal or $NAME indirection from host)
//
// Later entries with the same name win.
func buildContainerEnv(pin config.Pin) ([]docker.EnvVar, error) {
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
	//    The bash impl supported this convention so users could keep the
	//    OAuth token in a file rather than in their shell rc.
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

	// 3. pin.Env overrides. Values of the form "$NAME" are resolved
	//    against the host's environment; literal values pass through.
	for k, v := range pin.Env {
		if strings.HasPrefix(v, "$") {
			final[k] = os.Getenv(strings.TrimPrefix(v, "$"))
		} else {
			final[k] = v
		}
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

// runLaunchPrereqs probes every prereq referenced by the pin's extensions
// entries. Failure here is fatal — entering a container with broken
// host wiring just wastes the user's time. The error message
// references the extensions's setup-doc anchor so the user knows where
// to look.
//
// This is the wizard's "soft warn" promoted to "hard fail" for launch.
func runLaunchPrereqs(ctx context.Context, pin config.Pin, stderr io.Writer) error {
	if len(pin.Extensions) == 0 {
		return nil
	}
	entries, err := extensions.LoadAll(vibrator.ExtensionsFS)
	if err != nil {
		return fmt.Errorf("load extensions: %w", err)
	}

	// Walk pin.Extensions and collect distinct prereq IDs referenced.
	prereqIDs := map[string]bool{}
	for _, id := range pin.Extensions {
		key := pin.Harness + "/" + id
		entry, ok := entries[key]
		if !ok || entry.Prereq == "" {
			continue
		}
		prereqIDs[entry.Prereq] = true
	}
	if len(prereqIDs) == 0 {
		return nil
	}

	// For each unique prereq id, probe.
	for id := range prereqIDs {
		// claude-mem is the only built-in prereq for now. New ones can
		// drop into this switch as they're added.
		var p *prereq.Prereq
		switch id {
		case prereq.ClaudeMemPrereqID:
			cfg, err := prereq.LoadClaudeMemAdminConfig()
			if err != nil {
				return fmt.Errorf("claude-mem admin config not found (%s) — see extensions/claude-code/claude-mem.md#host-setup", prereq.ClaudeMemAdminConfigPath())
			}
			p = prereq.ClaudeMemPrereq(cfg, nil)
		default:
			fmt.Fprintf(stderr, "  (skipping unknown prereq %q — no probe registered)\n", id)
			continue
		}

		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		r := p.Verifier.Verify(probeCtx)
		cancel()

		if !r.OK {
			return fmt.Errorf(
				"prereq %q FAILED at launch: %s\nhint: %s\nsee: %s",
				id, r.Message, r.Hint, p.SetupDoc)
		}
		fmt.Fprintf(stderr, "  ✓ prereq %s: %s\n", id, r.Message)
	}
	return nil
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
