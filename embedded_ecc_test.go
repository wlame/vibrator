package vibrator_test

// Contract tests for the ECC ("Everything Claude Code") extension family.
//
// ECC ships as one extension per (harness, profile) — e.g.
// claude-code/ecc-developer, codex/ecc-core, opencode/ecc-full. Every entry
// drives the same upstream installer (scripts/install-apply.js) at the same
// pinned commit, differing only in the --target (per harness) and --profile
// (per entry suffix) flags. These tests pin that contract so an accidental
// edit — a drifted SHA, a wrong target, a typo'd profile — fails here rather
// than at docker-build time inside a container.

import (
	"regexp"
	"strings"
	"testing"

	vibrator "github.com/wlame/vibrator"
	"github.com/wlame/vibrator/internal/extensions"
)

// eccTargetForHarness maps a vibrator harness directory to the
// install-apply.js --target value ECC expects for it. Only the harnesses
// ECC actually supports appear here; pi has no ECC adapter (it gets a
// documentation stub with no install snippet, skipped by these tests).
var eccTargetForHarness = map[string]string{
	"claude-code": "claude",
	"codex":       "codex",
	"opencode":    "opencode",
}

// eccKnownProfiles is the set of ECC manifest profiles we expose as entries.
// Mirrors manifests/install-profiles.json upstream.
var eccKnownProfiles = map[string]bool{
	"minimal":   true,
	"core":      true,
	"developer": true,
	"security":  true,
	"research":  true,
	"full":      true,
}

var eccRefLine = regexp.MustCompile(`(?m)^\s*ECC_REF=([0-9a-f]{40})\s*$`)

// eccEntries returns every loaded entry whose ID starts with "ecc-".
func eccEntries(t *testing.T) []*extensions.Entry {
	t.Helper()
	all, err := extensions.LoadAll(vibrator.ExtensionsFS)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	var out []*extensions.Entry
	for _, e := range all {
		if strings.HasPrefix(e.ID, "ecc-") {
			out = append(out, e)
		}
	}
	return out
}

// TestECC_EntriesPresent ensures the ECC family didn't vanish — at minimum
// the headline developer profile must exist for claude-code.
func TestECC_EntriesPresent(t *testing.T) {
	var hasClaudeDeveloper bool
	for _, e := range eccEntries(t) {
		if e.Harness == "claude-code" && e.ID == "ecc-developer" {
			hasClaudeDeveloper = true
		}
	}
	if !hasClaudeDeveloper {
		t.Errorf("expected claude-code/ecc-developer entry to exist")
	}
}

// TestECC_InstallContract asserts each ECC entry installs via the unified
// upstream installer with the harness-correct --target and a valid --profile
// matching the entry's ID suffix, declares the node feature dep, and is a
// plugin-kind entry.
func TestECC_InstallContract(t *testing.T) {
	for _, e := range eccEntries(t) {
		target, ok := eccTargetForHarness[e.Harness]
		if !ok {
			continue // e.g. a pi stub with no real install snippet
		}

		if e.Kind != extensions.KindPlugin {
			t.Errorf("%s: kind = %q, want plugin", e.Key(), e.Kind)
		}

		profile := strings.TrimPrefix(e.ID, "ecc-")
		if !eccKnownProfiles[profile] {
			t.Errorf("%s: ID suffix %q is not a known ECC profile", e.Key(), profile)
		}

		mustContain := []string{
			"scripts/install-apply.js",
			"--target " + target,
			"--profile " + profile,
		}
		for _, want := range mustContain {
			if !strings.Contains(e.Install, want) {
				t.Errorf("%s: install snippet missing %q", e.Key(), want)
			}
		}

		var hasNode bool
		for _, f := range e.Deps.Features {
			if f == "node" {
				hasNode = true
			}
		}
		if !hasNode {
			t.Errorf("%s: missing node feature dep (installer runs on node)", e.Key())
		}
	}
}

// TestECC_PinnedRefIsUniform guards the most error-prone part of bumping ECC:
// every entry must pin the SAME commit, so a bump is a single uniform edit.
func TestECC_PinnedRefIsUniform(t *testing.T) {
	entries := eccEntries(t)
	refs := map[string][]string{} // sha -> entry keys
	for _, e := range entries {
		if eccTargetForHarness[e.Harness] == "" {
			continue
		}
		m := eccRefLine.FindStringSubmatch(e.Install)
		if m == nil {
			t.Errorf("%s: install snippet has no `ECC_REF=<40-hex-sha>` line", e.Key())
			continue
		}
		refs[m[1]] = append(refs[m[1]], e.Key())
	}
	if len(refs) > 1 {
		t.Errorf("ECC entries pin %d different commits (must be uniform): %v", len(refs), refs)
	}
}
