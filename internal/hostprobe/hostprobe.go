// Package hostprobe scans the host machine for already-installed agent
// tools, plugins, skills, MCP servers, and the like — so the wizard can
// pre-check items the user already has, and the launch path can verify
// the host carries what the chosen catalog selections expect.
//
// # Why hostprobe is separate from the catalog
//
// Hostprobe is intentionally catalog-agnostic. It returns raw host-side
// identifiers (e.g., "claude-mem", "context7", "serena") plus diagnostic
// notes. Mapping those identifiers to specific catalog entries is the
// catalog package's responsibility — see catalog.MatchHostIDs.
//
// This split lets us add new probers without touching the catalog, and
// makes hostprobe unit-testable against a `t.TempDir()` rather than
// requiring a full catalog fixture.
//
// # Best-effort
//
// Detection is intentionally lossy: if a harness reorganizes its config
// schema (claude-code already did this once — moving from settings.json's
// `enabledPlugins` to plugins/installed_plugins.json), we still want the
// wizard to function. Probers report whatever they can extract and
// silently skip what they can't parse. Notes can be used to surface
// degraded-detection cases to the user later.
//
// # Probers run on the HOST
//
// `vibrate` runs on the host machine, so probers read paths like
// `$HOME/.claude/`. Inside a vibrator-managed container the same call
// would see a different filesystem — but vibrate itself never runs there.
package hostprobe

import (
	"sort"
)

// Detected is the result of one prober's scan. All fields are optional and
// Installed=false means "harness home directory not present; everything
// else is zero".
type Detected struct {
	// HarnessID identifies which harness was probed. Always populated.
	HarnessID string

	// Installed is true when the harness's home directory exists. Probers
	// MAY also use auxiliary signals (binary on PATH for "pi", for
	// example), but the home directory is the canonical signal.
	Installed bool

	// HomeDir is the resolved absolute path the prober looked at. Always
	// populated (even when Installed=false — useful for diagnostics).
	HomeDir string

	// PluginIDs is the set of raw host-side identifiers discovered. NOT
	// yet mapped to catalog entries — the catalog package does that.
	// Sorted for stable iteration.
	PluginIDs []string

	// MCPServers is the set of MCP server names discovered (where the
	// harness has a separate MCP config — currently just claude-code).
	// Sorted for stable iteration.
	MCPServers []string

	// Notes is free-form diagnostic output describing where data was found
	// and how. Used by `vibrate prereqs status` and wizard's verbose mode.
	Notes []string
}

// Prober scans one harness's host config.
//
// Implementations live in per-harness files (claudecode.go, codex.go, ...)
// and register themselves via init() so the global Registry stays in sync
// with what `internal/harness/all` pulls in.
//
// Probe MUST NOT return an error for "harness not installed" — that's a
// normal state, signalled by Installed=false. Errors are reserved for
// genuine failures (e.g., a config file exists but won't parse).
type Prober interface {
	// HarnessID is the harness identifier used in the registry, matching
	// the corresponding entry in internal/harness.Registry.
	HarnessID() string

	// Probe runs the host scan. homeBase is $HOME (or a test fake).
	Probe(homeBase string) (Detected, error)
}

// Registry holds all registered probers, keyed by harness ID. Probers
// register via the package-level Register() call from init(); ProbeAll
// iterates this map.
var Registry = map[string]Prober{}

// Register adds a Prober to the global Registry. Panics on duplicate
// HarnessID — duplicates indicate a programming bug (two files claiming
// the same harness).
func Register(p Prober) {
	if p == nil {
		panic("hostprobe.Register: nil prober")
	}
	id := p.HarnessID()
	if id == "" {
		panic("hostprobe.Register: empty HarnessID")
	}
	if _, dup := Registry[id]; dup {
		panic("hostprobe.Register: duplicate harness id " + id)
	}
	Registry[id] = p
}

// ByID returns the prober for the given harness ID, or (nil, false).
func ByID(id string) (Prober, bool) {
	p, ok := Registry[id]
	return p, ok
}

// HarnessIDs returns the IDs of all registered probers in lexicographic
// order. Stable iteration matters for the wizard's grouped display.
func HarnessIDs() []string {
	ids := make([]string, 0, len(Registry))
	for id := range Registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ProbeAll runs every registered prober and returns the results keyed by
// harness ID. A single prober's error doesn't abort the rest — the
// resulting map carries whichever results succeeded, and the returned
// error (if any) describes the first failure for caller logging.
func ProbeAll(homeBase string) (map[string]Detected, error) {
	out := make(map[string]Detected, len(Registry))
	var firstErr error
	for _, id := range HarnessIDs() {
		p := Registry[id]
		d, err := p.Probe(homeBase)
		out[id] = d
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return out, firstErr
}
