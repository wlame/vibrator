package harness_test

import (
	"slices"
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
		"pi":          {"npm", "install", "-g", "@earendil-works/pi-coding-agent@latest"},
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

// TestSessionPersistenceMounts verifies codex and opencode each declare a
// MountDirEnsure entry for their session/rollout history directory, so a
// fresh container still gets a writable dir and history survives container
// recreation — parity with claude-code's session-persist dirs. Confirmed
// against the real paths: codex writes rollout jsonl files under
// ~/.codex/sessions/<year>/<month>/<day>/ (verified by running `codex exec`
// locally and inspecting the resulting tree); opencode stores session/
// message/part data under ~/.local/share/opencode/storage (documented at
// https://deepwiki.com/sst/opencode/2.9-storage-and-database).
func TestSessionPersistenceMounts(t *testing.T) {
	cases := []struct {
		id       string
		wantHost string
	}{
		{"codex", ".codex/sessions"},
		{"opencode", ".local/share/opencode/storage"},
	}
	for _, tc := range cases {
		h, _ := harness.ByID(tc.id)
		found := false
		for _, m := range h.HostMounts(harness.HostMountContext{WorkspaceDir: "/w"}) {
			if m.HostRel == tc.wantHost && m.Kind == harness.MountDirEnsure {
				found = true
			}
		}
		if !found {
			t.Errorf("%s HostMounts missing MountDirEnsure %s", tc.id, tc.wantHost)
		}
	}
}

// TestCodexConfigMountsToSidecar verifies that Codex's config.toml mounts to
// the .host sidecar (config.host.toml) rather than shadowing the baked
// config.toml (which contains vibrator's MCP extensions). The materializer
// entrypoint seeds the sidecar copy into the writable config.toml and replays
// the baked MCPs on top — same pattern as claude-code's .claude.host.json.
func TestCodexConfigMountsToSidecar(t *testing.T) {
	h, _ := harness.ByID("codex")
	mounts := h.HostMounts(harness.HostMountContext{WorkspaceDir: "/w"})
	var sidecar, shadowing bool
	for _, m := range mounts {
		if m.HostRel == ".codex/config.toml" && m.ContainerRel == ".codex/config.host.toml" {
			sidecar = true
		}
		if m.ContainerRel == ".codex/config.toml" {
			shadowing = true
		}
	}
	if !sidecar {
		t.Error("codex config.toml should mount to the .host sidecar")
	}
	if shadowing {
		t.Error("codex must NOT mount over container .codex/config.toml (shadows baked MCPs)")
	}
}

// TestOpencodeConfigMountsToHostSidecar verifies that OpenCode's config dir
// mounts to the .host sidecar (.config/opencode.host) rather than shadowing
// the baked directory (which contains vibrator's extension artifacts: MCPs
// in config.json, agent/, themes/, tui.json). The materializer entrypoint
// seeds the real config dir from the baked snapshot and merges the sidecar
// over it — same pattern as codex's config.host.toml.
func TestOpencodeConfigMountsToHostSidecar(t *testing.T) {
	h, _ := harness.ByID("opencode")
	mounts := h.HostMounts(harness.HostMountContext{WorkspaceDir: "/w"})
	var sidecar, shadowing bool
	for _, m := range mounts {
		if m.HostRel == ".config/opencode" && m.ContainerRel == ".config/opencode.host" {
			sidecar = true
			if !m.ReadOnly {
				t.Error("the .config/opencode.host sidecar must be read-only")
			}
			if m.Kind != harness.MountDirIfExists {
				t.Errorf("sidecar mount kind = %v, want MountDirIfExists", m.Kind)
			}
		}
		if m.ContainerRel == ".config/opencode" {
			shadowing = true
		}
	}
	if !sidecar {
		t.Error("opencode config dir should mount to the .config/opencode.host sidecar")
	}
	if shadowing {
		t.Error("opencode must NOT mount over container .config/opencode (shadows baked extension artifacts)")
	}
}

// TestPiMountsSidecarWithCarveOuts verifies pi's restructured mounts: the
// host ~/.pi tree lands read-only at the .pi.host sidecar (so it can no
// longer shadow baked extension artifacts or be corrupted by the
// container), while agent/auth.json (login writeback) and agent/sessions
// (history persistence) keep today's read-write behavior via granular
// mounts. No mount may target the container's real .pi root — the
// materializer owns it.
func TestPiMountsSidecarWithCarveOuts(t *testing.T) {
	h, _ := harness.ByID("pi")
	mounts := h.HostMounts(harness.HostMountContext{WorkspaceDir: "/w"})
	var sidecar, auth, sessions, shadowing bool
	for _, m := range mounts {
		switch {
		case m.HostRel == ".pi" && m.ContainerRel == ".pi.host":
			sidecar = true
			if !m.ReadOnly {
				t.Error("the .pi.host sidecar must be read-only")
			}
			if m.Kind != harness.MountDirIfExists {
				t.Errorf("sidecar mount kind = %v, want MountDirIfExists", m.Kind)
			}
		case m.HostRel == ".pi/agent/auth.json" && m.ContainerRel == ".pi/agent/auth.json":
			auth = true
			if m.ReadOnly {
				t.Error("agent/auth.json must stay read-write (login writeback)")
			}
			if m.Kind != harness.MountFileIfExists {
				t.Errorf("auth mount kind = %v, want MountFileIfExists", m.Kind)
			}
		case m.HostRel == ".pi/agent/sessions" && m.ContainerRel == ".pi/agent/sessions":
			sessions = true
			if m.Kind != harness.MountDirEnsure {
				t.Errorf("sessions mount kind = %v, want MountDirEnsure", m.Kind)
			}
		}
		if m.ContainerRel == ".pi" {
			shadowing = true
		}
	}
	if !sidecar {
		t.Error("missing the .pi.host sidecar mount")
	}
	if !auth {
		t.Error("missing the agent/auth.json rw carve-out mount")
	}
	if !sessions {
		t.Error("missing the agent/sessions persistence mount")
	}
	if shadowing {
		t.Error("pi must NOT mount over container .pi (shadows baked extension artifacts)")
	}
}

func TestExtraDirArgs(t *testing.T) {
	dirs := []string{"/data/refs", "/work/lib"}

	cc, ok := harness.ByID("claude-code")
	if !ok {
		t.Fatal("claude-code harness not registered")
	}
	got := cc.ExtraDirArgs(dirs)
	want := []string{"--add-dir", "/data/refs", "--add-dir", "/work/lib"}
	if len(got) != len(want) {
		t.Fatalf("claude-code ExtraDirArgs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("claude-code ExtraDirArgs = %v, want %v", got, want)
		}
	}
	if cc.ExtraDirArgs(nil) != nil {
		t.Fatal("claude-code ExtraDirArgs(nil) must be nil")
	}

	for _, id := range []string{"codex", "opencode", "pi"} {
		h, ok := harness.ByID(id)
		if !ok {
			t.Fatalf("harness %q not registered", id)
		}
		if h.ExtraDirArgs(dirs) != nil {
			t.Fatalf("%s ExtraDirArgs must be nil", id)
		}
	}
}

// PermissionBypassArgs is the single source of truth for each harness's
// "skip approvals / YOLO" flag. Pin the known values so an upstream flag
// rename (or a harness losing/gaining a bypass flag) is a deliberate edit
// here, not a silent behavior change picked up at launch time.
func TestPermissionBypassArgs_KnownValues(t *testing.T) {
	cc, ok := harness.ByID("claude-code")
	if !ok {
		t.Fatal("claude-code harness not registered")
	}
	if got := cc.PermissionBypassArgs(); len(got) != 1 || got[0] != "--dangerously-skip-permissions" {
		t.Errorf("claude-code PermissionBypassArgs = %v, want [--dangerously-skip-permissions]", got)
	}

	// codex: verified against `codex --help` (codex-cli 0.142.5, 2026-07) —
	// --dangerously-bypass-approvals-and-sandbox is documented as "Skip all
	// confirmation prompts and execute commands without sandboxing."
	cx, ok := harness.ByID("codex")
	if !ok {
		t.Fatal("codex harness not registered")
	}
	if got := cx.PermissionBypassArgs(); len(got) != 1 || got[0] != "--dangerously-bypass-approvals-and-sandbox" {
		t.Errorf("codex PermissionBypassArgs = %v, want [--dangerously-bypass-approvals-and-sandbox]", got)
	}

	// opencode/pi: no confirmed single-flag bypass as of 2026-07 (opencode's
	// closest analogue, --auto, still honors explicit deny rules and isn't a
	// full bypass; pi's core CLI has no permission system to bypass at all).
	// nil is the honest default until upstream ships one.
	for _, id := range []string{"opencode", "pi"} {
		h, ok := harness.ByID(id)
		if !ok {
			t.Fatalf("harness %q not registered", id)
		}
		if got := h.PermissionBypassArgs(); got != nil {
			t.Errorf("%s PermissionBypassArgs = %v, want nil (no confirmed bypass flag)", id, got)
		}
	}
}

// Every registered harness must implement PermissionBypassArgs without
// panicking (nil is a valid return).
func TestRegistry_AllHavePermissionBypassArgs(t *testing.T) {
	for _, h := range harness.Registry {
		_ = h.PermissionBypassArgs() // must not panic
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

// TestLoginFlow_ClaudeCodeKnownValues pins claude-code's LoginFlow to the
// exact values previously hardcoded in internal/app/launch.go
// (claudeAuthURLMarker + authFields), copied verbatim so Task 2's move
// into the harness package can't silently drift the strings.
func TestLoginFlow_ClaudeCodeKnownValues(t *testing.T) {
	cc, _ := harness.ByID("claude-code")
	f := cc.LoginFlow()
	if f == nil {
		t.Fatal("claude-code LoginFlow is nil, want populated")
	}
	if len(f.Command) != 3 || f.Command[0] != "claude" || f.Command[1] != "auth" || f.Command[2] != "login" {
		t.Errorf("Command = %v, want [claude auth login]", f.Command)
	}
	if f.URLMarker != "If the browser didn't open, visit: " {
		t.Errorf("URLMarker = %q", f.URLMarker)
	}
	if f.Writeback == nil {
		t.Fatal("claude-code Writeback is nil, want populated")
	}
	if f.Writeback.ContainerRel != ".claude.json" || f.Writeback.HostRel != ".claude.json" {
		t.Errorf("Writeback paths = %q/%q", f.Writeback.ContainerRel, f.Writeback.HostRel)
	}
	wantFields := []string{
		"oauthAccount", "userID", "hasCompletedOnboarding", "lastOnboardingVersion",
		"subscriptionNoticeCount", "hasAvailableSubscription", "s1mAccessCache",
	}
	if !slices.Equal(f.Writeback.Fields, wantFields) {
		t.Errorf("Writeback.Fields = %v, want %v", f.Writeback.Fields, wantFields)
	}
}

// TestLoginFlow_NonClaudeAreNil documents that --login is only wired for
// claude-code today; the other three harnesses return nil until their
// in-container auth flow is verified (see each harness's LoginFlow doc
// comment for the specific blocker).
func TestLoginFlow_NonClaudeAreNil(t *testing.T) {
	for _, id := range []string{"codex", "opencode", "pi"} {
		h, _ := harness.ByID(id)
		if f := h.LoginFlow(); f != nil {
			t.Errorf("%s LoginFlow = %+v, want nil", id, f)
		}
	}
}

// Every registered harness implements LoginFlow nil-safely.
func TestRegistry_AllHaveLoginFlow(t *testing.T) {
	for _, h := range harness.Registry {
		_ = h.LoginFlow() // must not panic; nil is valid
	}
}
