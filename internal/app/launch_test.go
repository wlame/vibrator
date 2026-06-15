package app

import (
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
