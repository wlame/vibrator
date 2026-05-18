// Package runtime auto-detects the Docker runtime (Desktop, OrbStack, Colima,
// Rancher Desktop, Podman, native) and its socket path.
//
// macOS in particular ships several mutually-incompatible Docker runtimes that
// expose sockets under different paths. We can't assume /var/run/docker.sock.
// Linux native usually does, but Colima/Podman can be installed there too.
//
// Detection priority (first match wins):
//
//  1. Explicit socket override ($VIBRATOR_DOCKER_SOCKET or Options.SocketOverride)
//  2. $DOCKER_HOST environment variable (unix://path/to/sock)
//  3. `docker context inspect` output for the active context
//  4. Well-known socket paths in a deterministic order
//
// Tests stub the filesystem + env + exec layers via the Detector struct.
package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runtime identifies a particular Docker daemon implementation.
type Runtime string

const (
	DockerDesktop  Runtime = "docker-desktop"
	OrbStack       Runtime = "orbstack"
	Colima         Runtime = "colima"
	RancherDesktop Runtime = "rancher-desktop"
	Podman         Runtime = "podman"
	Native         Runtime = "native"
	Custom         Runtime = "custom"
	Unknown        Runtime = "unknown"
)

// Source records where a detection result came from, useful for diagnostics.
type Source string

const (
	SourceOverride   Source = "env-override"   // $VIBRATOR_DOCKER_SOCKET / Options.SocketOverride
	SourceDockerHost Source = "docker-host"    // $DOCKER_HOST env var
	SourceContext    Source = "docker-context" // `docker context inspect`
	SourceScan       Source = "socket-scan"    // walked well-known paths
)

// Detection is the result of a successful detection pass.
type Detection struct {
	Runtime Runtime
	Socket  string // absolute path to the Unix socket
	Source  Source // how we found it
}

// Options tunes Detect()'s priority chain. Zero values use sensible defaults
// (env lookups via os.Getenv, $HOME from os.UserHomeDir, etc.).
type Options struct {
	// SocketOverride forces detection to use this exact socket. Highest priority.
	// Equivalent to setting $VIBRATOR_DOCKER_SOCKET.
	SocketOverride string

	// ColimaProfile selects which Colima VM profile to look for. Defaults to
	// $COLIMA_PROFILE, then "default". Only consulted if the well-known-paths
	// scan reaches the Colima check.
	ColimaProfile string
}

// Detector encapsulates the side-effecting dependencies that Detect uses, so
// tests can inject stubs without touching real env / filesystem / docker CLI.
// Zero-value Detector uses production defaults (os.Getenv, os.Stat, exec.Command).
type Detector struct {
	// Env reads an environment variable. Defaults to os.Getenv when nil.
	Env func(key string) string

	// HomeDir returns the user's home directory. Defaults to os.UserHomeDir().
	HomeDir func() (string, error)

	// IsSocket reports whether path is a Unix socket. Defaults to a stat-based
	// check (mode&ModeSocket != 0).
	IsSocket func(path string) bool

	// Readlink returns the target of a symlink (one hop, not -f resolution),
	// matching the behaviour the bash version needed for /var/run/docker.sock
	// pointing at e.g. ~/.colima/default/docker.sock. Defaults to os.Readlink.
	Readlink func(path string) (string, error)

	// DockerCtxEndpoint runs `docker context inspect --format '{{.Endpoints.docker.Host}}'`
	// and returns the endpoint URL. Empty string means "no docker CLI" or
	// "no active context". Defaults to exec.Command-based impl.
	DockerCtxEndpoint func() (string, error)
}

// DefaultDetector returns a Detector wired up to real os/exec dependencies.
// This is the value you almost certainly want outside of tests.
func DefaultDetector() *Detector {
	return &Detector{
		Env:               os.Getenv,
		HomeDir:           os.UserHomeDir,
		IsSocket:          isSocket,
		Readlink:          os.Readlink,
		DockerCtxEndpoint: dockerCtxEndpoint,
	}
}

// Detect runs the priority chain and returns the first match. Returns Unknown
// + a non-nil error when no runtime can be found.
func (d *Detector) Detect(opts Options) (Detection, error) {
	d.fillDefaults()

	home, err := d.HomeDir()
	if err != nil {
		// Not fatal — only well-known-path detection needs HOME. Continue with
		// an empty string; the path scan will simply not match anything.
		home = ""
	}

	// 1. Explicit socket override
	if sock := opts.SocketOverride; sock != "" {
		if d.IsSocket(sock) {
			return Detection{
				Runtime: identifyByPath(sock),
				Socket:  sock,
				Source:  SourceOverride,
			}, nil
		}
	}
	if sock := d.Env("VIBRATOR_DOCKER_SOCKET"); sock != "" {
		if d.IsSocket(sock) {
			return Detection{
				Runtime: identifyByPath(sock),
				Socket:  sock,
				Source:  SourceOverride,
			}, nil
		}
	}

	// 2. $DOCKER_HOST
	if hostURL := d.Env("DOCKER_HOST"); hostURL != "" {
		sock := stripUnixScheme(hostURL)
		if sock != "" && d.IsSocket(sock) {
			return Detection{
				Runtime: identifyByPath(sock),
				Socket:  sock,
				Source:  SourceDockerHost,
			}, nil
		}
	}

	// 3. docker context inspect (only if docker CLI is available — errors here
	// are non-fatal; we fall through to socket scanning)
	if endpoint, err := d.DockerCtxEndpoint(); err == nil && endpoint != "" {
		sock := stripUnixScheme(endpoint)
		if sock != "" && d.IsSocket(sock) {
			return Detection{
				Runtime: identifyByEndpoint(endpoint, sock),
				Socket:  sock,
				Source:  SourceContext,
			}, nil
		}
	}

	// 4. Well-known socket paths, in priority order
	if home != "" {
		// Docker Desktop (post-4.18 default)
		if p := filepath.Join(home, ".docker", "run", "docker.sock"); d.IsSocket(p) {
			return Detection{Runtime: DockerDesktop, Socket: p, Source: SourceScan}, nil
		}
		// OrbStack
		if p := filepath.Join(home, ".orbstack", "run", "docker.sock"); d.IsSocket(p) {
			return Detection{Runtime: OrbStack, Socket: p, Source: SourceScan}, nil
		}
		// Colima (with profile)
		profile := opts.ColimaProfile
		if profile == "" {
			if env := d.Env("COLIMA_PROFILE"); env != "" {
				profile = env
			} else {
				profile = "default"
			}
		}
		if p := filepath.Join(home, ".colima", profile, "docker.sock"); d.IsSocket(p) {
			return Detection{Runtime: Colima, Socket: p, Source: SourceScan}, nil
		}
		// Rancher Desktop
		if p := filepath.Join(home, ".rd", "docker.sock"); d.IsSocket(p) {
			return Detection{Runtime: RancherDesktop, Socket: p, Source: SourceScan}, nil
		}
		// Podman machine (linux + macOS)
		if p := filepath.Join(home, ".local", "share", "containers", "podman", "machine", "podman.sock"); d.IsSocket(p) {
			return Detection{Runtime: Podman, Socket: p, Source: SourceScan}, nil
		}
	}

	// 5. /var/run/docker.sock — could be native, or a symlink into one of the
	// per-user runtimes. Follow the link to identify if possible.
	const stdSock = "/var/run/docker.sock"
	if d.IsSocket(stdSock) {
		runtime := Native
		if target, err := d.Readlink(stdSock); err == nil && target != "" {
			runtime = identifyByPath(target)
			// readlink can return a relative target; that's fine — we only
			// use it as a substring match for identifyByPath.
		}
		return Detection{Runtime: runtime, Socket: stdSock, Source: SourceScan}, nil
	}

	return Detection{Runtime: Unknown}, fmt.Errorf("no docker runtime detected")
}

// fillDefaults populates nil function fields with their os-backed defaults.
// Called at the start of every Detect to keep the zero-value Detector usable.
func (d *Detector) fillDefaults() {
	if d.Env == nil {
		d.Env = os.Getenv
	}
	if d.HomeDir == nil {
		d.HomeDir = os.UserHomeDir
	}
	if d.IsSocket == nil {
		d.IsSocket = isSocket
	}
	if d.Readlink == nil {
		d.Readlink = os.Readlink
	}
	if d.DockerCtxEndpoint == nil {
		d.DockerCtxEndpoint = dockerCtxEndpoint
	}
}

// Detect is a package-level convenience that uses DefaultDetector. Equivalent
// to DefaultDetector().Detect(opts).
func Detect(opts Options) (Detection, error) {
	return DefaultDetector().Detect(opts)
}

// identifyByPath inspects a socket path and returns the runtime it likely
// belongs to. Substring matching is intentional — readlink can return targets
// in either absolute (`/Users/x/.colima/default/docker.sock`) or relative
// (`../../../Users/x/.colima/...`) form.
func identifyByPath(p string) Runtime {
	switch {
	case strings.Contains(p, ".docker"):
		return DockerDesktop
	case strings.Contains(p, ".orbstack"):
		return OrbStack
	case strings.Contains(p, ".colima"):
		return Colima
	case strings.Contains(p, ".rd"):
		return RancherDesktop
	case strings.Contains(p, "podman"):
		return Podman
	case p == "/var/run/docker.sock":
		return Native
	default:
		return Custom
	}
}

// identifyByEndpoint inspects a docker context endpoint URL (which may
// include scheme like 'unix:///run/...' and may also contain context-name
// hints like 'desktop-linux' that the bare socket path lacks) and falls
// back to identifyByPath for the socket portion.
func identifyByEndpoint(endpoint, sock string) Runtime {
	switch {
	case strings.Contains(endpoint, "desktop-linux"):
		return DockerDesktop
	case strings.Contains(endpoint, "orbstack"):
		return OrbStack
	case strings.Contains(endpoint, "colima"):
		return Colima
	case strings.Contains(endpoint, "rancher-desktop"):
		return RancherDesktop
	case strings.Contains(endpoint, "podman"):
		return Podman
	}
	return identifyByPath(sock)
}

// stripUnixScheme normalizes "unix:///foo" → "/foo" and leaves bare paths
// alone. Returns empty string if the URL uses a non-unix scheme (we don't
// support remote docker daemons in this tool).
func stripUnixScheme(host string) string {
	if strings.HasPrefix(host, "unix://") {
		return strings.TrimPrefix(host, "unix://")
	}
	// Bare path? Treat it as a unix socket path.
	if strings.HasPrefix(host, "/") {
		return host
	}
	// tcp:// / http:// / ssh:// → unsupported
	return ""
}

// isSocket reports whether path is a Unix domain socket. Symlinks are
// followed (Stat, not Lstat) because most runtimes ship /var/run/docker.sock
// as a symlink into a per-user dir.
func isSocket(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSocket != 0
}

// dockerCtxEndpoint runs `docker context inspect` to retrieve the active
// context's endpoint URL. Returns ("", nil) when docker isn't installed —
// callers fall through to socket scanning.
func dockerCtxEndpoint() (string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "", nil
	}
	cmd := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}")
	out, err := cmd.Output()
	if err != nil {
		// Most likely: docker daemon not running, or no active context.
		// Either way, signal "no context info" so the caller falls through.
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}
