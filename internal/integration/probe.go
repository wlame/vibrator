package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Probe verifies that an integration is reachable. Probes are
// runtime-independent — they don't care HOW the integration is running,
// only whether it responds.
//
// This decoupling is intentional: the container side only ever uses
// Probe to decide if a host service is up. It never needs to know
// whether the host runs a Docker container, a process, or has the user
// managing things externally.
type Probe interface {
	// Check returns nil if reachable, error otherwise.
	Check(ctx context.Context) error

	// Wait polls Check until success or timeout. Implementations must
	// honour ctx cancellation. Returns the last error on timeout.
	Wait(ctx context.Context, timeout time.Duration) error

	// Describe returns a human-readable description (typically the URL).
	// Used in `list` output to show what we're probing.
	Describe() string
}

// HTTPProbe is a Probe that issues an HTTP GET and treats any response
// with status < 500 as success. The intent is "is the daemon listening
// and responding" — application-level 4xx errors are fine because they
// still prove the server is up.
type HTTPProbe struct {
	// URL is the full URL to probe, including scheme.
	URL string

	// Timeout is the per-attempt timeout. Defaults to 2 s if zero.
	Timeout time.Duration
}

// Check implements Probe.
func (p HTTPProbe) Check(ctx context.Context) error {
	timeout := p.Timeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, "GET", p.URL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", p.URL, err)
	}
	// Drain and close the body so the underlying connection is returned to
	// the pool, enabling reuse on the next polling iteration.
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 500 {
		return fmt.Errorf("GET %s: status %d", p.URL, resp.StatusCode)
	}
	return nil
}

// Wait implements Probe — polls Check every 2 seconds until success or
// the timeout fires.
func (p HTTPProbe) Wait(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := p.Check(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("probe %s: timeout after %s", p.URL, timeout)
	}
	return lastErr
}

// Describe implements Probe.
func (p HTTPProbe) Describe() string { return p.URL }
