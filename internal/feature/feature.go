// Package feature defines the build-time capability layers that compose into
// a vibrator Docker image: language toolchains (python, go, node), browser
// runtime (playwright), CLIs (gh, postgres-client), and tool bundles
// (audit-toolkit).
//
// Features are coarse, image-shaping units. Catalog entries (plugins, MCP
// servers, skills — see internal/catalog) reference features as deps to
// declare "I need Node.js in the image". The Resolve function walks those
// declarations and auto-enables transitive deps with explicit warnings.
//
// Features deliberately don't track install commands themselves — those live
// in templates/features/*.tmpl (Phase 3). This package is pure data + logic
// so it can be tested without touching the filesystem or docker.
package feature

import (
	"fmt"
	"sort"
)

// Feature is a single image capability layer. Identified by ID; the rest is
// documentation and image-sizing hints surfaced in the wizard.
type Feature struct {
	ID          string
	Name        string   // display label, e.g. "Python 3.13"
	Description string   // one-line summary for the wizard
	SizeMB      int      // approximate image-size impact (best-effort)
	Deps        []string // direct feature IDs required by this one
}

// Registry is the canonical list of features. Order is significant — it
// drives wizard rendering and explain output. Keep alphabetical-ish within
// categories: language toolchains first, then runtime, then CLIs, then bundles.
//
// Adding a feature here requires a corresponding entry in
// templates/features/<id>.tmpl (Phase 3 and onward) and any catalog entries
// that should depend on it.
var Registry = []Feature{
	{
		ID:          "python",
		Name:        "Python 3.13",
		Description: "Python 3.13 via uv; pulls a pre-built CPython from python-build-standalone.",
		SizeMB:      100,
	},
	{
		ID:          "go",
		Name:        "Go toolchain",
		Description: "Go compiler + standard library (latest stable).",
		SizeMB:      200,
	},
	{
		ID:          "node",
		Name:        "Node.js + Bun",
		Description: "Node.js 22 + Bun. Required by most JS-based MCP servers (claude-mem, playwright).",
		SizeMB:      150,
	},
	{
		ID:          "playwright",
		Name:        "Playwright + Chromium",
		Description: "Chromium binary + Playwright MCP for browser automation.",
		SizeMB:      500,
		Deps:        []string{"node"},
	},
	{
		ID:          "postgres-client",
		Name:        "Postgres client",
		Description: "psql, pg_dump, pg_restore — for talking to host-side Postgres (e.g., claude-mem).",
		SizeMB:      30,
	},
	{
		ID:          "gh",
		Name:        "GitHub CLI",
		Description: "`gh` for PRs, issues, releases — installs from the official apt repo.",
		SizeMB:      20,
	},
	{
		ID:          "audit-toolkit",
		Name:        "Production audit toolkit",
		Description: "trivy, syft, grype, semgrep, gitleaks, trufflehog, osv-scanner, checkov, dockle, scc, lizard.",
		SizeMB:      400,
		Deps:        []string{"python"},
	},
	{
		ID:          "codex-cli",
		Name:        "OpenAI Codex CLI",
		Description: "`codex` binary for cross-model code review (used by /planning:exec).",
		SizeMB:      30,
		Deps:        []string{"node"},
	},
	{
		ID:          "ralphex",
		Name:        "ralphex",
		Description: "Autonomous coding loop — executes implementation plans task-by-task in fresh sessions.",
		SizeMB:      20,
	},
	{
		ID:          "aider",
		Name:        "aider AI pair programming",
		Description: "`aider-chat` via uv tool install. Opt-in — not part of any default profile.",
		SizeMB:      80,
		Deps:        []string{"python"},
	},
}

// indexByID lets ByID / IsKnown / Resolve avoid linear scans.
var indexByID = func() map[string]Feature {
	m := make(map[string]Feature, len(Registry))
	for _, f := range Registry {
		m[f.ID] = f
	}
	return m
}()

// ByID returns the feature with the given ID. The bool is false when the ID
// is unknown — callers should treat that as a user error and surface the
// list of valid IDs.
func ByID(id string) (Feature, bool) {
	f, ok := indexByID[id]
	return f, ok
}

// IsKnown reports whether id is a registered feature.
func IsKnown(id string) bool {
	_, ok := indexByID[id]
	return ok
}

// IDs returns all known feature IDs in Registry order.
func IDs() []string {
	out := make([]string, len(Registry))
	for i, f := range Registry {
		out[i] = f.ID
	}
	return out
}

// ResolveResult holds the outcome of dependency resolution. AutoEnabled is
// the subset of Enabled that was implicitly turned on to satisfy a dep,
// suitable for emitting a "feature X required Y, auto-enabling" warning.
type ResolveResult struct {
	Enabled     []string // final enabled set, sorted lexicographically
	AutoEnabled []string // IDs implicitly enabled to satisfy deps, sorted
}

// Resolve computes the final feature set from a profile's initial features
// plus user `with` additions, minus `no` removals, transitively pulling in
// any missing deps. Missing deps are auto-enabled with the AutoEnabled
// signal so the caller can warn.
//
// Conflict policy: when `--no=X --with=Y` and Y depends on X, X gets
// auto-re-enabled. The bash version did the same — auto-enabling deps is
// considered more user-friendly than failing. To truly disable X, the user
// must also pass `--no=Y`.
//
// Returns an error if any ID in initial/with/no is unknown.
func Resolve(initial, with, no []string) (ResolveResult, error) {
	// Validate all IDs up front so we can fail fast with a clear message.
	for _, set := range []struct {
		name string
		ids  []string
	}{
		{"initial", initial},
		{"with", with},
		{"no", no},
	} {
		for _, id := range set.ids {
			if !IsKnown(id) {
				return ResolveResult{}, fmt.Errorf("unknown feature %q in %s (valid: %v)",
					id, set.name, IDs())
			}
		}
	}

	// Apply user choices: start from initial, add `with`, remove `no`.
	enabled := make(map[string]bool, len(initial))
	for _, id := range initial {
		enabled[id] = true
	}
	for _, id := range with {
		enabled[id] = true
	}
	for _, id := range no {
		delete(enabled, id)
	}

	// Transitive dep resolution. We iterate until nothing new gets added,
	// because a dep may itself have deps. Track which IDs were auto-enabled
	// (i.e., not in the post-with/no set but pulled in by a dep walk).
	autoSet := make(map[string]bool)
	for {
		changed := false
		for id := range enabled {
			f, _ := indexByID[id] // safe: validated above
			for _, dep := range f.Deps {
				if !enabled[dep] {
					enabled[dep] = true
					autoSet[dep] = true
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}

	return ResolveResult{
		Enabled:     sortedKeys(enabled),
		AutoEnabled: sortedKeys(autoSet),
	}, nil
}

// sortedKeys returns the keys of m sorted lexicographically.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
