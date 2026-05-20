package serena

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestRead_NoPIDFile asserts Read returns Stopped (not error) when
// the PID file is missing. This is the fresh-install case.
func TestRead_NoPIDFile(t *testing.T) {
	// Direct DataDir at an empty tempdir so we don't read the real
	// user's PID file.
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	state, err := Read(DefaultPort)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != StatusStopped {
		t.Errorf("Status = %v, want StatusStopped on fresh install", state.Status)
	}
	if state.PID != 0 {
		t.Errorf("PID = %d, want 0 on fresh install", state.PID)
	}
}

// TestRead_CorruptPIDFile asserts a non-numeric PID file is treated
// as stale (and cleaned up), reporting Stopped on the next call.
func TestRead_CorruptPIDFile(t *testing.T) {
	d := t.TempDir()
	t.Setenv("XDG_DATA_HOME", d)

	pidPath := filepath.Join(d, "vibrator", "serena.pid")
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(pidPath, []byte("not-a-pid"), 0o644); err != nil {
		t.Fatalf("write corrupt pid: %v", err)
	}

	state, err := Read(DefaultPort)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != StatusStopped {
		t.Errorf("Status = %v, want StatusStopped on corrupt PID", state.Status)
	}
	// The corrupt file should have been cleaned up.
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("corrupt PID file should have been removed (stat err: %v)", err)
	}
}

// TestRead_StalePIDFile asserts a PID file pointing at a dead process
// is recognized and reported as Stale + cleaned up.
func TestRead_StalePIDFile(t *testing.T) {
	d := t.TempDir()
	t.Setenv("XDG_DATA_HOME", d)

	pidPath := filepath.Join(d, "vibrator", "serena.pid")
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// PID 1 always exists; we want a definitely-dead one. Use a
	// huge integer near MaxInt32 that no real OS would assign.
	deadPID := 2147483640
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(deadPID)+"\n"), 0o644); err != nil {
		t.Fatalf("write pid: %v", err)
	}

	state, err := Read(DefaultPort)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// Read should detect the dead process. Depending on the platform
	// signal-0 behaviour, this may surface as Stale (PID captured) or
	// Stopped (file removed quietly). Either is correct cleanup;
	// what matters is that PID file is no longer there.
	if state.Status == StatusRunning {
		t.Errorf("Status = StatusRunning for dead PID %d", deadPID)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("stale PID file should have been removed (stat err: %v)", err)
	}
}

// TestDataDir_XDGOverride confirms XDG_DATA_HOME wins over $HOME.
func TestDataDir_XDGOverride(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/xdgdata")
	got, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if got != "/tmp/xdgdata/vibrator" {
		t.Errorf("DataDir = %q, want /tmp/xdgdata/vibrator", got)
	}
}

// TestPIDPath uses DataDir's resolution under a temp HOME.
func TestPIDPath(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/xdgdata")
	got, err := PIDPath()
	if err != nil {
		t.Fatalf("PIDPath: %v", err)
	}
	if !strings.HasSuffix(got, "vibrator/serena.pid") {
		t.Errorf("PIDPath = %q, want suffix vibrator/serena.pid", got)
	}
}

// TestTailLog_NoFile asserts an empty string + nil error on a
// not-yet-created log file (fresh install).
func TestTailLog_NoFile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	got, err := TailLog(1024)
	if err != nil {
		t.Errorf("TailLog: %v", err)
	}
	if got != "" {
		t.Errorf("TailLog = %q on fresh install, want empty", got)
	}
}

// TestTailLog_TrimsToMaxBytes asserts we don't return more bytes
// than requested.
func TestTailLog_TrimsToMaxBytes(t *testing.T) {
	d := t.TempDir()
	t.Setenv("XDG_DATA_HOME", d)

	logPath, err := LogPath()
	if err != nil {
		t.Fatalf("LogPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write 200 bytes; ask for the last 50.
	body := strings.Repeat("a", 200)
	if err := os.WriteFile(logPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	got, err := TailLog(50)
	if err != nil {
		t.Fatalf("TailLog: %v", err)
	}
	if len(got) != 50 {
		t.Errorf("TailLog returned %d bytes, want 50", len(got))
	}
}
