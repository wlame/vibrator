package localprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// ollamaGetJSON performs a GET against url with the given timeout. When
// dst is non-nil, the response body is decoded into it as JSON. When
// dst is nil, the body is drained and discarded — useful for probes
// that only care about reachability.
//
// Named for its first caller but used by both Ollama and LM Studio.
// Centralizing here keeps the providers free of HTTP plumbing
// duplication.
func ollamaGetJSON(ctx context.Context, url string, dst any, timeout time.Duration) error {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	// Short-lived client — no keep-alive pool pollution.
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	if dst == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

// newBytesReader wraps a byte slice in an io.Reader. Tiny shim to keep
// the providers' import lists minimal.
func newBytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// commandStarter spawns `binary` with args in the background, detaching
// from the vibrate parent process so the server keeps running after
// vibrate exits.
//
// This is a package-level var so unit tests can replace it with a
// recorder (see localprovider_test.go).
var commandStarter = defaultCommandStarter

// defaultCommandStarter is the real implementation. It redirects
// stdin/stdout/stderr to /dev/null so the wizard's UI doesn't get spammed
// with server logs, and calls Start() (not Run()) so the function
// returns immediately while the server keeps running.
//
// On POSIX hosts we can't fully detach without Setsid (which would
// require platform-specific code). In practice the process survives
// vibrate exiting because we never call cmd.Wait() and never Kill().
func defaultCommandStarter(binary string, args ...string) error {
	cmd := exec.Command(binary, args...)
	// Redirect all I/O to /dev/null — the server's stdout/stderr aren't
	// useful here and would clutter the wizard view.
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		// If /dev/null can't be opened (very unusual), fall back to no
		// redirection — the user might see one-off log lines, but the
		// command will still spawn.
		cmd.Stdin = nil
		cmd.Stdout = nil
		cmd.Stderr = nil
	} else {
		cmd.Stdin = devnull
		cmd.Stdout = devnull
		cmd.Stderr = devnull
	}
	return cmd.Start()
}

// waitForServer polls `probe` until it returns nil or the timeout elapses.
// Sleeps `interval` between attempts. Returns the last probe error on
// timeout, so the caller can include it in the wrapping diagnostic.
func waitForServer(ctx context.Context, probe func(context.Context) error,
	timeout, interval time.Duration,
) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := probe(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		// Sleep is interruptible via ctx — important so a SIGINT during
		// the poll doesn't get swallowed.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	if lastErr != nil {
		return fmt.Errorf("timeout after %s: %w", timeout, lastErr)
	}
	return fmt.Errorf("timeout after %s", timeout)
}
