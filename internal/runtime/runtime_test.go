package runtime

import (
	"path/filepath"
	"testing"
)

// fakeFS represents a synthetic filesystem for Detect tests: paths that exist
// (as sockets) map to true. Any path not in the map is treated as missing.
type fakeFS map[string]bool

func (f fakeFS) isSocket(p string) bool { return f[p] }

// stubEnv returns a Env func that looks up keys from the provided map.
func stubEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// stubHome returns a HomeDir func that returns a fixed path.
func stubHome(home string) func() (string, error) {
	return func() (string, error) { return home, nil }
}

// stubCtx returns a DockerCtxEndpoint func that returns a fixed endpoint.
func stubCtx(endpoint string) func() (string, error) {
	return func() (string, error) { return endpoint, nil }
}

// stubReadlink returns a Readlink func that returns a fixed target for any
// path. Use "" + non-nil error for "not a symlink".
func stubReadlink(target string) func(string) (string, error) {
	return func(string) (string, error) {
		if target == "" {
			return "", errNotSymlink
		}
		return target, nil
	}
}

// errNotSymlink is a sentinel for stubReadlink. The real implementation returns
// *os.PathError; we just need any non-nil error to signal "not a symlink".
var errNotSymlink = stubErr("not a symlink")

type stubErr string

func (e stubErr) Error() string { return string(e) }

// newDetector wires up a Detector with explicit fakes and zero side effects
// against the real OS. Tests should call this rather than DefaultDetector().
func newDetector(fs fakeFS, env map[string]string, home, ctxEndpoint, readlinkTarget string) *Detector {
	return &Detector{
		Env:               stubEnv(env),
		HomeDir:           stubHome(home),
		IsSocket:          fs.isSocket,
		Readlink:          stubReadlink(readlinkTarget),
		DockerCtxEndpoint: stubCtx(ctxEndpoint),
	}
}

func TestDetect_SocketOverrideViaOptions(t *testing.T) {
	customSock := "/custom/path/docker.sock"
	d := newDetector(fakeFS{customSock: true}, nil, "/home/user", "", "")

	got, err := d.Detect(Options{SocketOverride: customSock})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Socket != customSock || got.Source != SourceOverride {
		t.Errorf("want override at %s, got %+v", customSock, got)
	}
}

func TestDetect_SocketOverrideViaEnv(t *testing.T) {
	customSock := "/custom/path/docker.sock"
	d := newDetector(
		fakeFS{customSock: true},
		map[string]string{"VIBRATOR_DOCKER_SOCKET": customSock},
		"/home/user", "", "",
	)

	got, err := d.Detect(Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Source != SourceOverride {
		t.Errorf("want SourceOverride, got %s", got.Source)
	}
}

func TestDetect_DockerHostUnixURL(t *testing.T) {
	sock := "/run/some/docker.sock"
	d := newDetector(
		fakeFS{sock: true},
		map[string]string{"DOCKER_HOST": "unix://" + sock},
		"/home/user", "", "",
	)

	got, err := d.Detect(Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Socket != sock || got.Source != SourceDockerHost {
		t.Errorf("want %s via DOCKER_HOST, got %+v", sock, got)
	}
}

func TestDetect_DockerHostTCPSchemeIgnored(t *testing.T) {
	// tcp:// is unsupported; we should fall through to socket scan.
	home := "/home/user"
	desktopSock := filepath.Join(home, ".docker", "run", "docker.sock")
	d := newDetector(
		fakeFS{desktopSock: true},
		map[string]string{"DOCKER_HOST": "tcp://localhost:2375"},
		home, "", "",
	)

	got, err := d.Detect(Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Source != SourceScan || got.Runtime != DockerDesktop {
		t.Errorf("want scan→Desktop, got source=%s runtime=%s", got.Source, got.Runtime)
	}
}

func TestDetect_DockerContextInspect(t *testing.T) {
	sock := "/Users/x/.colima/default/docker.sock"
	d := newDetector(
		fakeFS{sock: true},
		nil,
		"/Users/x",
		"unix://"+sock,
		"",
	)

	got, err := d.Detect(Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Source != SourceContext || got.Runtime != Colima {
		t.Errorf("want SourceContext→Colima, got %+v", got)
	}
}

// Each well-known runtime path should resolve to the correct Runtime+Scan source.
func TestDetect_WellKnownPaths(t *testing.T) {
	home := "/home/user"
	tests := []struct {
		name string
		path string
		want Runtime
	}{
		{"docker desktop", filepath.Join(home, ".docker", "run", "docker.sock"), DockerDesktop},
		{"orbstack", filepath.Join(home, ".orbstack", "run", "docker.sock"), OrbStack},
		{"colima default", filepath.Join(home, ".colima", "default", "docker.sock"), Colima},
		{"rancher desktop", filepath.Join(home, ".rd", "docker.sock"), RancherDesktop},
		{"podman machine", filepath.Join(home, ".local", "share", "containers", "podman", "machine", "podman.sock"), Podman},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := newDetector(fakeFS{tc.path: true}, nil, home, "", "")

			got, err := d.Detect(Options{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Runtime != tc.want || got.Source != SourceScan {
				t.Errorf("want %s via scan, got %+v", tc.want, got)
			}
		})
	}
}

func TestDetect_ColimaProfileFromOptions(t *testing.T) {
	home := "/home/user"
	customSock := filepath.Join(home, ".colima", "custom", "docker.sock")
	d := newDetector(fakeFS{customSock: true}, nil, home, "", "")

	got, err := d.Detect(Options{ColimaProfile: "custom"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Runtime != Colima || got.Socket != customSock {
		t.Errorf("want Colima at custom profile, got %+v", got)
	}
}

func TestDetect_ColimaProfileFromEnv(t *testing.T) {
	home := "/home/user"
	customSock := filepath.Join(home, ".colima", "myprofile", "docker.sock")
	d := newDetector(
		fakeFS{customSock: true},
		map[string]string{"COLIMA_PROFILE": "myprofile"},
		home, "", "",
	)

	got, err := d.Detect(Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Runtime != Colima || got.Socket != customSock {
		t.Errorf("want Colima profile via env, got %+v", got)
	}
}

func TestDetect_NativeStandardSocket(t *testing.T) {
	d := newDetector(fakeFS{"/var/run/docker.sock": true}, nil, "/home/user", "", "")

	got, err := d.Detect(Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Runtime != Native || got.Socket != "/var/run/docker.sock" {
		t.Errorf("want Native at /var/run, got %+v", got)
	}
}

func TestDetect_StandardSocketSymlinkToColima(t *testing.T) {
	// /var/run/docker.sock pointing at a Colima profile socket should be
	// identified as Colima, not Native.
	target := "/Users/x/.colima/default/docker.sock"
	d := newDetector(
		fakeFS{"/var/run/docker.sock": true},
		nil, "/Users/x", "",
		target,
	)

	got, err := d.Detect(Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Runtime != Colima {
		t.Errorf("want Colima (via readlink), got %s", got.Runtime)
	}
	// Socket reported is the user-visible path, not the symlink target.
	if got.Socket != "/var/run/docker.sock" {
		t.Errorf("want socket /var/run/docker.sock, got %s", got.Socket)
	}
}

func TestDetect_NoRuntimeFound(t *testing.T) {
	d := newDetector(fakeFS{}, nil, "/home/user", "", "")

	got, err := d.Detect(Options{})
	if err == nil {
		t.Fatalf("want error, got success: %+v", got)
	}
	if got.Runtime != Unknown {
		t.Errorf("want Unknown when no runtime found, got %s", got.Runtime)
	}
}

func TestDetect_PriorityOverrideOverDockerHost(t *testing.T) {
	// Both override and DOCKER_HOST point at valid sockets. Override wins.
	override := "/explicit/override.sock"
	other := "/run/some.sock"
	d := newDetector(
		fakeFS{override: true, other: true},
		map[string]string{"DOCKER_HOST": "unix://" + other},
		"/home/user", "", "",
	)

	got, err := d.Detect(Options{SocketOverride: override})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Source != SourceOverride || got.Socket != override {
		t.Errorf("want override to win, got %+v", got)
	}
}

func TestStripUnixScheme(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"unix:///var/run/docker.sock", "/var/run/docker.sock"},
		{"/var/run/docker.sock", "/var/run/docker.sock"},
		{"tcp://localhost:2375", ""},
		{"http://docker:2375", ""},
		{"ssh://host", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := stripUnixScheme(tc.in); got != tc.want {
			t.Errorf("stripUnixScheme(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIdentifyByPath(t *testing.T) {
	cases := []struct {
		in   string
		want Runtime
	}{
		{"/Users/x/.docker/run/docker.sock", DockerDesktop},
		{"/Users/x/.orbstack/run/docker.sock", OrbStack},
		{"/Users/x/.colima/default/docker.sock", Colima},
		{"/Users/x/.rd/docker.sock", RancherDesktop},
		{"/run/podman/podman.sock", Podman},
		{"/var/run/docker.sock", Native},
		{"/totally/custom/path.sock", Custom},
	}
	for _, tc := range cases {
		if got := identifyByPath(tc.in); got != tc.want {
			t.Errorf("identifyByPath(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}
