package prereq

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// defaultProbeTimeout caps individual Verifier probes. Kept short so a
// wedged host doesn't block the wizard or launch path. Per-verifier Timeout
// fields override.
const defaultProbeTimeout = 3 * time.Second

// HTTPVerify probes an HTTP endpoint and treats any status code in
// ExpectStatus (default: 200..399) as success. We deliberately accept the
// 4xx range as well because many servers don't expose a `/health` route and
// will return 404 for the probe URL — but 404 still means the server is up.
// Callers that need exact-200 semantics can set ExpectStatus explicitly.
type HTTPVerify struct {
	// URL is the full URL to probe. Required.
	URL string

	// ExpectStatus is the set of accepted HTTP status codes. Empty = accept
	// any code in 200..499 (server replied = server up).
	ExpectStatus []int

	// Timeout per request. Zero = defaultProbeTimeout.
	Timeout time.Duration

	// BearerToken, if non-empty, is sent as `Authorization: Bearer <token>`.
	BearerToken string

	// Hint is the user-facing follow-up shown on failure. Optional.
	Hint string
}

// Verify implements Verifier. Returns OK=true when the URL responds with an
// accepted status code, OK=false on transport errors or unexpected codes.
func (h HTTPVerify) Verify(ctx context.Context) Result {
	if h.URL == "" {
		return Result{OK: false, Message: "HTTPVerify: URL is empty", Hint: h.Hint}
	}
	timeout := h.Timeout
	if timeout == 0 {
		timeout = defaultProbeTimeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, h.URL, nil)
	if err != nil {
		return Result{OK: false, Message: fmt.Sprintf("invalid URL %q: %v", h.URL, err), Hint: h.Hint}
	}
	if h.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+h.BearerToken)
	}

	// http.Client without keep-alive — short-lived probe, don't pollute the
	// process's connection pool.
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return Result{
			OK:      false,
			Message: fmt.Sprintf("unreachable: %v", err),
			Hint:    h.Hint,
		}
	}
	defer resp.Body.Close()
	// Drain the body so the connection can be reused or closed cleanly.
	_, _ = io.Copy(io.Discard, resp.Body)

	if h.isAccepted(resp.StatusCode) {
		return Result{OK: true, Message: fmt.Sprintf("reachable (HTTP %d)", resp.StatusCode)}
	}
	return Result{
		OK:      false,
		Message: fmt.Sprintf("unexpected status: HTTP %d", resp.StatusCode),
		Hint:    h.Hint,
	}
}

// isAccepted reports whether code matches ExpectStatus (or the default
// 200..499 band when ExpectStatus is empty).
func (h HTTPVerify) isAccepted(code int) bool {
	if len(h.ExpectStatus) == 0 {
		return code >= 200 && code < 500
	}
	for _, want := range h.ExpectStatus {
		if code == want {
			return true
		}
	}
	return false
}

// CommandVerify runs a host command and checks its exit code. Useful for
// probes like `which claude` or `gh auth status`. The command MUST be safe
// to run multiple times (no side effects).
//
// Stdout/stderr are discarded. If you want to surface the output, use
// CommandVerify only as a binary signal; build a separate diagnostic
// helper for verbose output.
type CommandVerify struct {
	// Args is the command + arguments. Args[0] is resolved via $PATH.
	// Required (len >= 1).
	Args []string

	// ExpectExit is the success exit code (default 0).
	ExpectExit int

	// Timeout per probe. Zero = defaultProbeTimeout.
	Timeout time.Duration

	// Hint is the user-facing follow-up shown on failure. Optional.
	Hint string
}

// Verify implements Verifier.
func (c CommandVerify) Verify(ctx context.Context) Result {
	if len(c.Args) == 0 {
		return Result{OK: false, Message: "CommandVerify: Args is empty", Hint: c.Hint}
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = defaultProbeTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, c.Args[0], c.Args[1:]...)
	err := cmd.Run()

	// Resolve exit code: zero on success, the ExitError's code on a non-zero
	// exit, or -1 for transport-style errors (binary not found, killed, etc).
	exitCode := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		} else {
			return Result{
				OK:      false,
				Message: fmt.Sprintf("command failed to start: %v", err),
				Hint:    c.Hint,
			}
		}
	}

	if exitCode == c.ExpectExit {
		return Result{
			OK:      true,
			Message: fmt.Sprintf("`%s` exited %d", strings.Join(c.Args, " "), exitCode),
		}
	}
	return Result{
		OK:      false,
		Message: fmt.Sprintf("`%s` exited %d (wanted %d)", strings.Join(c.Args, " "), exitCode, c.ExpectExit),
		Hint:    c.Hint,
	}
}

// FileVerify checks for file presence (or absence). Commonly used to assert
// a CLI tool's config file exists (e.g., `~/.claude/settings.json` after the
// user authenticated) before a feature that depends on it is enabled.
type FileVerify struct {
	// Path to check.
	Path string

	// MustExist=true: pass when file exists. MustExist=false: pass when file
	// is absent.
	MustExist bool

	// Hint is the user-facing follow-up shown on failure. Optional.
	Hint string
}

// Verify implements Verifier.
func (f FileVerify) Verify(_ context.Context) Result {
	_, err := os.Stat(f.Path)
	exists := err == nil
	switch {
	case f.MustExist && exists:
		return Result{OK: true, Message: fmt.Sprintf("file present: %s", f.Path)}
	case f.MustExist && !exists:
		return Result{OK: false, Message: fmt.Sprintf("file missing: %s", f.Path), Hint: f.Hint}
	case !f.MustExist && exists:
		return Result{OK: false, Message: fmt.Sprintf("file should be absent but exists: %s", f.Path), Hint: f.Hint}
	default:
		return Result{OK: true, Message: fmt.Sprintf("file correctly absent: %s", f.Path)}
	}
}

// VerifierFunc adapts a plain function into a Verifier. Useful when you need
// to compose multiple checks (e.g., "URL reachable AND cached key present").
type VerifierFunc func(ctx context.Context) Result

// Verify implements Verifier.
func (f VerifierFunc) Verify(ctx context.Context) Result { return f(ctx) }
