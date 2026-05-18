package dockerfile_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/catalog"
	"github.com/wlame/vibrator/internal/dockerfile"
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
	mustContain(t, s, "FROM harness AS catalog")
	mustContain(t, s, "FROM catalog AS runtime")
	mustContain(t, s, "https://claude.ai/install.sh")
	mustContain(t, s, "CMD [\"/bin/zsh\"]")
	mustContain(t, s, "LABEL vibrator.harness=\"claude-code\"")
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
			mustContain(t, string(out), "CMD [\"/bin/"+sh+"\"]")
			mustContain(t, string(out), "useradd -m -s /bin/"+sh)
		})
	}
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

func TestGenerate_CatalogEntriesEmitAlphabetically(t *testing.T) {
	// Pass two catalog entries in non-alphabetical order; the generator
	// should still emit them sorted by ID.
	entries := []*catalog.Entry{
		{Harness: "claude-code", ID: "zebra", Kind: catalog.KindPlugin, Install: "RUN echo zebra"},
		{Harness: "claude-code", ID: "alpha", Kind: catalog.KindPlugin, Install: "RUN echo alpha"},
	}
	out, err := dockerfile.Generate(dockerfile.Spec{
		Harness:        hrn(t, "claude-code"),
		Profile:        "full",
		Shell:          "zsh",
		CatalogEntries: entries,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	alphaIdx := strings.Index(s, "# --- catalog/claude-code/alpha")
	zebraIdx := strings.Index(s, "# --- catalog/claude-code/zebra")
	if alphaIdx < 0 || zebraIdx < 0 {
		t.Fatalf("missing catalog banners:\n%s", s)
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
			name: "full-claude-code-with-catalog",
			spec: dockerfile.Spec{
				Harness:  hrn(t, "claude-code"),
				Profile:  "full",
				Shell:    "fish",
				Features: feats(t, "python", "node", "ralphex"),
				CatalogEntries: []*catalog.Entry{
					{Harness: "claude-code", ID: "context7", Kind: catalog.KindMCP,
						Source: "https://github.com/upstash/context7",
						Install: "claude mcp add context7 --scope user --transport http https://mcp.context7.com/mcp"},
					{Harness: "claude-code", ID: "sequential-thinking", Kind: catalog.KindMCP,
						Source: "https://github.com/modelcontextprotocol/servers",
						Install: "npm install -g @modelcontextprotocol/server-sequential-thinking\nclaude mcp add sequential-thinking --scope user --transport stdio -- mcp-server-sequential-thinking"},
				},
				HostUID:         1000,
				HostGID:         1000,
				Username:        "vibrate",
				VibratorVersion: "test-1.0",
			},
			filename: "full-claude-code-with-catalog.dockerfile",
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
