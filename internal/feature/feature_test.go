package feature

import (
	"reflect"
	"strings"
	"testing"
)

func TestByID(t *testing.T) {
	f, ok := ByID("python")
	if !ok {
		t.Fatalf("expected python to be a known feature")
	}
	if f.Name == "" {
		t.Errorf("python feature has empty Name")
	}

	if _, ok := ByID("not-a-feature"); ok {
		t.Errorf("expected ByID of unknown id to return false")
	}
}

func TestIsKnown(t *testing.T) {
	if !IsKnown("python") {
		t.Errorf("python should be known")
	}
	if IsKnown("nope") {
		t.Errorf("nope should be unknown")
	}
}

func TestIDs_MatchesRegistry(t *testing.T) {
	ids := IDs()
	if len(ids) != len(Registry) {
		t.Fatalf("IDs len = %d, Registry len = %d", len(ids), len(Registry))
	}
	for i, id := range ids {
		if id != Registry[i].ID {
			t.Errorf("IDs[%d]=%q, Registry[%d].ID=%q", i, id, i, Registry[i].ID)
		}
	}
}

func TestRegistry_HasNoDuplicates(t *testing.T) {
	seen := make(map[string]int)
	for _, f := range Registry {
		seen[f.ID]++
		if seen[f.ID] > 1 {
			t.Errorf("duplicate feature ID %q in Registry", f.ID)
		}
	}
}

func TestRegistry_AllHaveDockerfile(t *testing.T) {
	// Every feature must carry a non-empty Dockerfile fragment, otherwise
	// the generator emits an empty stage when that feature is enabled.
	for _, f := range Registry {
		if strings.TrimSpace(f.Dockerfile) == "" {
			t.Errorf("feature %q has empty Dockerfile fragment", f.ID)
		}
	}
}

func TestResolve_EnabledIsRegistryOrder(t *testing.T) {
	// Resolve must emit Enabled in Registry order so deps always precede
	// dependents in the generated Dockerfile (playwright → node means node
	// must be in Enabled before playwright).
	got, err := Resolve(nil, []string{"playwright"}, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// playwright depends on node. Find each index.
	nodeIdx, pwIdx := -1, -1
	for i, id := range got.Enabled {
		if id == "node" {
			nodeIdx = i
		}
		if id == "playwright" {
			pwIdx = i
		}
	}
	if nodeIdx < 0 || pwIdx < 0 {
		t.Fatalf("missing node or playwright in Enabled: %v", got.Enabled)
	}
	if nodeIdx > pwIdx {
		t.Errorf("node (idx %d) should precede playwright (idx %d) in Enabled: %v",
			nodeIdx, pwIdx, got.Enabled)
	}
}

func TestRegistry_DepsAreKnown(t *testing.T) {
	// Every Deps entry must point at a real feature, otherwise Resolve
	// produces silent bugs (the dep would always be missing).
	for _, f := range Registry {
		for _, dep := range f.Deps {
			if !IsKnown(dep) {
				t.Errorf("feature %q declares unknown dep %q", f.ID, dep)
			}
		}
	}
}

func TestResolve_HappyPath(t *testing.T) {
	got, err := Resolve(
		[]string{"python", "go"},
		[]string{"gh"},
		nil,
	)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// Enabled is sorted by Registry order so deps always precede dependents
	// in the generated Dockerfile.
	want := []string{"python", "go", "gh"}
	if !reflect.DeepEqual(got.Enabled, want) {
		t.Errorf("Enabled = %v, want %v", got.Enabled, want)
	}
	if len(got.AutoEnabled) != 0 {
		t.Errorf("AutoEnabled = %v, want empty (no deps required)", got.AutoEnabled)
	}
}

func TestResolve_NoFlagRemovesInitial(t *testing.T) {
	got, err := Resolve(
		[]string{"python", "go", "node"},
		nil,
		[]string{"go"},
	)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	for _, id := range got.Enabled {
		if id == "go" {
			t.Errorf("go should have been removed but is still in %v", got.Enabled)
		}
	}
}

func TestResolve_AutoEnablesDeps(t *testing.T) {
	// audit-toolkit declares python as a dep. Starting without python and
	// adding audit-toolkit should auto-enable python with the warning signal.
	got, err := Resolve(nil, []string{"audit-toolkit"}, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !contains(got.Enabled, "python") {
		t.Errorf("python should have been auto-enabled, got Enabled=%v", got.Enabled)
	}
	if !contains(got.AutoEnabled, "python") {
		t.Errorf("python should be in AutoEnabled, got %v", got.AutoEnabled)
	}
	// audit-toolkit itself was explicitly requested, so it should NOT be
	// in AutoEnabled.
	if contains(got.AutoEnabled, "audit-toolkit") {
		t.Errorf("audit-toolkit should not be in AutoEnabled, got %v", got.AutoEnabled)
	}
}

func TestResolve_TransitiveDeps(t *testing.T) {
	// playwright → node. Start with neither, add playwright.
	got, err := Resolve(nil, []string{"playwright"}, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	for _, id := range []string{"node", "playwright"} {
		if !contains(got.Enabled, id) {
			t.Errorf("%s should be enabled (transitively via playwright→node)", id)
		}
	}
}

func TestResolve_NoConflictsWithWith(t *testing.T) {
	// `--no=python --with=serena` doesn't apply here because serena isn't a
	// feature (it's an extension). Use audit-toolkit which has the same
	// dep: with audit-toolkit but no python. Expected behavior (matching
	// bash): python gets re-enabled because audit-toolkit needs it.
	got, err := Resolve(nil, []string{"audit-toolkit"}, []string{"python"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !contains(got.Enabled, "python") {
		t.Errorf("python should be re-enabled via audit-toolkit dep, got %v", got.Enabled)
	}
	if !contains(got.AutoEnabled, "python") {
		t.Errorf("python should be flagged AutoEnabled, got %v", got.AutoEnabled)
	}
}

func TestResolve_UnknownIDInWith(t *testing.T) {
	_, err := Resolve(nil, []string{"not-a-feature"}, nil)
	if err == nil {
		t.Errorf("expected error for unknown feature in with, got nil")
	}
	if !strings.Contains(err.Error(), "not-a-feature") {
		t.Errorf("error should mention the bad id, got: %v", err)
	}
}

func TestResolve_UnknownIDInInitial(t *testing.T) {
	_, err := Resolve([]string{"ghost-feature"}, nil, nil)
	if err == nil {
		t.Errorf("expected error for unknown id in initial, got nil")
	}
}

func TestResolve_UnknownIDInNo(t *testing.T) {
	_, err := Resolve(nil, nil, []string{"phantom"})
	if err == nil {
		t.Errorf("expected error for unknown id in no, got nil")
	}
}

func TestResolve_OrderIndependence(t *testing.T) {
	// Two equivalent inputs in different orders should produce identical results.
	a, _ := Resolve([]string{"python", "go", "node"}, []string{"gh"}, nil)
	b, _ := Resolve([]string{"node", "python", "go"}, []string{"gh"}, nil)
	if !reflect.DeepEqual(a.Enabled, b.Enabled) {
		t.Errorf("order matters when it shouldn't: %v vs %v", a.Enabled, b.Enabled)
	}
}

func TestResolve_Idempotent_WithAndInitialOverlap(t *testing.T) {
	// User says --with=python when python is already in initial. Shouldn't
	// duplicate or break anything.
	got, err := Resolve([]string{"python"}, []string{"python"}, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !reflect.DeepEqual(got.Enabled, []string{"python"}) {
		t.Errorf("expected [python], got %v", got.Enabled)
	}
}

func TestResolve_EmptyInputs(t *testing.T) {
	got, err := Resolve(nil, nil, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Enabled) != 0 {
		t.Errorf("expected empty Enabled, got %v", got.Enabled)
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
