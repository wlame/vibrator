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

// Every harness's HostMounts descriptors must be well-formed: a non-empty
// host- and container-relative path, a valid MountKind, and — critically —
// neither path may climb out of its home root via "..". The orchestrator
// (internal/app) also guards against escape at mount time, but catching a
// bad descriptor here keeps a harness-authoring mistake from silently
// being skipped at launch.
func TestRegistry_HostMountsAreWellFormed(t *testing.T) {
	ctx := harness.HostMountContext{WorkspaceDir: "/home/alice/project"}
	for _, h := range harness.Registry {
		for i, m := range h.HostMounts(ctx) {
			if m.HostRel == "" || m.ContainerRel == "" {
				t.Errorf("harness %q HostMounts[%d] has an empty path: %+v", h.ID(), i, m)
			}
			if m.Kind < harness.MountFileIfExists || m.Kind > harness.MountDirEnsure {
				t.Errorf("harness %q HostMounts[%d] has invalid Kind %d", h.ID(), i, m.Kind)
			}
			for _, rel := range []string{m.HostRel, m.ContainerRel} {
				if rel == ".." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
					t.Errorf("harness %q HostMounts[%d] path %q must be relative and stay within home", h.ID(), i, rel)
				}
			}
		}
	}
}

// Harness IDs must match extensions/<id>/ directory names. Verified against
// the embedded extensions in the root vibrator package — checked there to
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

// Every harness must declare a non-empty LaunchCommand — without it,
// bare `vibrate` can't exec anything inside the container.
func TestRegistry_AllHaveLaunchCommand(t *testing.T) {
	for _, h := range harness.Registry {
		argv := h.LaunchCommand()
		if len(argv) == 0 {
			t.Errorf("harness %q has empty LaunchCommand", h.ID())
			continue
		}
		// First element should be the canonical command name — match
		// the harness ID convention. Not a strict rule (codex's CLI
		// could theoretically be "codex-cli" while harness ID stays
		// "codex"), but verify it's plausibly a binary name.
		if argv[0] == "" {
			t.Errorf("harness %q LaunchCommand[0] is empty", h.ID())
		}
		if strings.Contains(argv[0], "/") || strings.Contains(argv[0], " ") {
			t.Errorf("harness %q LaunchCommand[0] %q looks suspicious (path / space)", h.ID(), argv[0])
		}
	}
}

// Every harness must declare an UpdateCommand — `vibrate update`
// requires a non-empty argv to act on. An empty value would surface
// as "harness X doesn't support in-place updates" at runtime, which
// is a polite error but a CI test catches it before release.
func TestRegistry_AllHaveUpdateCommand(t *testing.T) {
	for _, h := range harness.Registry {
		argv := h.UpdateCommand()
		if len(argv) == 0 {
			t.Errorf("harness %q has empty UpdateCommand", h.ID())
			continue
		}
		if argv[0] == "" {
			t.Errorf("harness %q UpdateCommand[0] is empty", h.ID())
		}
		if strings.Contains(argv[0], "/") {
			t.Errorf("harness %q UpdateCommand[0] %q has a slash (use bare binary name + PATH lookup)",
				h.ID(), argv[0])
		}
	}
}

// Specifically pin the update commands so a rename upstream (claude
// renames "update" to "upgrade", npm pkg rename, etc.) is a deliberate
// edit caught here rather than discovered at runtime by a confused user.
func TestUpdateCommand_KnownValues(t *testing.T) {
	cases := map[string][]string{
		"claude-code": {"claude", "update"},
		"codex":       {"npm", "install", "-g", "@openai/codex@latest"},
		"opencode":    {"opencode", "upgrade"},
		"pi":          {"npm", "install", "-g", "@mariozechner/pi-coding-agent@latest"},
	}
	for id, want := range cases {
		t.Run(id, func(t *testing.T) {
			h, ok := harness.ByID(id)
			if !ok {
				t.Fatalf("harness %q not registered", id)
			}
			got := h.UpdateCommand()
			if len(got) != len(want) {
				t.Fatalf("UpdateCommand len = %d, want %d (got %v, want %v)",
					len(got), len(want), got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("UpdateCommand[%d] = %q, want %q", i, got[i], want[i])
				}
			}
		})
	}
}

// Specifically pin the launch commands so a rename in upstream
// projects (e.g., claude → claude-cli) is a deliberate edit, not a
// silent change.
func TestLaunchCommand_KnownValues(t *testing.T) {
	cases := map[string]string{
		"claude-code": "claude",
		"codex":       "codex",
		"opencode":    "opencode",
		"pi":          "pi",
	}
	for id, want := range cases {
		t.Run(id, func(t *testing.T) {
			h, ok := harness.ByID(id)
			if !ok {
				t.Fatalf("harness %q not registered", id)
			}
			argv := h.LaunchCommand()
			if len(argv) == 0 || argv[0] != want {
				t.Errorf("LaunchCommand[0] = %v, want first element %q", argv, want)
			}
		})
	}
}
