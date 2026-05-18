package profile

import (
	"testing"
)

func TestAll_ContainsExpectedProfiles(t *testing.T) {
	want := []string{IDMinimal, IDBackend, IDFrontend, IDFull}
	if len(All) != len(want) {
		t.Fatalf("All len = %d, want %d", len(All), len(want))
	}
	for i, p := range All {
		if p.ID != want[i] {
			t.Errorf("All[%d].ID = %q, want %q", i, p.ID, want[i])
		}
	}
}

func TestByID(t *testing.T) {
	p, ok := ByID(IDBackend)
	if !ok {
		t.Fatalf("expected backend profile to be known")
	}
	if p.ID != IDBackend {
		t.Errorf("ByID(backend).ID = %q", p.ID)
	}

	if _, ok := ByID("not-a-profile"); ok {
		t.Errorf("ByID of unknown id should return false")
	}
}

func TestIDs(t *testing.T) {
	ids := IDs()
	if len(ids) != len(All) {
		t.Errorf("IDs len = %d, All len = %d", len(ids), len(All))
	}
}

func TestDefault_IsFull(t *testing.T) {
	if Default.ID != IDFull {
		t.Errorf("Default = %q, want %q (full profile)", Default.ID, IDFull)
	}
}

// All profile feature lists must reference real features. If we add a profile
// that references a feature we forgot to register, this test catches it at
// build time rather than runtime.
func TestProfile_AllFeaturesValid(t *testing.T) {
	for _, p := range All {
		if err := p.Validate(); err != nil {
			t.Errorf("profile %q: %v", p.ID, err)
		}
	}
}

func TestProfile_MinimalHasNoFeatures(t *testing.T) {
	// Minimal is intentionally empty — the always-on substrate is everything.
	if len(Minimal.Features) != 0 {
		t.Errorf("Minimal.Features = %v, want empty", Minimal.Features)
	}
}

func TestProfile_FullSupersetOfBackendAndFrontend(t *testing.T) {
	// Sanity check that "full" includes everything from "backend" and "frontend".
	// If we add a feature to either and forget to add it to full, this test
	// surfaces the gap.
	fullSet := make(map[string]bool)
	for _, f := range Full.Features {
		fullSet[f] = true
	}
	for _, src := range []Profile{Backend, Frontend} {
		for _, f := range src.Features {
			if !fullSet[f] {
				t.Errorf("Full is missing feature %q (present in %q)", f, src.ID)
			}
		}
	}
}

func TestResolve_BackendProfile(t *testing.T) {
	got, err := Backend.Resolve(nil, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// Just verify we got back a deterministic, populated result. The
	// concrete contents are an internal/feature test concern.
	if len(got.Enabled) == 0 {
		t.Errorf("backend.Resolve produced empty Enabled")
	}
	for _, want := range []string{"python", "go", "gh"} {
		found := false
		for _, id := range got.Enabled {
			if id == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("backend.Resolve missing expected feature %q (got %v)", want, got.Enabled)
		}
	}
}

func TestResolve_LayersWithAndNo(t *testing.T) {
	// Backend + --with=playwright - --no=go
	got, err := Backend.Resolve([]string{"playwright"}, []string{"go"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	enabled := make(map[string]bool)
	for _, id := range got.Enabled {
		enabled[id] = true
	}
	if !enabled["playwright"] {
		t.Errorf("expected playwright via --with, got %v", got.Enabled)
	}
	if enabled["go"] {
		t.Errorf("expected go to be removed via --no, got %v", got.Enabled)
	}
	// playwright→node, so node should be auto-enabled.
	if !enabled["node"] {
		t.Errorf("expected node to be auto-enabled (playwright dep), got %v", got.Enabled)
	}
}
