// Package harness models the AI agent harnesses (Claude Code, Codex,
// OpenCode, Pi) that vibrator can run inside a container.
//
// Each harness implements the Harness interface: how to install it, which
// env vars to forward for auth, where its host-side config lives, and which
// base features (Python, Node, …) it requires. The Dockerfile generator
// (internal/dockerfile) consumes these declarations to bake a harness-aware
// image.
//
// New harnesses are added by writing a new Go file with a struct
// implementing Harness and a global Registry entry. No runtime discovery —
// extension is a PR. Catalog entries per harness still come from
// catalog/<harness>/*.md (data-driven), so day-to-day "add another plugin"
// is a no-Go-code change.
package harness

import "fmt"

// Harness is the contract every supported agent harness implements. Keep it
// minimal — harness-specific surface (e.g., per-harness entrypoint hooks)
// is added later (Phase 4) once the wizard + lifecycle needs them.
type Harness interface {
	// ID is the kebab-case stable identifier used everywhere (image tags,
	// CLI flags, catalog directory name). Match catalog/<id>/.
	ID() string

	// Name is the display label shown in the wizard and `--help`.
	Name() string

	// Dockerfile returns the verbatim Dockerfile fragment that installs the
	// harness binary into the image. The fragment runs after features have
	// been installed and after the unprivileged user has been created, but
	// before catalog entries are processed. All install commands should
	// land binaries in /usr/local/bin/ so they're on PATH for every user.
	Dockerfile() string

	// AuthEnvVars returns the env var names this harness expects to be
	// present (in either the host environment or `~/.<harness>/...`) for
	// the user to authenticate. Vibrator forwards these via `docker run -e`
	// from the host. Empty list means "no env-var auth — uses OAuth /
	// browser flow".
	AuthEnvVars() []string

	// HostConfigDir returns the absolute path to the harness's config dir
	// on the host (e.g., "$HOME/.claude"). Vibrator selectively mounts
	// useful subpaths into the container so user settings persist across
	// runs. Empty string means "no host config persistence".
	HostConfigDir() string

	// RequiredFeatures lists base features (internal/feature IDs) this
	// harness needs to run. The Dockerfile generator unions these with
	// the user's resolved feature set so a harness's deps are always
	// satisfied regardless of profile/--no settings.
	RequiredFeatures() []string

	// SupportsLLMProvider reports whether the wizard should show the LLM
	// provider/model selection step for this harness.
	//
	// Return false for harnesses locked to a single provider (e.g., Claude
	// Code is Anthropic-only; the existing AuthEnvVars forwarding suffices).
	// Return true for provider-agnostic harnesses (Codex, OpenCode, Pi).
	//
	// When true, the wizard will populate `pin.LLM` with the user's choice;
	// the launch orchestrator (Phase 4e) reads it to derive container env
	// vars and to manage local-provider lifecycle.
	SupportsLLMProvider() bool

	// LLMEnvVars maps an LLM provider configuration into the container
	// environment variables this harness expects.
	//
	// Inputs are the resolved LLM choice from `pin.LLM`:
	//   - provider: canonical id ("openai", "anthropic", "ollama",
	//     "lmstudio", "openai-compat")
	//   - model:    model name as the provider expects it
	//   - baseURL:  endpoint URL (empty for provider defaults)
	//   - apiKey:   resolved key (orchestrator extracts from .vb's
	//     llm.auth.value OR $llm.auth.env on the host)
	//
	// Returns a map of env var name → value the orchestrator will
	// pass into `docker run`. Harnesses that don't support LLM
	// configuration (claude-code) return an empty map; the existing
	// AuthEnvVars surface still does its job.
	LLMEnvVars(provider, model, baseURL, apiKey string) map[string]string
}

// Registry holds every built-in harness, ordered for display in the wizard.
// Adding a new harness = appending here. Validation that catalog/<id>/
// exists for every registered harness happens via a self-check test in
// the root vibrator package (embedded_test.go).
var Registry []Harness

// Register adds a harness to the global registry. Called from each
// concrete harness's init(). Panics on duplicate ID — that's a programming
// bug, not user error, and we want it caught at startup.
func Register(h Harness) {
	for _, existing := range Registry {
		if existing.ID() == h.ID() {
			panic(fmt.Sprintf("harness: duplicate ID %q", h.ID()))
		}
	}
	Registry = append(Registry, h)
}

// ByID looks up a harness by ID. The bool is false when unknown — callers
// should treat that as a user error and surface the valid IDs.
func ByID(id string) (Harness, bool) {
	for _, h := range Registry {
		if h.ID() == id {
			return h, true
		}
	}
	return nil, false
}

// IDs returns all registered harness IDs in Registry order.
func IDs() []string {
	out := make([]string, len(Registry))
	for i, h := range Registry {
		out[i] = h.ID()
	}
	return out
}
