package harness_test

import (
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/feature"
	"github.com/wlame/vibrator/internal/harness"
	_ "github.com/wlame/vibrator/internal/harness/all" // register all built-in harnesses
)

// Tests live in package harness_test (external test package) so they import
// the same way real callers do — via the all/ side-effect package — which
// catches "did the import accidentally drop a harness" regressions.

func TestRegistry_HasFourBuiltins(t *testing.T) {
	ids := harness.IDs()
	want := []string{"claude-code", "codex", "opencode", "pi"}
	got := make(map[string]bool)
	for _, id := range ids {
		got[id] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing harness %q (registered: %v)", w, ids)
		}
	}
}

func TestRegistry_NoDuplicates(t *testing.T) {
	seen := make(map[string]int)
	for _, h := range harness.Registry {
		seen[h.ID()]++
		if seen[h.ID()] > 1 {
			t.Errorf("duplicate harness ID %q in registry", h.ID())
		}
	}
}

func TestByID(t *testing.T) {
	h, ok := harness.ByID("claude-code")
	if !ok {
		t.Fatalf("claude-code should be registered")
	}
	if h.Name() == "" {
		t.Errorf("claude-code has empty Name")
	}

	if _, ok := harness.ByID("does-not-exist"); ok {
		t.Errorf("expected ByID of unknown id to return false")
	}
}

// Every harness must declare a non-empty Dockerfile fragment, otherwise
// the generator emits an empty install step.
func TestRegistry_AllHaveDockerfile(t *testing.T) {
	for _, h := range harness.Registry {
		if strings.TrimSpace(h.Dockerfile()) == "" {
			t.Errorf("harness %q has empty Dockerfile fragment", h.ID())
		}
	}
}

// RequiredFeatures must reference real feature IDs from internal/feature.
// A typo here would silently produce a Dockerfile that references an
// undefined feature stage.
func TestRegistry_RequiredFeaturesValid(t *testing.T) {
	for _, h := range harness.Registry {
		for _, f := range h.RequiredFeatures() {
			if !feature.IsKnown(f) {
				t.Errorf("harness %q declares unknown required feature %q", h.ID(), f)
			}
		}
	}
}

// Every harness must declare a host config dir — Phase 4 lifecycle code
// relies on this to set up selective mounts for settings/auth/plugins.
func TestRegistry_AllHaveHostConfigDir(t *testing.T) {
	for _, h := range harness.Registry {
		if h.HostConfigDir() == "" {
			t.Errorf("harness %q has empty HostConfigDir", h.ID())
		}
	}
}

// Harness IDs must match catalog/<id>/ directory names. Verified against
// the embedded catalog in the root vibrator package — checked there to
// avoid an import cycle.
func TestRegistry_IDsAreKebabCase(t *testing.T) {
	for _, h := range harness.Registry {
		id := h.ID()
		for _, r := range id {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				t.Errorf("harness ID %q is not kebab-case (bad char %q)", id, r)
			}
		}
	}
}
