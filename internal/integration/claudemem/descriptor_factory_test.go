package claudemem

import (
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/integration"
)

// TestDescriptor_StableShape pins the load-bearing fields of the
// claude-mem descriptor. Same intent as the Serena equivalent.
func TestDescriptor_StableShape(t *testing.T) {
	d := descriptor()

	if d.ID != "claude-mem" {
		t.Errorf("ID = %q, want claude-mem", d.ID)
	}
	if d.Workspace == nil {
		t.Error("WorkspaceDriver is nil — workspace bootstrap won't run")
	}
	if d.AdminConfig == nil {
		t.Error("AdminConfig is nil — config form has no anchor")
	}

	kinds := map[string]bool{}
	for _, rt := range d.Runtimes {
		kinds[rt.Kind()] = true
	}
	for _, want := range []string{"compose", "external"} {
		if !kinds[want] {
			t.Errorf("Runtimes missing %q (got %v)", want, kinds)
		}
	}
}

func TestInit_RegistersClaudeMem(t *testing.T) {
	got, ok := integration.Get("claude-mem")
	if !ok {
		t.Fatal("claude-mem not in registry after package init")
	}
	if got.Workspace == nil || got.Workspace.PrereqID() == "" {
		t.Error("registered claude-mem missing WorkspaceDriver.PrereqID")
	}
}

func TestDynamicComposeRuntime_KindAndLabel(t *testing.T) {
	r := &dynamicComposeRuntime{}
	if r.Kind() != "compose" {
		t.Errorf("Kind = %q", r.Kind())
	}
	if !strings.Contains(r.Label(), "Compose") {
		t.Errorf("Label = %q", r.Label())
	}
}
