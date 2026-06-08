// Package serena manages the lifecycle of a Serena MCP host server —
// either as a detached background process (uvx) or a Docker container.
// It is pure logic with no TUI dependency; the CLI layer owns all huh forms.
package serena

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// DefaultPort is the port Serena listens on unless overridden.
	DefaultPort = 8765

	// ContainerName is the fixed Docker container name for vibrate-managed Serena.
	ContainerName = "vibrate-serena"

	// containerImage is the base image: has uv + uvx pre-installed.
	containerImage = "ghcr.io/astral-sh/uv:python3.12-bookworm-slim"

	// uvxSource is the git reference passed to uvx --from.
	uvxSource = "git+https://github.com/oraios/serena"
)

// Status represents the observed state of the Serena daemon.
type Status int

const (
	StatusStopped Status = iota // no PID file or process gone
	StatusRunning               // PID file present and process alive
	StatusStale                 // PID file present but process dead
)

// ProcessState is a snapshot of the daemon state at a point in time.
type ProcessState struct {
	Status  Status
	PID     int    // zero when Stopped/Stale
	PIDPath string // absolute path (always populated)
	LogFile string // absolute path to log file
	Port    int
}

// DataDir returns the vibrator state directory, honouring $XDG_DATA_HOME.
func DataDir() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "vibrator"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "vibrator"), nil
}

// PIDPath returns the path to the PID file for the Serena process.
func PIDPath() (string, error) {
	d, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "serena.pid"), nil
}

// LogPath returns the path to the Serena log file.
func LogPath() (string, error) {
	d, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "serena.log"), nil
}

// Probe reports whether Serena is reachable at the given port.
func Probe(port int) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/mcp", port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// Read returns the current ProcessState by checking the PID file and
// sending signal 0 to test liveness. A stale PID file is cleaned up
// automatically so the next caller sees StatusStopped.
func Read(port int) (ProcessState, error) {
	pidPath, err := PIDPath()
	if err != nil {
		return ProcessState{}, err
	}
	logPath, err := LogPath()
	if err != nil {
		return ProcessState{}, err
	}

	base := ProcessState{PIDPath: pidPath, LogFile: logPath, Port: port}

	data, err := os.ReadFile(pidPath)
	if os.IsNotExist(err) {
		base.Status = StatusStopped
		return base, nil
	}
	if err != nil {
		return ProcessState{}, fmt.Errorf("read PID file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		// Corrupt PID file — treat as stale.
		_ = os.Remove(pidPath)
		base.Status = StatusStopped
		return base, nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = os.Remove(pidPath)
		base.Status = StatusStopped
		return base, nil
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// Process is dead — stale file; clean up and report stopped.
		_ = os.Remove(pidPath)
		base.Status = StatusStale
		base.PID = pid
		return base, nil
	}

	base.Status = StatusRunning
	base.PID = pid
	return base, nil
}

// Start spawns Serena as a detached background process. The process is
// put in its own session (Setsid) so it survives the parent terminal
// closing. Stdout and stderr are appended to the log file.
// Returns the PID immediately — call Probe in a loop to wait for ready.
func Start(port int) (int, error) {
	d, err := DataDir()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(d, 0755); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", d, err)
	}

	logPath := filepath.Join(d, "serena.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return 0, fmt.Errorf("open log: %w", err)
	}

	args := []string{
		"--from", uvxSource,
		"serena", "start-mcp-server",
		"--transport=streamable-http",
		"--host=0.0.0.0",
		fmt.Sprintf("--port=%d", port),
		"--enable-web-dashboard=true",
	}
	cmd := exec.Command("uvx", args...)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, fmt.Errorf("uvx: %w", err)
	}
	// Log file handle can be closed in the parent — the child has its own fd.
	logFile.Close()

	pid := cmd.Process.Pid
	pidPath := filepath.Join(d, "serena.pid")
	// Write PID after Start() succeeds so a failed start leaves no stale file.
	// Non-fatal — the process is running even without the tracking file.
	_ = os.WriteFile(pidPath, []byte(strconv.Itoa(pid)+"\n"), 0644)
	return pid, nil
}

// Stop sends SIGTERM to the tracked process, waits up to 5 s, then
// SIGKILL. Removes the PID file on success.
func Stop(state ProcessState) error {
	if state.PID == 0 {
		return fmt.Errorf("no PID to stop (status: %v)", state.Status)
	}
	proc, err := os.FindProcess(state.PID)
	if err != nil {
		_ = os.Remove(state.PIDPath)
		return nil // already gone
	}

	_ = proc.Signal(syscall.SIGTERM)
	for i := 0; i < 25; i++ { // 5 s total
		time.Sleep(200 * time.Millisecond)
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			// Gone.
			_ = os.Remove(state.PIDPath)
			return nil
		}
	}
	// Still alive after 5 s — SIGKILL.
	_ = proc.Signal(syscall.SIGKILL)
	_ = os.Remove(state.PIDPath)
	return nil
}

// ContainerRunning reports whether the vibrate-serena Docker container
// is in the "running" state.
func ContainerRunning() bool {
	out, err := exec.Command(
		"docker", "inspect", "--format={{.State.Running}}", ContainerName,
	).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// ContainerExists reports whether the container exists (running or stopped).
func ContainerExists() bool {
	err := exec.Command("docker", "inspect", ContainerName).Run()
	return err == nil
}

// StartDocker runs Serena inside a Docker container with auto-restart.
// Port is bound to 127.0.0.1 on the host so only local clients connect.
func StartDocker(port int) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	serenaData := filepath.Join(home, ".serena")

	args := []string{
		"run", "-d",
		"--name", ContainerName,
		"--restart", "unless-stopped",
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", port, port),
		"-v", "vibrate-serena-cache:/root/.cache/uv",
		"-v", serenaData + ":/root/.serena",
		containerImage,
		"uvx", "--from", uvxSource,
		"serena", "start-mcp-server",
		"--transport=streamable-http",
		"--host=0.0.0.0",
		fmt.Sprintf("--port=%d", port),
		"--enable-web-dashboard=true",
	}
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// StopDocker stops and removes the vibrate-serena container.
func StopDocker() error {
	if err := exec.Command("docker", "stop", ContainerName).Run(); err != nil {
		return fmt.Errorf("docker stop: %w", err)
	}
	if err := exec.Command("docker", "rm", ContainerName).Run(); err != nil {
		return fmt.Errorf("docker rm: %w", err)
	}
	return nil
}

// TailLog returns the last maxBytes bytes of the log file as a string.
// Returns ("", nil) when the file doesn't exist yet.
func TailLog(maxBytes int64) (string, error) {
	logPath, err := LogPath()
	if err != nil {
		return "", err
	}
	f, err := os.Open(logPath)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	offset := fi.Size() - maxBytes
	if offset < 0 {
		offset = 0
	}
	if _, err := f.Seek(offset, 0); err != nil {
		return "", err
	}
	buf := make([]byte, maxBytes)
	n, _ := f.Read(buf)
	return string(buf[:n]), nil
}

// PollReady probes every 2 s until Serena is up or timeout is reached.
// Returns true if ready, false on timeout.
func PollReady(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if Probe(port) {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return false
}
