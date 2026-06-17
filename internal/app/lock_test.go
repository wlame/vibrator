package app

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/lockfile"
)

// TestRun_FailsFastWhenWorkspaceLockHeld exercises the fail-fast contention
// path end to end: another process (simulated here by acquiring the lock
// directly in-process — flock attaches to the open file description, so a
// second Acquire in the SAME process on a fresh fd still conflicts; see
// internal/lockfile's tests) already holds .vb.lock for the workspace, so
// Run must refuse to proceed rather than racing the wizard/pin-write/build/
// create phase against it.
func TestRun_FailsFastWhenWorkspaceLockHeld(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// Simulate a concurrent `vibrate` already holding the workspace lock.
	held, err := lockfile.Acquire(filepath.Join(dir, ".vb.lock"))
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer held.Release()

	// Install the launch-seam stubs so that IF Run somehow reached the
	// build/run/exec phase, that would be recorded rather than silently
	// performing real docker work — this proves the lock check short-
	// circuits before any mutating step, not just that the error text
	// happens to look right.
	probe := installLaunchStubs(t)

	err = Run(context.Background(), Options{
		Harness:  "claude-code", // pinned so nothing short of the lock blocks Run
		NoWizard: true,
		Stdout:   &bytes.Buffer{},
		Stderr:   &bytes.Buffer{},
		Stdin:    strings.NewReader(""),
	})
	if err == nil {
		t.Fatal("Run: want error, got nil")
	}
	if !strings.Contains(err.Error(), "another vibrate is running") {
		t.Errorf("Run err = %q, want it to mention %q", err.Error(), "another vibrate is running")
	}
	if probe.built || probe.ran || probe.execed {
		t.Errorf("Run performed launch work despite lock contention: %+v", probe)
	}
}
