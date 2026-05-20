package integration

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestBuildManifest_FiltersByHarness(t *testing.T) {
	resetRegistry()

	Register(&Integration{
		ID: "alpha",
		Wiring: []Wiring{
			{Harness: "claudecode", MCP: &MCPWiring{Name: "alpha-mcp",
				HTTP: &MCPHTTP{URL: "http://a"}}},
			{Harness: "codex", MCP: &MCPWiring{Name: "alpha-mcp",
				HTTP: &MCPHTTP{URL: "http://a"}}},
		},
	})
	Register(&Integration{
		ID: "beta",
		Wiring: []Wiring{
			{Harness: "*", MCP: &MCPWiring{Name: "beta-mcp",
				HTTP: &MCPHTTP{URL: "http://b"}}},
		},
	})
	Register(&Integration{
		ID: "gamma",
		Wiring: []Wiring{
			{Harness: "opencode", MCP: &MCPWiring{Name: "gamma-mcp",
				HTTP: &MCPHTTP{URL: "http://c"}}},
		},
	})

	cc := BuildManifest("claudecode")
	ids := make([]string, len(cc))
	for i, e := range cc {
		ids[i] = e.ID
	}
	// Expect alpha (matched explicitly) + beta (matched via *).
	// Gamma is opencode-only and should be filtered.
	if len(cc) != 2 || ids[0] != "alpha" || ids[1] != "beta" {
		t.Errorf("BuildManifest(claudecode) IDs = %v, want [alpha beta]", ids)
	}
}

func TestBuildManifest_StarMatchesAllHarnesses(t *testing.T) {
	resetRegistry()
	Register(&Integration{
		ID: "wild",
		Wiring: []Wiring{
			{Harness: "*", MCP: &MCPWiring{Name: "wild-mcp",
				HTTP: &MCPHTTP{URL: "http://wild"}}},
		},
	})

	for _, h := range []string{"claudecode", "codex", "opencode", "pi"} {
		entries := BuildManifest(h)
		if len(entries) != 1 || entries[0].ID != "wild" {
			t.Errorf("BuildManifest(%s) = %v, want one wild entry", h, entries)
		}
	}
}

func TestBuildManifest_DropsEmptyEntries(t *testing.T) {
	resetRegistry()
	Register(&Integration{
		ID: "void",
		Wiring: []Wiring{
			{Harness: "claudecode"}, // no MCP, no env — drop
		},
	})
	got := BuildManifest("claudecode")
	if len(got) != 0 {
		t.Errorf("BuildManifest = %v, want empty (the wiring had no MCP or env)", got)
	}
}

func TestBuildManifest_DropsMCPWithoutHTTPOrStdio(t *testing.T) {
	resetRegistry()
	Register(&Integration{
		ID: "broken-mcp",
		Wiring: []Wiring{
			{Harness: "claudecode", MCP: &MCPWiring{Name: "broken"}},
		},
	})
	got := BuildManifest("claudecode")
	if len(got) != 0 {
		t.Errorf("BuildManifest = %v, want empty (MCP has no HTTP or Stdio)", got)
	}
}

func TestBuildManifest_DeterministicOrder(t *testing.T) {
	resetRegistry()
	// Register in reverse-alphabetical, expect alphabetical output.
	Register(&Integration{
		ID: "zulu",
		Wiring: []Wiring{
			{Harness: "claudecode", MCP: &MCPWiring{Name: "z",
				HTTP: &MCPHTTP{URL: "http://z"}}},
		},
	})
	Register(&Integration{
		ID: "alpha",
		Wiring: []Wiring{
			{Harness: "claudecode", MCP: &MCPWiring{Name: "a",
				HTTP: &MCPHTTP{URL: "http://a"}}},
		},
	})

	got := BuildManifest("claudecode")
	if len(got) != 2 || got[0].ID != "alpha" || got[1].ID != "zulu" {
		t.Errorf("ordering = %s, %s; want alpha, zulu",
			got[0].ID, got[1].ID)
	}
}

func TestBuildManifest_KeepsEnvOnlyEntries(t *testing.T) {
	resetRegistry()
	Register(&Integration{
		ID: "envonly",
		Wiring: []Wiring{
			{Harness: "claudecode", EnvVars: map[string]string{"K": "V"}},
		},
	})
	got := BuildManifest("claudecode")
	if len(got) != 1 || got[0].EnvVars["K"] != "V" {
		t.Errorf("BuildManifest = %v, want one env-only entry", got)
	}
}

func TestWriteManifest_AlwaysEmitsArray(t *testing.T) {
	resetRegistry()
	var buf bytes.Buffer
	if err := WriteManifest(&buf, "claudecode"); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	// Parse as JSON to confirm it's a valid array literal.
	var got []ManifestEntry
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON array: %v\noutput: %s", err, buf.String())
	}
	if got == nil {
		// json.Unmarshal on "[]" decodes to a nil slice (or zero-len).
		// What we care about is that the output is "[]\n", not "null\n".
		if buf.String() != "[]\n" {
			t.Errorf("empty manifest = %q, want %q", buf.String(), "[]\n")
		}
	}
}

func TestWriteManifest_RoundTrips(t *testing.T) {
	resetRegistry()
	Register(&Integration{
		ID: "rt",
		Wiring: []Wiring{{
			Harness: "claudecode",
			MCP: &MCPWiring{
				Name: "rt",
				HTTP: &MCPHTTP{URL: "http://rt/mcp",
					Headers: map[string]string{"Auth": "x"}},
				Stdio: &MCPStdio{
					Command: []string{"rt-cmd", "--port=1"},
					Env:     map[string]string{"DEBUG": "1"},
				},
			},
			EnvVars: map[string]string{"FOO": "bar"},
		}},
	})

	var buf bytes.Buffer
	if err := WriteManifest(&buf, "claudecode"); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	var entries []ManifestEntry
	if err := json.Unmarshal(buf.Bytes(), &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.ID != "rt" || e.Harness != "claudecode" || e.MCP == nil ||
		e.MCP.HTTP == nil || e.MCP.HTTP.URL != "http://rt/mcp" ||
		e.MCP.HTTP.Headers["Auth"] != "x" ||
		e.MCP.Stdio == nil || len(e.MCP.Stdio.Command) != 2 ||
		e.MCP.Stdio.Env["DEBUG"] != "1" ||
		e.EnvVars["FOO"] != "bar" {
		t.Errorf("round-trip mismatch: %+v", e)
	}
}
