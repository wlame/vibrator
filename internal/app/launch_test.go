package app

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/mount"
	"github.com/wlame/vibrator/internal/workspace"
)

func TestRunContainerIncludesExtraMounts(t *testing.T) {
	roDir := t.TempDir()
	rwDir := t.TempDir()
	ws := t.TempDir()

	dc := docker.NewMock()
	var captured docker.RunSpec
	dc.RunHandler = func(_ context.Context, spec docker.RunSpec) error {
		captured = spec
		return nil
	}

	pin := config.Pin{Harness: "claude-code", Shell: "zsh",
		Mounts: []string{roDir, rwDir + ":rw"}}
	opts := Options{LaunchTarget: LaunchHarness, Stdout: io.Discard, Stderr: io.Discard}

	err := runContainer(context.Background(), dc, "img:tag", "ctr", ws,
		workspace.Spec{Harness: "claude-code"}, pin, nil, opts)
	if err != nil {
		t.Fatalf("runContainer: %v", err)
	}

	var sawRO, sawRW bool
	for _, v := range captured.Volumes {
		if v.Host == roDir && v.Container == roDir && v.ReadOnly {
			sawRO = true
		}
		if v.Host == rwDir && v.Container == rwDir && !v.ReadOnly {
			sawRW = true
		}
	}
	if !sawRO {
		t.Errorf("read-only mount %s missing or not ro in RunSpec.Volumes: %+v", roDir, captured.Volumes)
	}
	if !sawRW {
		t.Errorf("read-write mount %s missing or not rw in RunSpec.Volumes: %+v", rwDir, captured.Volumes)
	}
}

func TestRunContainerMissingMountAborts(t *testing.T) {
	ws := t.TempDir()
	dc := docker.NewMock()

	pin := config.Pin{Harness: "claude-code", Shell: "zsh",
		Mounts: []string{"/definitely/does/not/exist/vb-missing"}}
	opts := Options{LaunchTarget: LaunchHarness, Stdout: io.Discard, Stderr: io.Discard}

	err := runContainer(context.Background(), dc, "img:tag", "ctr", ws,
		workspace.Spec{Harness: "claude-code"}, pin, nil, opts)
	if err == nil {
		t.Fatal("expected runContainer to abort on a missing --mount path, got nil")
	}
	if !strings.Contains(err.Error(), "no such directory") {
		t.Fatalf("error should mention the missing directory, got: %v", err)
	}
	// Fail-fast: no container was created.
	if runs := dc.CallsFor("run"); len(runs) != 0 {
		t.Errorf("expected NO docker run after a bad --mount, got %d run call(s)", len(runs))
	}
}

func TestExecIntoContainerAnnouncesMounts(t *testing.T) {
	roDir := t.TempDir()
	ws := t.TempDir()
	dc := docker.NewMock()
	var stderr bytes.Buffer

	pin := config.Pin{Harness: "claude-code", Shell: "zsh", Mounts: []string{roDir}}
	opts := Options{LaunchTarget: LaunchHarness, Stdout: io.Discard, Stderr: &stderr}

	if err := execIntoContainer(context.Background(), dc, "ctr", ws, pin, opts); err != nil {
		t.Fatalf("execIntoContainer: %v", err)
	}
	if !strings.Contains(stderr.String(), "Mounted 1 extra folder") {
		t.Fatalf("expected a mount notice on the reuse path, got: %q", stderr.String())
	}
}

// TestRunContainerCarriesYoloEnvOverride pins that runContainer forwards
// the runtime VIBRATOR_YOLO_ARGS override in RunSpec.Env — the mechanism
// that lets --no-yolo blank the in-container alias without a rebuild. Uses
// RunHandler (not the recorded Calls argv) because the recorded argv only
// captures env NAMEs (see docker.envArgs) — the VALUE only appears on the
// full RunSpec the handler receives.
func TestRunContainerCarriesYoloEnvOverride(t *testing.T) {
	ws := t.TempDir()

	for _, tc := range []struct {
		name    string
		noYolo  bool
		wantVal string
	}{
		{"bypass on by default", false, "--dangerously-skip-permissions"},
		{"no-yolo blanks it", true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dc := docker.NewMock()
			var captured docker.RunSpec
			dc.RunHandler = func(_ context.Context, spec docker.RunSpec) error {
				captured = spec
				return nil
			}

			pin := config.Pin{Harness: "claude-code", Shell: "zsh"}
			opts := Options{LaunchTarget: LaunchHarness, NoYolo: tc.noYolo, Stdout: io.Discard, Stderr: io.Discard}

			err := runContainer(context.Background(), dc, "img:tag", "ctr", ws,
				workspace.Spec{Harness: "claude-code"}, pin, nil, opts)
			if err != nil {
				t.Fatalf("runContainer: %v", err)
			}

			if !envContains(captured.Env, "VIBRATOR_YOLO_ARGS", tc.wantVal) {
				t.Errorf("RunSpec.Env = %+v, want VIBRATOR_YOLO_ARGS=%q", captured.Env, tc.wantVal)
			}
		})
	}
}

// TestExecIntoContainerCarriesYoloEnvOverride mirrors the run-path test
// above for the re-entry path: exec'ing into an already-running container
// must ALSO carry the override, so toggling --no-yolo on a second `vibrate`
// invocation against a live container still takes effect (the alias lives
// in the shell process env, not baked into a long-lived container state).
func TestExecIntoContainerCarriesYoloEnvOverride(t *testing.T) {
	ws := t.TempDir()

	for _, tc := range []struct {
		name    string
		noYolo  bool
		wantVal string
	}{
		{"bypass on by default", false, "--dangerously-skip-permissions"},
		{"no-yolo blanks it", true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dc := docker.NewMock()
			var captured docker.ExecSpec
			dc.ExecHandler = func(_ context.Context, spec docker.ExecSpec) error {
				captured = spec
				return nil
			}

			pin := config.Pin{Harness: "claude-code", Shell: "zsh"}
			opts := Options{LaunchTarget: LaunchHarness, NoYolo: tc.noYolo, Stdout: io.Discard, Stderr: io.Discard}

			err := execIntoContainer(context.Background(), dc, "ctr", ws, pin, opts)
			if err != nil {
				t.Fatalf("execIntoContainer: %v", err)
			}

			if !envContains(captured.Env, "VIBRATOR_YOLO_ARGS", tc.wantVal) {
				t.Errorf("ExecSpec.Env = %+v, want VIBRATOR_YOLO_ARGS=%q", captured.Env, tc.wantVal)
			}
		})
	}
}

// envContains reports whether env has an entry matching name/value exactly.
func envContains(env []docker.EnvVar, name, value string) bool {
	for _, e := range env {
		if e.Name == name && e.Value == value {
			return true
		}
	}
	return false
}

func TestMountVolumesAndDirs(t *testing.T) {
	rs := []mount.Resolved{
		{Path: "/data/refs", ReadOnly: true},
		{Path: "/work/lib", ReadOnly: false},
	}
	vols := mountVolumes(rs)
	if len(vols) != 2 {
		t.Fatalf("got %d volumes, want 2", len(vols))
	}
	if vols[0].Host != "/data/refs" || vols[0].Container != "/data/refs" || !vols[0].ReadOnly {
		t.Fatalf("vol0 = %+v", vols[0])
	}
	if vols[1].ReadOnly {
		t.Fatalf("vol1 should be writable: %+v", vols[1])
	}
	dirs := mountDirs(rs)
	if len(dirs) != 2 || dirs[0] != "/data/refs" || dirs[1] != "/work/lib" {
		t.Fatalf("dirs = %v", dirs)
	}
}
