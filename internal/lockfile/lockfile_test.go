package lockfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireReleaseCycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".vb.lock")
	l, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	l.Release()
	l2, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire after Release: %v", err)
	}
	l2.Release()
	l2.Release() // idempotent — must not panic
}

func TestSecondAcquireFailsWithPID(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".vb.lock")
	l, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer l.Release()

	_, err = Acquire(path)
	var held *HeldError
	if !errors.As(err, &held) {
		t.Fatalf("second Acquire err = %v, want HeldError", err)
	}
	if held.PID != os.Getpid() {
		t.Errorf("held.PID = %d, want %d", held.PID, os.Getpid())
	}
}

func TestLeftoverFileWithoutHolderIsNotStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".vb.lock")
	if err := os.WriteFile(path, []byte("99999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := Acquire(path) // flock gone with its (nonexistent) process
	if err != nil {
		t.Fatalf("Acquire over leftover file: %v", err)
	}
	l.Release()
}
