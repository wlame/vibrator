// Package localprovider models locally-hosted LLM servers (Ollama,
// LM Studio, etc.) as a uniform interface, so the wizard and the launch
// orchestrator can treat them interchangeably.
//
// Two layers share the same Provider interface:
//
//   - Wizard (Phase 4c): probes reachability and enumerates locally-
//     downloaded models so the user can pick one.
//
//   - Launch orchestrator (Phase 4e): calls EnsureRunning() right before
//     `docker run`. The provider starts the host-side server if needed,
//     waits for it to respond, then returns control.
//
// Why split this from `wizard` and `prereq`?
//
//   - Reusability: both the wizard (enumeration) and the orchestrator
//     (auto-start) need the same per-provider knowledge.
//
//   - Testability: the wizard imports localprovider but not the other way
//     around. Each provider's HTTP plumbing is tested in isolation with
//     httptest.NewServer.
//
//   - Forward fit: new providers (e.g., MLX, vLLM) drop in by implementing
//     Provider and calling Register().
package localprovider

import (
	"context"
	"sort"
)

// Provider is implemented by each supported local LLM server.
//
// All methods take a context for cancellation/timeout. The `url` argument
// is the base URL the user configured (e.g., "http://host.docker.internal:11434"
// for Ollama) — pass DefaultURL() if you don't have one.
type Provider interface {
	// ID is the stable identifier persisted in `.vb` under `[llm].provider`.
	// Lowercase kebab-case. Examples: "ollama", "lmstudio".
	ID() string

	// Name is the display string shown in the wizard. Examples:
	// "Ollama", "LM Studio".
	Name() string

	// DefaultURL is the URL the wizard pre-fills. Examples:
	// "http://host.docker.internal:11434" (Ollama), ":1234" (LM Studio).
	//
	// We use host.docker.internal so the same URL works whether the call
	// originates from the host (wizard probing the local server) or from
	// inside the container (the agent harness talking to it at runtime).
	DefaultURL() string

	// HostBinary is the executable name we LookPath for to start the
	// server (e.g., "ollama", "lms"). Empty string means "no CLI-driven
	// startup; user must start the server manually".
	HostBinary() string

	// Probe checks whether the server at url responds to its enumeration
	// endpoint. Fast (~2s timeout). Returns nil on success.
	Probe(ctx context.Context, url string) error

	// ListLocalModels returns the names of models already downloaded /
	// available on this server. Used by the wizard to populate the model
	// picker. Sorted lexicographically for stable rendering.
	ListLocalModels(ctx context.Context, url string) ([]string, error)

	// EnsureRunning is the launch-orchestrator entry point. It:
	//   1. Probes url. If reachable, returns nil immediately.
	//   2. If unreachable AND HostBinary is on PATH, spawns the server
	//      in the background.
	//   3. Polls url until it responds (up to ~10s).
	//   4. If `model` is non-empty, ensures the model is loaded/pulled.
	//
	// Returns an error if the server can't be reached after startup.
	// Callers (the launch orchestrator) should abort `vibrate` on error
	// rather than running a container that will immediately fail.
	EnsureRunning(ctx context.Context, url, model string) error
}

// Registry is the global registry of providers, keyed by ID. Probers
// register themselves via init() in their respective source files.
var Registry = map[string]Provider{}

// Register adds a provider to the global registry. Panics on duplicate ID
// (same convention as feature/harness/prereq).
func Register(p Provider) {
	if p == nil {
		panic("localprovider.Register: nil provider")
	}
	id := p.ID()
	if id == "" {
		panic("localprovider.Register: empty ID")
	}
	if _, dup := Registry[id]; dup {
		panic("localprovider.Register: duplicate id " + id)
	}
	Registry[id] = p
}

// ByID returns the provider with the given ID, or (nil, false).
func ByID(id string) (Provider, bool) {
	p, ok := Registry[id]
	return p, ok
}

// IDs returns the registered provider IDs in lexicographic order.
// Used by the wizard to render the provider-type picker.
func IDs() []string {
	ids := make([]string, 0, len(Registry))
	for id := range Registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
