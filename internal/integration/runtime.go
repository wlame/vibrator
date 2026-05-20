package integration

import (
	"context"
	"time"
)

// HostRuntime is one way an integration can run on the host machine.
//
// Implementations live alongside their integration's descriptor (e.g.
// serena.processRuntime, serena.dockerRuntime, or future
// claudemem.composeRuntime) or as generic adapters in this package
// (ExternalRuntime).
//
// Multiple HostRuntime values can be attached to a single Integration
// (Serena offers both a uvx process and a Docker container, for
// example). At any given moment at most one is active — Status reports
// per-mode.
//
// Implementations should be cheap to construct and idempotent for
// Start/Stop: starting an already-running runtime should be a no-op,
// not an error.
type HostRuntime interface {
	// Kind returns a short identifier ("docker", "process", "compose",
	// "external"). Used by the CLI to render badges and disambiguate in
	// pickers and `list` output.
	Kind() string

	// Label returns a human-readable description for the picker, e.g.,
	// "Background process (uvx, survives terminal close)".
	Label() string

	// Status reports whether THIS runtime mode is currently active. A
	// runtime can return Running=false without error — it simply means
	// the integration isn't currently running in that mode.
	Status(ctx context.Context) (RuntimeStatus, error)

	// Start launches this runtime. Should be idempotent — if already
	// running, return nil without re-spawning.
	Start(ctx context.Context) error

	// Stop halts this runtime. Idempotent — if not running, return nil.
	Stop(ctx context.Context) error

	// Logs returns up to maxBytes of the most recent log output. Empty
	// string + nil error is a valid response for runtimes that don't
	// collect logs (or when no log has been produced yet).
	Logs(ctx context.Context, maxBytes int64) (string, error)
}

// RuntimeStatus describes the state of a single HostRuntime mode.
type RuntimeStatus struct {
	// Running is true when the process/container backing this runtime
	// mode is alive right now.
	Running bool

	// PID is populated for process runtimes (0 otherwise).
	PID int

	// Container is populated for docker runtimes — usually the container
	// name (empty otherwise).
	Container string

	// StartedAt is when the runtime started, when known. Zero value
	// means "unknown".
	StartedAt time.Time

	// Detail is a free-form extra string for the CLI to display
	// (e.g., "auto-restart enabled", "external network").
	Detail string
}

// ExternalRuntime is a HostRuntime that does nothing — used when the
// integration is started and managed by the user (a system service, an
// existing compose stack started outside of vibrator, etc.).
//
// Status always reports Running=false. Detection of "actually running
// externally" is left to the Probe — see the `list` command flow for
// how that is presented.
type ExternalRuntime struct {
	// Instructions is shown by the CLI when this mode is selected, to
	// help the user set things up themselves.
	Instructions string
}

func (e *ExternalRuntime) Kind() string  { return "external" }
func (e *ExternalRuntime) Label() string { return "Externally managed — I'll start it myself" }
func (e *ExternalRuntime) Status(_ context.Context) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}
func (e *ExternalRuntime) Start(_ context.Context) error { return nil }
func (e *ExternalRuntime) Stop(_ context.Context) error  { return nil }
func (e *ExternalRuntime) Logs(_ context.Context, _ int64) (string, error) {
	return "", nil
}
