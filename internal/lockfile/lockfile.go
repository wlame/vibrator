// Package lockfile provides a fail-fast, flock(2)-based advisory lock for
// serializing vibrate's mutating setup phase per workspace.
//
// Design: stdlib syscall.Flock, not a third-party lock library — vibrator
// is Unix-only (Windows runs via WSL2) and fail-fast semantics need no
// timeout/retry machinery. The lock lives and dies with the file
// descriptor: a crashed or killed process releases automatically, so a
// leftover lock FILE is harmless and never means "stale lock".
package lockfile

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// HeldError reports that another process holds the lock.
type HeldError struct {
	PID  int    // holder's pid as recorded in the lock file (0 if unreadable)
	Path string // the lock file
}

func (e *HeldError) Error() string {
	if e.PID > 0 {
		return fmt.Sprintf("lock %s is held by pid %d", e.Path, e.PID)
	}
	return fmt.Sprintf("lock %s is held by another process", e.Path)
}

// Lock is a held workspace lock. Release is idempotent.
type Lock struct {
	f    *os.File
	once sync.Once
}

// Acquire takes an exclusive non-blocking flock on path, creating the file
// if needed, and records the caller's pid in it for diagnostics. Returns
// *HeldError (wrapped) when another process already holds it.
func Acquire(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		pid := readPID(f)
		f.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, &HeldError{PID: pid, Path: path}
		}
		return nil, fmt.Errorf("flock %s: %w", path, err)
	}
	// Record our pid for the contention message. Best-effort.
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	return &Lock{f: f}, nil
}

// Release unlocks and closes the fd. The lock file is left in place —
// removing it would race a concurrent Acquire on the same path.
func (l *Lock) Release() {
	l.once.Do(func() {
		_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
		_ = l.f.Close()
	})
}

func readPID(f *os.File) int {
	buf := make([]byte, 32)
	n, err := f.ReadAt(buf, 0)
	if n == 0 && err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	if err != nil {
		return 0
	}
	return pid
}
