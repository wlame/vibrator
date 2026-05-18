// Package profile defines the named feature bundles users pick from in the
// wizard: minimal, backend, frontend, full. A profile is just a starting
// feature set — `--with X` and `--no Y` deltas (and catalog entries with deps)
// are layered on top by internal/feature.Resolve.
//
// Profiles don't carry implementation details. The Dockerfile generator
// (Phase 3) walks the resolved feature list and emits stage fragments per
// feature ID; profiles are purely UX.
package profile

import (
	"fmt"

	"github.com/wlame/vibrator/internal/feature"
)

// Profile is a named bundle of feature IDs.
type Profile struct {
	ID          string
	Name        string   // display label
	Description string   // wizard-line description
	SizeMB      int      // approximate image-size estimate (informational only)
	Features    []string // initial feature IDs (before user --with/--no deltas)
}

// ID constants. Use these instead of bare strings throughout the codebase so
// the compiler catches typos.
const (
	IDMinimal  = "minimal"
	IDBackend  = "backend"
	IDFrontend = "frontend"
	IDFull     = "full"
)

// Minimal: just the always-on base toolkit. No language toolchains, no MCP
// dependencies, no audit tools. ~150 MB image.
//
// Useful for: CI runs, tight resource environments, "I just want a shell
// with rg + jq + vim in a box".
var Minimal = Profile{
	ID:          IDMinimal,
	Name:        "Minimal",
	Description: "Just the always-on base toolkit (jq, rg, fd, vim, curl, ssh, git). No language runtimes.",
	SizeMB:      150,
	Features:    nil,
}

// Backend: language toolchains + service-side CLIs.
var Backend = Profile{
	ID:   IDBackend,
	Name: "Backend",
	Description: "Backend dev — adds Python, Go, GitHub CLI, Postgres client, ralphex. " +
		"No browser, no audit toolkit.",
	SizeMB:   600,
	Features: []string{"python", "go", "gh", "postgres-client", "ralphex"},
}

// Frontend: Node + Bun + Playwright + browser deps.
var Frontend = Profile{
	ID:   IDFrontend,
	Name: "Frontend",
	Description: "Frontend dev — adds Node.js + Bun + Playwright (Chromium). " +
		"No Python, no Go, no audit toolkit.",
	SizeMB:   1000,
	Features: []string{"node", "playwright", "gh", "ralphex"},
}

// Full: backend + frontend + audit + codex-cli. The default if the user
// picks nothing in the wizard.
var Full = Profile{
	ID:   IDFull,
	Name: "Full",
	Description: "Everything except aider — backend + frontend + audit toolkit + codex CLI. " +
		"Default when the wizard is skipped.",
	SizeMB: 2000,
	Features: []string{
		"python", "go", "node", "playwright",
		"gh", "postgres-client",
		"audit-toolkit", "codex-cli", "ralphex",
	},
}

// All is the canonical ordered list of profiles. Order is the order shown in
// the wizard — keep it small→large for easy scanning.
var All = []Profile{Minimal, Backend, Frontend, Full}

// Default is the profile applied when the user doesn't pick one. Per the plan,
// full mirrors the bash version's "everything except aider" behaviour.
var Default = Full

// indexByID lets ByID avoid linear scans and gives constant-time existence
// checks for input validation.
var indexByID = func() map[string]Profile {
	m := make(map[string]Profile, len(All))
	for _, p := range All {
		m[p.ID] = p
	}
	return m
}()

// ByID returns the profile with the given ID. The bool is false when the ID
// isn't registered.
func ByID(id string) (Profile, bool) {
	p, ok := indexByID[id]
	return p, ok
}

// IDs returns all known profile IDs in wizard display order.
func IDs() []string {
	out := make([]string, len(All))
	for i, p := range All {
		out[i] = p.ID
	}
	return out
}

// Resolve walks the profile's Features list, layering user `with` / `no`
// deltas on top via feature.Resolve. Returns the final enabled feature set
// + any deps that were auto-pulled, suitable for emitting wizard warnings.
//
// This is a thin wrapper — internal/feature owns the actual resolution
// logic; profile only contributes the initial set.
func (p Profile) Resolve(with, no []string) (feature.ResolveResult, error) {
	return feature.Resolve(p.Features, with, no)
}

// Validate ensures every feature ID in this profile's Features list refers
// to a known feature. Run at startup (via TestProfile_AllFeaturesValid)
// rather than runtime — failures here are programming bugs, not user errors.
func (p Profile) Validate() error {
	for _, f := range p.Features {
		if !feature.IsKnown(f) {
			return fmt.Errorf("profile %q references unknown feature %q", p.ID, f)
		}
	}
	return nil
}
