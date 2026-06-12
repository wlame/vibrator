package dockerfile_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/dockerfile"
	"github.com/wlame/vibrator/internal/extensions"
	"github.com/wlame/vibrator/internal/feature"
	"github.com/wlame/vibrator/internal/harness"
	_ "github.com/wlame/vibrator/internal/harness/all" // register built-in harnesses
)

// updateGolden lets `go test -update` rewrite the expected golden files
// after a deliberate change. Off by default; keep CI green.
var updateGolden = flag.Bool("update", false, "rewrite golden files instead of comparing")

// helper: load N features by ID in Registry order.
func feats(t *testing.T, ids ...string) []feature.Feature {
	t.Helper()
	out := make([]feature.Feature, 0, len(ids))
	for _, id := range ids {
		f, ok := feature.ByID(id)
		if !ok {
			t.Fatalf("feature %q not in registry", id)
		}
		out = append(out, f)
	}
	return out
}

// helper: resolve a harness by ID or fail the test.
func hrn(t *testing.T, id string) harness.Harness {
	t.Helper()
	h, ok := harness.ByID(id)
	if !ok {
		t.Fatalf("harness %q not registered", id)
	}
	return h
}

func TestGenerate_RejectsUnknownShell(t *testing.T) {
	_, err := dockerfile.Generate(dockerfile.Spec{
		Harness: hrn(t, "claude-code"),
		Shell:   "tcsh",
	})
	if err == nil {
		t.Errorf("expected error for unsupported shell, got nil")
	}
}

func TestGenerate_RejectsMissingHarness(t *testing.T) {
	_, err := dockerfile.Generate(dockerfile.Spec{Shell: "zsh"})
	if err == nil {
		t.Errorf("expected error for missing harness, got nil")
	}
}

func TestGenerate_Deterministic(t *testing.T) {
	spec := dockerfile.Spec{
		Harness:  hrn(t, "claude-code"),
		Profile:  "minimal",
		Shell:    "zsh",
		HostUID:  1000,
		HostGID:  1000,
		Username: "vibrate",
	}

	a, err := dockerfile.Generate(spec)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, err := dockerfile.Generate(spec)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if string(a) != string(b) {
		t.Errorf("two Generate calls with the same spec produced different output")
	}
}

func TestGenerate_ContainsExpectedSections(t *testing.T) {
	out, err := dockerfile.Generate(dockerfile.Spec{
		Harness: hrn(t, "claude-code"),
		Profile: "full",
		Shell:   "zsh",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)

	mustContain(t, s, "FROM ubuntu:24.04 AS base")
	mustContain(t, s, "FROM base AS features")
	mustContain(t, s, "FROM features AS harness")
	mustContain(t, s, "FROM harness AS extensions")
	mustContain(t, s, "FROM extensions AS runtime")
	mustContain(t, s, "https://claude.ai/install.sh")
	mustContain(t, s, "CMD [\"/usr/local/bin/claude-exec\", \"/bin/zsh\"]")
	mustContain(t, s, "LABEL vibrator.harness=\"claude-code\"")
	// htop, the docker CLI, and the latest git are baked into the base for
	// EVERY image.
	mustContain(t, s, "htop")
	mustContain(t, s, "docker-ce-cli")
	mustContain(t, s, "ppa:git-core/ppa")
}

func TestGenerate_ShellAffectsCMDAndUserShell(t *testing.T) {
	for _, sh := range []string{"bash", "zsh", "fish"} {
		t.Run(sh, func(t *testing.T) {
			out, err := dockerfile.Generate(dockerfile.Spec{
				Harness: hrn(t, "claude-code"),
				Shell:   sh,
			})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			mustContain(t, string(out), "CMD [\"/usr/local/bin/claude-exec\", \"/bin/"+sh+"\"]")
			mustContain(t, string(out), "useradd -m -s /bin/"+sh)
		})
	}
}

// Regression: USER switch + WORKDIR must happen BEFORE the harness stage
// so claude (Stage 3) and extension entries (Stage 4) install into the
// unprivileged user's home, not /root. Failure mode if this drifts:
// `claude: permission denied` after build, plugins invisible to the user.
func TestGenerate_UserCreationHappensBeforeHarnessStage(t *testing.T) {
	out, err := dockerfile.Generate(dockerfile.Spec{
		Harness: hrn(t, "claude-code"),
		Shell:   "zsh",
		Profile: "backend",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)

	userIdx := strings.Index(s, "USER ${USERNAME}")
	harnessIdx := strings.Index(s, "FROM features AS harness")
	if userIdx < 0 || harnessIdx < 0 {
		t.Fatalf("missing markers: userIdx=%d harnessIdx=%d in:\n%s", userIdx, harnessIdx, s)
	}
	if userIdx > harnessIdx {
		t.Errorf("USER switch (idx=%d) must precede harness stage (idx=%d) — "+
			"otherwise claude installs as root and is unreachable from the user.",
			userIdx, harnessIdx)
	}
}

// Regression: shell rc files for all three shells must be COPY'd into
// /etc/skel/ regardless of which shell is the default, so:
//   - useradd -m later copies them into the unprivileged user's home
//   - any shell invocation (bash, zsh, fish) gets the vibrator PS1 +
//     aliases + welcome banner
//   - zsh-newuser-install never fires (an empty .zshrc would suffice;
//     a real one is strictly better)
//
// Plus the welcome banner script must be installed in a stable
// system-wide location so each rc file can source it.
func TestGenerate_AllShellRcFilesAreCopiedRegardlessOfDefault(t *testing.T) {
	for _, sh := range []string{"bash", "zsh", "fish"} {
		t.Run("default="+sh, func(t *testing.T) {
			out, err := dockerfile.Generate(dockerfile.Spec{
				Harness: hrn(t, "claude-code"),
				Shell:   sh,
			})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			s := string(out)
			// Each rc file lands in /etc/skel/.
			mustContain(t, s, "COPY shells/bashrc /etc/skel/.bashrc")
			mustContain(t, s, "COPY shells/zshrc /etc/skel/.zshrc")
			mustContain(t, s, "COPY shells/config.fish /etc/skel/.config/fish/config.fish")
			// Welcome banner lands in a stable system-wide location.
			mustContain(t, s, "COPY scripts/welcome.sh /opt/vibrator/welcome.sh")
		})
	}
}

// Regression: install-destination ENVs set in Stages 1-2 (as root,
// pointing at system paths) MUST be overridden after the USER switch
// to point at user-writable paths. Otherwise extension installs in
// Stage 4 hit EACCES on /usr/local/* paths. Failure mode is silent
// until an extension actually tries to install a tool that respects
// the env var.
//
// Concretely today: UV_TOOL_BIN_DIR is set to /usr/local/bin in Stage 1
// (correct for root-stage uv tool installs in audit-toolkit feature)
// and MUST be re-set to a user path after USER switch (so a
// 'uv tool install' in an extension at Stage 4 doesn't fail). Same
// shape for NPM_CONFIG_PREFIX.
//
// If you add a new install-direction ENV to Stage 1 without an override
// here, extension installs that depend on that tool will break. Document
// it in the INVARIANT block in writeUserCreation and extend this test.
func TestGenerate_InstallEnvsOverriddenAtUserSwitch(t *testing.T) {
	out, err := dockerfile.Generate(dockerfile.Spec{
		Harness: hrn(t, "claude-code"),
		Shell:   "zsh",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)

	userIdx := strings.Index(s, "USER ${USERNAME}")
	if userIdx < 0 {
		t.Fatalf("USER switch not found in generated Dockerfile")
	}
	postUser := s[userIdx:]

	requiredOverrides := []struct {
		envName  string
		userHome string
	}{
		{"NPM_CONFIG_PREFIX", "/home/${USERNAME}/.npm-global"},
		{"UV_TOOL_BIN_DIR", "/home/${USERNAME}/.local/bin"},
	}
	for _, want := range requiredOverrides {
		needle := "ENV " + want.envName + "=" + want.userHome
		if !strings.Contains(postUser, needle) {
			t.Errorf("missing post-USER override: %q\n"+
				"Without this, any extension that uses the corresponding tool "+
				"will fail with EACCES when writing to its system path.",
				needle)
		}
	}
}

// Regression: the welcome banner reads VIBRATOR_PROFILE / FEATURES_LIST /
// EXTENSIONS_LIST from the container's env at runtime — these must be set
// as ENV (not just LABEL) so they're visible to /bin/sh.
func TestGenerate_VariantMetadataIsEmittedAsEnv(t *testing.T) {
	out, err := dockerfile.Generate(dockerfile.Spec{
		Harness: hrn(t, "claude-code"),
		Shell:   "zsh",
		Profile: "backend",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	mustContain(t, s, `ENV VIBRATOR_PROFILE="backend"`)
	// FEATURES_LIST is emitted even when empty (consistent shape for
	// banner scripts) — value will be "" but the key is present.
	mustContain(t, s, "ENV VIBRATOR_FEATURES_LIST=")
	mustContain(t, s, "ENV VIBRATOR_EXTENSIONS_LIST=")
}

func TestGenerate_FeaturesEmitInGivenOrder(t *testing.T) {
	// feat list is the dep-resolved Registry-order slice. Generator must
	// preserve that order so deps emit before dependents.
	spec := dockerfile.Spec{
		Harness:  hrn(t, "claude-code"),
		Profile:  "full",
		Shell:    "zsh",
		Features: feats(t, "node", "playwright"),
	}
	out, err := dockerfile.Generate(spec)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	nodeIdx := strings.Index(s, "# --- feature: node")
	pwIdx := strings.Index(s, "# --- feature: playwright")
	if nodeIdx < 0 || pwIdx < 0 {
		t.Fatalf("missing feature banners in output:\n%s", s)
	}
	if nodeIdx > pwIdx {
		t.Errorf("node banner should precede playwright banner: node@%d pw@%d", nodeIdx, pwIdx)
	}
}

func TestGenerate_NoFeaturesProducesEmptyFeaturesStage(t *testing.T) {
	out, err := dockerfile.Generate(dockerfile.Spec{
		Harness: hrn(t, "claude-code"),
		Profile: "minimal",
		Shell:   "bash",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	mustContain(t, string(out), "(no features enabled — minimal profile)")
}

func TestGenerate_ExtensionsEmitAlphabetically(t *testing.T) {
	// Pass two extension entries in non-alphabetical order; the generator
	// should still emit them sorted by ID.
	entries := []*extensions.Entry{
		{Harness: "claude-code", ID: "zebra", Kind: extensions.KindPlugin, Install: "RUN echo zebra"},
		{Harness: "claude-code", ID: "alpha", Kind: extensions.KindPlugin, Install: "RUN echo alpha"},
	}
	out, err := dockerfile.Generate(dockerfile.Spec{
		Harness:    hrn(t, "claude-code"),
		Profile:    "full",
		Shell:      "zsh",
		Extensions: entries,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	alphaIdx := strings.Index(s, "# --- extensions/claude-code/alpha")
	zebraIdx := strings.Index(s, "# --- extensions/claude-code/zebra")
	if alphaIdx < 0 || zebraIdx < 0 {
		t.Fatalf("missing extensions banners:\n%s", s)
	}
	if alphaIdx > zebraIdx {
		t.Errorf("alpha should precede zebra in output (got alpha@%d zebra@%d)",
			alphaIdx, zebraIdx)
	}
}

func TestGenerate_HeaderIncludesReproductionCommand(t *testing.T) {
	out, err := dockerfile.Generate(dockerfile.Spec{
		Harness:  hrn(t, "claude-code"),
		Profile:  "backend",
		Shell:    "zsh",
		Features: feats(t, "python", "go"),
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	mustContain(t, string(out), "# Reproduce this Dockerfile with:")
	mustContain(t, string(out), "vibrate build-dockerfile")
	mustContain(t, string(out), "--harness=claude-code")
	mustContain(t, string(out), "--profile=backend")
	mustContain(t, string(out), "--shell=zsh")
}

// --- Golden file tests ---

func TestGolden(t *testing.T) {
	cases := []struct {
		name     string
		spec     dockerfile.Spec
		filename string
	}{
		{
			name: "minimal-claude-code-bash",
			spec: dockerfile.Spec{
				Harness:         hrn(t, "claude-code"),
				Profile:         "minimal",
				Shell:           "bash",
				HostUID:         1000,
				HostGID:         1000,
				Username:        "vibrate",
				VibratorVersion: "test-1.0",
			},
			filename: "minimal-claude-code-bash.dockerfile",
		},
		{
			name: "backend-codex-zsh",
			spec: dockerfile.Spec{
				Harness:         hrn(t, "codex"),
				Profile:         "backend",
				Shell:           "zsh",
				Features:        feats(t, "python", "go", "node", "postgres-client", "gh", "ralphex"),
				HostUID:         1000,
				HostGID:         1000,
				Username:        "vibrate",
				VibratorVersion: "test-1.0",
			},
			filename: "backend-codex-zsh.dockerfile",
		},
		{
			// Plain minimal/zsh variant — no extra features. Confirms the
			// always-on base substrate (docker client, htop, …) is present
			// even on the leanest image; docker is no longer a feature.
			name: "minimal-claude-code-zsh",
			spec: dockerfile.Spec{
				Harness:         hrn(t, "claude-code"),
				Profile:         "minimal",
				Shell:           "zsh",
				HostUID:         1000,
				HostGID:         1000,
				Username:        "vibrate",
				VibratorVersion: "test-1.0",
			},
			filename: "minimal-claude-code-zsh.dockerfile",
		},
		{
			name: "full-claude-code-with-extensions",
			spec: dockerfile.Spec{
				Harness:  hrn(t, "claude-code"),
				Profile:  "full",
				Shell:    "fish",
				Features: feats(t, "python", "node", "ralphex"),
				Extensions: []*extensions.Entry{
					{Harness: "claude-code", ID: "context7", Kind: extensions.KindMCP,
						Source:  "https://github.com/upstash/context7",
						Install: "claude mcp add context7 --scope user --transport http https://mcp.context7.com/mcp"},
					{Harness: "claude-code", ID: "sequential-thinking", Kind: extensions.KindMCP,
						Source:  "https://github.com/modelcontextprotocol/servers",
						Install: "npm install -g @modelcontextprotocol/server-sequential-thinking\nclaude mcp add sequential-thinking --scope user --transport stdio -- mcp-server-sequential-thinking"},
				},
				HostUID:         1000,
				HostGID:         1000,
				Username:        "vibrate",
				VibratorVersion: "test-1.0",
			},
			filename: "full-claude-code-with-extensions.dockerfile",
		},
		{
			// OpenCode installs from a pinned GitHub Releases tarball
			// (multi-line RUN). Golden guards against the generator
			// mangling that fragment.
			name: "frontend-opencode-bash",
			spec: dockerfile.Spec{
				Harness:         hrn(t, "opencode"),
				Profile:         "frontend",
				Shell:           "bash",
				Features:        feats(t, "node", "playwright"),
				HostUID:         1000,
				HostGID:         1000,
				Username:        "vibrate",
				VibratorVersion: "test-1.0",
			},
			filename: "frontend-opencode-bash.dockerfile",
		},
		{
			// Pi installs via npm and verifies `pi --version` (no
			// `|| true`). Golden guards the harness fragment.
			name: "minimal-pi-zsh",
			spec: dockerfile.Spec{
				Harness:         hrn(t, "pi"),
				Profile:         "minimal",
				Shell:           "zsh",
				Features:        feats(t, "node"),
				HostUID:         1000,
				HostGID:         1000,
				Username:        "vibrate",
				VibratorVersion: "test-1.0",
			},
			filename: "minimal-pi-zsh.dockerfile",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := dockerfile.Generate(tc.spec)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			goldPath := filepath.Join("testdata", "golden", tc.filename)

			if *updateGolden {
				if err := os.MkdirAll(filepath.Dir(goldPath), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				if err := os.WriteFile(goldPath, got, 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return
			}

			want, err := os.ReadFile(goldPath)
			if err != nil {
				t.Fatalf("ReadFile %s: %v (run `go test -update` to create)", goldPath, err)
			}
			if string(got) != string(want) {
				t.Errorf("Dockerfile diverges from golden — rerun with -update if change is intentional.\n"+
					"--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("output missing %q\n--- output ---\n%s", needle, haystack)
	}
}

// TestGenerate_EmitsVibratorHarnessEnv asserts the runtime stage sets
// VIBRATOR_HARNESS — claude-exec.sh reads this env to filter the
// integration manifest at session-start time.
func TestGenerate_EmitsVibratorHarnessEnv(t *testing.T) {
	out, err := dockerfile.Generate(dockerfile.Spec{
		Harness: hrn(t, "claude-code"),
		Shell:   "zsh",
		Profile: "minimal",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	mustContain(t, string(out), `ENV VIBRATOR_HARNESS="claude-code"`)
}

// TestGenerate_CopiesIntegrationsManifest pins the COPY directive
// for the build-time manifest. Without it, /etc/vibrator/integrations.json
// is missing in the image and claude-exec.sh's loop silently no-ops.
func TestGenerate_CopiesIntegrationsManifest(t *testing.T) {
	out, err := dockerfile.Generate(dockerfile.Spec{
		Harness: hrn(t, "claude-code"),
		Shell:   "zsh",
		Profile: "minimal",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	mustContain(t, s, "COPY integrations.json /etc/vibrator/integrations.json")
	mustContain(t, s, "RUN mkdir -p /etc/vibrator")
}
