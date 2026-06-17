package serena

import (
	"context"
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/integration"
)

// TestDescriptor_StableShape pins the load-bearing fields of the
// Serena descriptor — these are what the generic runIntegration runner
// reads. A field rename or removal here breaks the manage menu or the
// container-side manifest, so we want a test that fails fast.
func TestDescriptor_StableShape(t *testing.T) {
	t.Setenv("SERENA_PORT", "")
	d := descriptor()

	if d.ID != "serena" {
		t.Errorf("ID = %q, want serena", d.ID)
	}
	if d.Name == "" {
		t.Error("Name is empty")
	}
	if d.Category != "mcp-tools" {
		t.Errorf("Category = %q, want mcp-tools", d.Category)
	}

	// Must offer at least process + docker + external (per the
	// design decision in step 1).
	kinds := map[string]bool{}
	for _, rt := range d.Runtimes {
		kinds[rt.Kind()] = true
	}
	for _, want := range []string{"process", "docker", "external"} {
		if !kinds[want] {
			t.Errorf("Runtimes missing %q (got %v)", want, kinds)
		}
	}

	// Must declare an HTTP probe.
	if d.ProbeFn == nil {
		t.Fatal("ProbeFn is nil")
	}
	probe, err := d.ProbeFn(context.Background())
	if err != nil {
		t.Fatalf("ProbeFn: %v", err)
	}
	if probe == nil {
		t.Fatal("ProbeFn returned nil probe")
	}
	if !strings.Contains(probe.Describe(), "/mcp") {
		t.Errorf("probe URL = %q, expected to end with /mcp", probe.Describe())
	}

	// Must declare an MCP wiring for claude-code with both http and stdio.
	if len(d.Wiring) == 0 {
		t.Fatal("no Wiring declared")
	}
	w := d.Wiring[0]
	if w.Harness != "claude-code" {
		t.Errorf("Wiring[0].Harness = %q, want claude-code", w.Harness)
	}
	if w.MCP == nil || w.MCP.HTTP == nil || w.MCP.Stdio == nil {
		t.Errorf("MCP wiring incomplete: %+v", w.MCP)
	}
}

func TestPortFromEnv_HonorsEnvOverride(t *testing.T) {
	t.Setenv("SERENA_PORT", "9999")
	if got := portFromEnv(); got != 9999 {
		t.Errorf("portFromEnv = %d, want 9999", got)
	}
}

func TestPortFromEnv_DefaultsToConstant(t *testing.T) {
	t.Setenv("SERENA_PORT", "")
	if got := portFromEnv(); got != DefaultPort {
		t.Errorf("portFromEnv = %d, want %d", got, DefaultPort)
	}
}

func TestPortFromEnv_InvalidValueFallsBack(t *testing.T) {
	t.Setenv("SERENA_PORT", "not-a-number")
	if got := portFromEnv(); got != DefaultPort {
		t.Errorf("portFromEnv on garbage = %d, want %d", got, DefaultPort)
	}
}

// Quick smoke: registry registration is what wires the descriptor up
// at program start. If init() drifts away from calling Register, the
// list command silently loses Serena — catch that.
func TestInit_RegistersSerena(t *testing.T) {
	got, ok := integration.Get("serena")
	if !ok {
		t.Fatal("serena not in registry after package init")
	}
	if got.ID != "serena" {
		t.Errorf("Get(serena).ID = %q", got.ID)
	}
}

// The bug this guards: descriptor harness IDs drifting from registry IDs
// made BuildManifest return an empty manifest for claude-code images.
func TestSerenaWiringReachesClaudeCodeManifest(t *testing.T) {
	entries := integration.BuildManifest("claude-code")
	for _, e := range entries {
		if e.ID == "serena" && e.MCP != nil {
			return
		}
	}
	t.Fatalf("BuildManifest(\"claude-code\") has no serena MCP entry: %+v", entries)
}
