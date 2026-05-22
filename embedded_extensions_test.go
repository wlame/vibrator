package vibrator_test

// This file exercises the embedded extensions catalogue end-to-end: it
// reads every Markdown file under extensions/, parses it via the
// loader, and asserts catalogue-wide invariants. Failures here mean
// the binary ships with a broken catalogue.

import (
	"strings"
	"testing"

	vibrator "github.com/wlame/vibrator"
	"github.com/wlame/vibrator/internal/extensions"
	"github.com/wlame/vibrator/internal/feature"
	"github.com/wlame/vibrator/internal/harness"
	_ "github.com/wlame/vibrator/internal/harness/all"
)

// TestEmbeddedExtensions_CountByHarness pins the minimum entry count
// per harness. The real number should only grow; a sharp drop here
// means someone accidentally deleted a folder of entries.
//
// Update the floor when the catalogue legitimately shrinks.
func TestEmbeddedExtensions_CountByHarness(t *testing.T) {
	all, err := extensions.LoadAll(vibrator.ExtensionsFS)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	counts := map[string]int{}
	for _, e := range all {
		counts[e.Harness]++
	}
	floors := map[string]int{
		"claude-code": 25,
		"codex":       20,
		"opencode":    15,
		"pi":          25,
	}
	for h, floor := range floors {
		if counts[h] < floor {
			t.Errorf("harness %q has %d entries, want >= %d", h, counts[h], floor)
		}
	}
}

// TestEmbeddedExtensions_ValidCategories asserts every entry's
// declared category is one of the known constants (or empty). Catches
// typos like "ai-integraton" before they reach the wizard.
func TestEmbeddedExtensions_ValidCategories(t *testing.T) {
	all, err := extensions.LoadAll(vibrator.ExtensionsFS)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	known := map[extensions.Category]bool{}
	for _, c := range extensions.AllCategories {
		known[c] = true
	}
	for _, e := range all {
		if e.Category == "" {
			continue // optional field
		}
		if !known[e.Category] {
			t.Errorf("%s: unknown category %q (typo? See extensions.AllCategories)", e.Key(), e.Category)
		}
	}
}

// TestEmbeddedExtensions_ValidHarness asserts every entry's harness
// matches a registered harness ID. Drift between folder names and
// the harness registry would silently break the wizard's per-harness
// filtering.
func TestEmbeddedExtensions_ValidHarness(t *testing.T) {
	all, err := extensions.LoadAll(vibrator.ExtensionsFS)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	for _, e := range all {
		if _, ok := harness.ByID(e.Harness); !ok {
			t.Errorf("%s: harness %q not in registry", e.Key(), e.Harness)
		}
	}
}

// TestEmbeddedExtensions_InstallNoReservedDelimiter guards against
// the heredoc-nesting bug: any install snippet containing a standalone
// line matching the dockerfile generator's outer delimiter would
// terminate the wrapping RUN heredoc early.
//
// The generator itself rejects such snippets (writeExtensionInstall
// returns an error), but we want the failure to surface at test time,
// not at docker build time.
func TestEmbeddedExtensions_InstallNoReservedDelimiter(t *testing.T) {
	const reserved = "VIBRATE_EXT_INSTALL"
	all, err := extensions.LoadAll(vibrator.ExtensionsFS)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	for _, e := range all {
		for _, line := range strings.Split(e.Install, "\n") {
			if strings.TrimSpace(line) == reserved {
				t.Errorf("%s: install contains reserved heredoc delimiter %q on its own line", e.Key(), reserved)
			}
		}
	}
}

// TestEmbeddedExtensions_ConsistencyChecks pins lightweight invariants
// across the catalogue. Bundling them so one failure doesn't mask
// others.
func TestEmbeddedExtensions_ConsistencyChecks(t *testing.T) {
	all, err := extensions.LoadAll(vibrator.ExtensionsFS)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	for _, e := range all {
		// auth.env must be UPPER_SNAKE_CASE if set — env vars
		// downstream tools read won't accept anything else.
		if e.Auth != nil && e.Auth.Env != "" {
			if strings.ToUpper(e.Auth.Env) != e.Auth.Env {
				t.Errorf("%s: auth.env %q is not upper-case", e.Key(), e.Auth.Env)
			}
		}
		// source URLs should look like URLs.
		if !strings.HasPrefix(e.Source, "http://") && !strings.HasPrefix(e.Source, "https://") {
			t.Errorf("%s: source %q doesn't look like a URL", e.Key(), e.Source)
		}
		// install snippets that mention `set -e` are fine but
		// redundant — the generator already injects `set -e` at the
		// top. (Informational only; not a hard failure.)
		// Skipped — keep noise low.

		// host_aliases (if set) must be lowercase to match hostprobe.
		for _, a := range e.HostAliases {
			if strings.ToLower(a) != a {
				t.Errorf("%s: host_alias %q is not lowercase", e.Key(), a)
			}
		}
	}
}

// TestEmbeddedExtensions_FeatureDepsResolvable cross-checks every
// dep against the feature registry. Already covered by
// embedded_test.go's TestEmbeddedExtensionsFeatureDepsAreValid; this
// variant reports ALL offenders in one pass, easier to triage.
func TestEmbeddedExtensions_FeatureDepsResolvable(t *testing.T) {
	all, err := extensions.LoadAll(vibrator.ExtensionsFS)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	bad := 0
	for _, e := range all {
		for _, f := range e.Deps.Features {
			if !feature.IsKnown(f) {
				t.Errorf("%s: unknown feature dep %q", e.Key(), f)
				bad++
			}
		}
	}
	if bad > 0 {
		t.Logf("%d offending feature deps total", bad)
	}
}
