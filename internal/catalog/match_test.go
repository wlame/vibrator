package catalog

import (
	"reflect"
	"testing"
)

func TestMatchHostIDs_ExactID(t *testing.T) {
	entries := map[string]*Entry{
		"claude-code/claude-mem": {Harness: "claude-code", ID: "claude-mem", Kind: KindPlugin, Name: "x", Source: "x"},
		"claude-code/context7":   {Harness: "claude-code", ID: "context7", Kind: KindMCP, Name: "x", Source: "x"},
	}
	got := MatchHostIDs(entries, "claude-code", []string{"claude-mem", "unknown"})
	if !reflect.DeepEqual(got, []string{"claude-mem"}) {
		t.Errorf("got %v, want [claude-mem]", got)
	}
}

func TestMatchHostIDs_ViaAlias(t *testing.T) {
	// Host reports plugin name "playwright" but our catalog ID is
	// "playwright-mcp" — bridge via HostAliases.
	entries := map[string]*Entry{
		"claude-code/playwright-mcp": {
			Harness: "claude-code", ID: "playwright-mcp", Kind: KindMCP,
			Name: "x", Source: "x",
			HostAliases: []string{"playwright"},
		},
	}
	got := MatchHostIDs(entries, "claude-code", []string{"playwright"})
	if !reflect.DeepEqual(got, []string{"playwright-mcp"}) {
		t.Errorf("got %v, want [playwright-mcp] via alias", got)
	}
}

func TestMatchHostIDs_FiltersByHarness(t *testing.T) {
	// An entry for codex with the same ID should NOT match a claude-code
	// query — harness scoping prevents cross-harness collisions.
	entries := map[string]*Entry{
		"claude-code/github": {Harness: "claude-code", ID: "github", Kind: KindMCP, Name: "x", Source: "x"},
		"codex/github":       {Harness: "codex", ID: "github", Kind: KindPlugin, Name: "x", Source: "x"},
	}
	got := MatchHostIDs(entries, "claude-code", []string{"github"})
	if !reflect.DeepEqual(got, []string{"github"}) {
		t.Errorf("got %v, want [github]", got)
	}
}

func TestMatchHostIDs_EmptyInputs(t *testing.T) {
	entries := map[string]*Entry{
		"claude-code/a": {Harness: "claude-code", ID: "a", Kind: KindPlugin, Name: "x", Source: "x"},
	}
	if got := MatchHostIDs(entries, "claude-code", nil); got != nil {
		t.Errorf("nil hostIDs should return nil, got %v", got)
	}
	if got := MatchHostIDs(nil, "claude-code", []string{"a"}); got != nil {
		t.Errorf("nil entries should return nil, got %v", got)
	}
}

func TestMatchHostIDs_SortsResults(t *testing.T) {
	entries := map[string]*Entry{
		"claude-code/zeta":  {Harness: "claude-code", ID: "zeta", Kind: KindPlugin, Name: "x", Source: "x"},
		"claude-code/alpha": {Harness: "claude-code", ID: "alpha", Kind: KindPlugin, Name: "x", Source: "x"},
	}
	got := MatchHostIDs(entries, "claude-code", []string{"zeta", "alpha"})
	if !reflect.DeepEqual(got, []string{"alpha", "zeta"}) {
		t.Errorf("got %v, want [alpha zeta] (sorted)", got)
	}
}
