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
// extension is a PR. Extensions entries per harness still come from
// extensions/<harness>/*.md (data-driven), so day-to-day "add another plugin"
// is a no-Go-code change.
package harness

import "fmt"

// Harness is the contract every supported agent harness implements. Keep it
// minimal — harness-specific surface (e.g., per-harness entrypoint hooks)
// is added later (Phase 4) once the wizard + lifecycle needs them.
type Harness interface {
	// ID is the kebab-case stable identifier used everywhere (image tags,
	// CLI flags, extensions directory name). Match extensions/<id>/.
	ID() string

	// Name is the display label shown in the wizard and `--help`.
	Name() string

	// Dockerfile returns the verbatim Dockerfile fragment that installs the
	// harness binary into the image. The fragment runs after features have
	// been installed and after the unprivileged user has been created, but
	// before extension entries are processed. All install commands should
	// land binaries in /usr/local/bin/ so they're on PATH for every user.
	Dockerfile() string

	// AuthEnvVars returns the env var names this harness expects to be
	// present (in either the host environment or `~/.<harness>/...`) for
	// the user to authenticate. Vibrator forwards these via `docker run -e`
	// from the host. Empty list means "no env-var auth — uses OAuth /
	// browser flow".
	AuthEnvVars() []string

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

	// UpdateCommand returns the argv that `vibrate update` runs to
	// upgrade the harness's own CLI to its latest version, in place.
	// It's the same command users would run manually inside the
	// container — `claude update` for Claude Code, `npm install -g
	// <pkg>@latest` for npm-installed harnesses, etc.
	//
	// Return an empty slice when the harness has no idiomatic
	// self-update path (the orchestrator surfaces a "manual rebuild
	// required" error in that case). Every built-in harness today
	// supports a self-update path.
	UpdateCommand() []string

	// LaunchCommand returns the argv that `vibrate` exec's inside the
	// container by default — the harness's own CLI entry point. Bare
	// `vibrate` (and `vibrate run`) launch this directly; `vibrate
	// shell` is the escape hatch that launches the user's shell
	// instead.
	//
	// Implementations should return the canonical command users would
	// type at the shell. Empty slice is a programming error — every
	// harness has a launch command, that's what makes it a harness.
	//
	// The orchestrator prepends /usr/local/bin/claude-exec so the
	// session-start hooks (integration manifest, transport switching)
	// still run before the agent boots.
	LaunchCommand() []string

	// ExtraDirArgs returns the harness-specific argv that grants the agent
	// read access to additional directories mounted into the container
	// beyond the workspace. dirs are absolute container paths (vibrator
	// mounts each at its host path). The orchestrator appends the result
	// to the launch command. Return nil when the harness has no such
	// mechanism — its mounts are still bound; the user is just told to add
	// them inside the agent manually.
	ExtraDirArgs(dirs []string) []string

	// HostMounts returns the harness's DECLARATIVE host→container bind
	// mounts — the host-side config/auth/session state that should be
	// visible inside the container so settings and logins persist across
	// runs.
	//
	// Harnesses return these as PURE DATA: no filesystem access, no OS
	// calls. The orchestrator (internal/app) does all probing, directory
	// creation, path expansion, and the translation to a concrete docker
	// mount. This keeps harness implementations trivially testable and
	// keeps the "which harness gets host state" decision out of the
	// orchestrator (no `if harness == "claude-code"` special cases).
	//
	// Return an empty slice for "no host config persistence".
	HostMounts(ctx HostMountContext) []HostMount

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

	// PermissionBypassArgs returns the argv fragment that puts the harness
	// into its "skip approvals / YOLO" mode — vibrator's default, since the
	// container is the blast-radius boundary. nil means the harness has no
	// such flag (no bypass applied, no alias emitted).
	//
	// Single source of truth: this drives BOTH the direct-launch argv (bare
	// `vibrate`) AND the shell alias baked into the image, so the two can
	// never diverge.
	PermissionBypassArgs() []string

	// LoginFlow declares how `vibrate --login` authenticates this harness
	// inside the container, or nil when login is unsupported. See LoginFlow.
	LoginFlow() *LoginFlow
}

// MountKind tells the orchestrator how to treat a HostMount's source
// path on the host before binding it into the container.
type MountKind int

const (
	// MountFileIfExists binds the source only when it exists and is a
	// regular file. Used for single config/auth files (e.g. auth.json).
	// A missing source is a silent no-op.
	MountFileIfExists MountKind = iota

	// MountDirIfExists binds the source only when it exists and is a
	// directory. A missing source is a silent no-op.
	MountDirIfExists

	// MountDirEnsure creates the source directory (and parents) on the
	// host if missing, then binds it. Used for state dirs the container
	// must be able to write to on first run (sessions, history, …). A
	// failed mkdir is logged and the mount is skipped — never fatal.
	MountDirEnsure
)

// HostMount is a harness's declarative request to bind a host path into
// the container. See Harness.HostMounts for the contract.
//
// HostRel is interpreted relative to the host user's home directory;
// ContainerRel relative to the container user's home directory. Both
// use forward slashes and must stay within their home root — the
// orchestrator rejects any entry whose cleaned path escapes home (a
// guard against a "../../etc/…" descriptor).
type HostMount struct {
	HostRel      string
	ContainerRel string
	ReadOnly     bool
	Kind         MountKind
}

// HostMountContext carries the per-launch values a harness needs to
// compute its mount set without touching the filesystem.
type HostMountContext struct {
	// WorkspaceDir is the absolute path of the workspace being launched.
	// It is mounted at the same path on host and container, so harnesses
	// that scope state per-project (e.g. Claude Code's projects/<encoded>)
	// can derive the encoded name from it as a pure string operation.
	WorkspaceDir string
}

// LoginFlow, when non-nil, declares how `vibrate --login` authenticates a
// harness inside the container. nil means login is unsupported for the
// harness (the data-driven replacement for the old hardcoded gate).
type LoginFlow struct {
	// Command is the interactive argv run in the container to start auth,
	// e.g. []string{"claude","auth","login"}.
	Command []string

	// URLMarker is the stdout prefix the CLI prints just before the auth
	// URL; the orchestrator scrapes the URL after it and opens the host
	// browser (the container has none). "" disables scraping.
	URLMarker string

	// Writeback, when non-nil, copies auth state from a container file back
	// to a host file after login — needed when the host auth file is mounted
	// read-only (claude-code). nil means the auth file is mounted read-write
	// so login persists to the host directly.
	Writeback *AuthWriteback
}

// AuthWriteback describes a post-login container→host auth-state merge.
type AuthWriteback struct {
	// ContainerRel / HostRel are home-relative paths (forward slashes),
	// matching the HostMount convention.
	ContainerRel string
	HostRel      string

	// Fields are the JSON top-level keys merged from container into host.
	// Empty merges every top-level key — use with care: for a rich config
	// file (e.g. ~/.claude.json) that copies ALL container state (project
	// history, MCP entries, …) back to the host, so prefer naming the exact
	// auth fields. A host file that exists but fails to parse aborts the
	// writeback (never overwrite a config with a partial).
	Fields []string
}

// Registry holds every built-in harness, ordered for display in the wizard.
// Adding a new harness = appending here. Validation that extensions/<id>/
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
