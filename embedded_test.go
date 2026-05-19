package vibrator

import (
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/extensions"
	"github.com/wlame/vibrator/internal/feature"
)

// TestEmbeddedExtensionsLoads is the gateway test for extensions content. If any
// markdown file under extensions/ has bad frontmatter or violates the schema,
// this test fails — surfaced before binary release rather than at user run.
func TestEmbeddedExtensionsLoads(t *testing.T) {
	entries, err := extensions.LoadAll(ExtensionsFS)
	if err != nil {
		t.Fatalf("extensions.LoadAll: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("embedded extensions is empty — did `//go:embed extensions` lose the directory?")
	}
	t.Logf("loaded %d extension entries across %d harnesses", len(entries), countHarnesses(entries))
}

// TestEmbeddedExtensionsFeatureDepsAreValid cross-checks that every extensions
// entry's Deps.Features list references a feature defined in
// internal/feature.Registry. If a contributor adds an extension that
// declares `deps: features: [foo]` but `foo` isn't a real feature, this
// test catches it before the dockerfile generator (Phase 3) fails opaquely.
func TestEmbeddedExtensionsFeatureDepsAreValid(t *testing.T) {
	entries, err := extensions.LoadAll(ExtensionsFS)
	if err != nil {
		t.Fatalf("extensions.LoadAll: %v", err)
	}
	if err := extensions.ValidateAgainstFeatures(entries, feature.IsKnown); err != nil {
		t.Errorf("invalid feature dep in extensions: %v", err)
	}
}

// TestEmbeddedExtensionsHasExpectedHarnesses verifies all four launch harnesses
// are represented. Catches accidental directory typos or empty harness dirs
// (which embed silently skips).
func TestEmbeddedExtensionsHasExpectedHarnesses(t *testing.T) {
	got, err := extensions.Harnesses(ExtensionsFS)
	if err != nil {
		t.Fatalf("extensions.Harnesses: %v", err)
	}
	want := []string{"claude-code", "codex", "opencode", "pi"}
	gotSet := make(map[string]bool)
	for _, h := range got {
		gotSet[h] = true
	}
	for _, h := range want {
		if !gotSet[h] {
			t.Errorf("missing harness %q in embedded extensions (got: %v)", h, got)
		}
	}
}

// TestEmbeddedExtensionsHaveSources verifies the `source` field is set
// on every entry. This is a soft-required field — the loader enforces it,
// but a regression in that enforcement should still surface here.
func TestEmbeddedExtensionsHaveSources(t *testing.T) {
	entries, err := extensions.LoadAll(ExtensionsFS)
	if err != nil {
		t.Fatalf("extensions.LoadAll: %v", err)
	}
	for key, e := range entries {
		if strings.TrimSpace(e.Source) == "" {
			t.Errorf("extensions entry %s has empty Source — traceability rule violated", key)
		}
	}
}

func countHarnesses(entries map[string]*extensions.Entry) int {
	set := make(map[string]bool)
	for _, e := range entries {
		set[e.Harness] = true
	}
	return len(set)
}
