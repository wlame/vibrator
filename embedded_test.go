package vibrator

import (
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/catalog"
	"github.com/wlame/vibrator/internal/feature"
)

// TestEmbeddedCatalogLoads is the gateway test for catalog content. If any
// markdown file under catalog/ has bad frontmatter or violates the schema,
// this test fails — surfaced before binary release rather than at user run.
func TestEmbeddedCatalogLoads(t *testing.T) {
	entries, err := catalog.LoadAll(CatalogFS)
	if err != nil {
		t.Fatalf("catalog.LoadAll: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("embedded catalog is empty — did `//go:embed catalog` lose the directory?")
	}
	t.Logf("loaded %d catalog entries across %d harnesses", len(entries), countHarnesses(entries))
}

// TestEmbeddedCatalogFeatureDepsAreValid cross-checks that every catalog
// entry's Deps.Features list references a feature defined in
// internal/feature.Registry. If a contributor adds a catalog entry that
// declares `deps: features: [foo]` but `foo` isn't a real feature, this
// test catches it before the dockerfile generator (Phase 3) fails opaquely.
func TestEmbeddedCatalogFeatureDepsAreValid(t *testing.T) {
	entries, err := catalog.LoadAll(CatalogFS)
	if err != nil {
		t.Fatalf("catalog.LoadAll: %v", err)
	}
	if err := catalog.ValidateAgainstFeatures(entries, feature.IsKnown); err != nil {
		t.Errorf("invalid feature dep in catalog: %v", err)
	}
}

// TestEmbeddedCatalogHasExpectedHarnesses verifies all four launch harnesses
// are represented. Catches accidental directory typos or empty harness dirs
// (which embed silently skips).
func TestEmbeddedCatalogHasExpectedHarnesses(t *testing.T) {
	got, err := catalog.Harnesses(CatalogFS)
	if err != nil {
		t.Fatalf("catalog.Harnesses: %v", err)
	}
	want := []string{"claude-code", "codex", "opencode", "pi"}
	gotSet := make(map[string]bool)
	for _, h := range got {
		gotSet[h] = true
	}
	for _, h := range want {
		if !gotSet[h] {
			t.Errorf("missing harness %q in embedded catalog (got: %v)", h, got)
		}
	}
}

// TestEmbeddedCatalogEntriesHaveSources verifies the `source` field is set
// on every entry. This is a soft-required field — the loader enforces it,
// but a regression in that enforcement should still surface here.
func TestEmbeddedCatalogEntriesHaveSources(t *testing.T) {
	entries, err := catalog.LoadAll(CatalogFS)
	if err != nil {
		t.Fatalf("catalog.LoadAll: %v", err)
	}
	for key, e := range entries {
		if strings.TrimSpace(e.Source) == "" {
			t.Errorf("catalog entry %s has empty Source — traceability rule violated", key)
		}
	}
}

func countHarnesses(entries map[string]*catalog.Entry) int {
	set := make(map[string]bool)
	for _, e := range entries {
		set[e.Harness] = true
	}
	return len(set)
}
