// Package integration defines the registry and core types for vibrator's
// host-side integrations (claude-mem, Serena, MCP servers, etc.).
//
// An Integration is the declarative description of one such integration:
// how it can run on the host (Runtimes), how to verify reachability
// (ProbeFn), what configuration it needs (AdminConfig), how harnesses
// inside the container consume it (Wiring), and how per-workspace
// credentials are minted (Workspace).
//
// Sub-packages under internal/integration/ each describe one concrete
// integration and call Register() from their init() function. Importing
// such a sub-package for side effects — e.g.
//
//	import _ "github.com/wlame/vibrator/internal/integration/serena"
//
// — is what causes the integration to appear in the global registry at
// program start.
//
// Design philosophy
//
// Most integrations only need 3 dimensions (Runtimes, Probe, Wiring) and
// can be described declaratively. A small number — claude-mem in
// particular — also have host-only secrets (DSN), per-workspace
// credential minting, and custom bootstrap logic; those plug into
// AdminConfig and WorkspaceDriver.
//
// The container side intentionally only ever sees Probe + Wiring — it
// never learns how the host runs the integration or what host secrets
// were used to bootstrap it. This is the same separation the existing
// claude-mem and Serena code uses; the registry just makes it explicit
// and reusable.
package integration

import "context"

// Integration is the metadata + behavior descriptor for one integration.
//
// Identity fields (ID, Name, Summary, DocsURL, Category) are always set.
// The behavior fields (Runtimes, ProbeFn, Wiring, AdminConfig, Workspace)
// are populated as needed — many integrations leave AdminConfig and
// Workspace nil.
type Integration struct {
	// ── Identity ────────────────────────────────────────────────────────

	// ID is the stable slug used in CLI subcommands ("serena",
	// "claude-mem"), pin-file keys, and registry lookups. Must be unique.
	ID string

	// Name is the human-readable display label ("Serena MCP").
	Name string

	// Summary is the one-line description shown in pickers and `list`.
	Summary string

	// DocsURL points to the primary documentation for the integration.
	DocsURL string

	// Category groups related integrations in UIs ("mcp-tools", "memory",
	// "observability"). Freeform — no fixed taxonomy.
	Category string

	// ── Host-side ───────────────────────────────────────────────────────

	// Runtimes lists the available ways to run this integration on the
	// host. At least one entry should be present. When more than one is
	// present, the CLI presents a picker when starting from scratch.
	//
	// At runtime, at most one of the listed runtimes is active. The CLI
	// detects which (if any) by calling Status() on each.
	Runtimes []HostRuntime

	// ProbeFn returns a Probe for the integration's current state. For
	// static probes (URL fixed at compile time), it returns the same
	// value on every call; for dynamic probes (URL derived from admin
	// config), it loads the config on each call. Callers MUST NOT cache
	// the returned Probe.
	//
	// Returning (nil, nil) means "no probe possible right now" (e.g.,
	// not configured yet) — the CLI silently skips the reachability check.
	ProbeFn func(ctx context.Context) (Probe, error)

	// ── Container-side ──────────────────────────────────────────────────

	// Wiring is the list of harness-specific consumption descriptors:
	// MCP entries, env vars, etc. One Integration may have multiple
	// wirings (one per harness, or one shared with Harness="*").
	Wiring []Wiring

	// ── Optional configuration ──────────────────────────────────────────

	// AdminConfig declares the host-side configuration file this
	// integration uses, if any. nil means no admin config is needed.
	AdminConfig *AdminConfigSchema

	// Workspace, when non-nil, mints/rotates per-workspace credentials
	// (stored under [prereqs.<id>] in the workspace .vb file).
	Workspace WorkspaceDriver
}
