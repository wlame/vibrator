package cli

import (
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/integration"
)

func TestFirstMCPURL_PicksHTTPFromFirstWiring(t *testing.T) {
	integ := &integration.Integration{
		ID: "x",
		Wiring: []integration.Wiring{
			{Harness: "claudecode", MCP: &integration.MCPWiring{
				Name: "x",
				HTTP: &integration.MCPHTTP{URL: "http://primary"},
			}},
			{Harness: "codex", MCP: &integration.MCPWiring{
				Name: "x",
				HTTP: &integration.MCPHTTP{URL: "http://secondary"},
			}},
		},
	}
	if got := firstMCPURL(integ); got != "http://primary" {
		t.Errorf("firstMCPURL = %q, want http://primary", got)
	}
}

func TestFirstMCPURL_EmptyWhenNoHTTP(t *testing.T) {
	integ := &integration.Integration{
		ID: "stdio-only",
		Wiring: []integration.Wiring{
			{Harness: "claudecode", MCP: &integration.MCPWiring{
				Name:  "x",
				Stdio: &integration.MCPStdio{Command: []string{"cmd"}},
			}},
		},
	}
	if got := firstMCPURL(integ); got != "" {
		t.Errorf("firstMCPURL = %q, want empty", got)
	}
}

func TestFirstMCPURL_EmptyWhenNoMCP(t *testing.T) {
	integ := &integration.Integration{
		ID: "no-mcp",
		Wiring: []integration.Wiring{
			{Harness: "claudecode", EnvVars: map[string]string{"K": "V"}},
		},
	}
	if got := firstMCPURL(integ); got != "" {
		t.Errorf("firstMCPURL = %q, want empty", got)
	}
}

func TestIndent(t *testing.T) {
	cases := []struct {
		name       string
		in, prefix string
		want       string
	}{
		{
			name:   "single-line",
			in:     "hello",
			prefix: "  ",
			want:   "  hello",
		},
		{
			name:   "multi-line",
			in:     "line1\nline2",
			prefix: "  ",
			want:   "  line1\n  line2",
		},
		{
			name:   "preserves-blank-lines",
			in:     "line1\n\nline2",
			prefix: "  ",
			want:   "  line1\n\n  line2",
		},
		{
			// Blank lines stay blank (no prefix added). Same applies
			// to the all-empty case — output is empty too.
			name:   "empty-input",
			in:     "",
			prefix: "  ",
			want:   "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := indent(tc.in, tc.prefix)
			if got != tc.want {
				t.Errorf("indent = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseDSN_RoundTripsViaCMBuild(t *testing.T) {
	cases := []struct {
		host, port, user, pw, db string
	}{
		{"localhost", "5432", "alice", "secret", "appdb"},
		{"10.0.0.1", "5433", "bob", "", "main"},
		// Password with chars that get URL-encoded by cmBuildDSN.
		{"db", "5432", "carol", "p@ss", "test"},
	}
	for _, tc := range cases {
		t.Run(tc.host+"-"+tc.user, func(t *testing.T) {
			dsn := cmBuildDSN(tc.host, tc.port, tc.user, tc.pw, tc.db)
			if !strings.HasPrefix(dsn, "postgres://") {
				t.Fatalf("cmBuildDSN returned %q, want postgres://...", dsn)
			}
			h, p, u, _, d, ok := parseDSN(dsn)
			if !ok {
				t.Fatalf("parseDSN(%q) ok=false", dsn)
			}
			if h != tc.host || p != tc.port || u != tc.user || d != tc.db {
				t.Errorf("round-trip: %s:%s/%s/%s, want %s:%s/%s/%s",
					h, p, u, d, tc.host, tc.port, tc.user, tc.db)
			}
		})
	}
}

func TestParseDSN_InvalidReturnsFalse(t *testing.T) {
	cases := []string{
		"not a url",
		"mysql://x@h/db",
		"postgres://no-at-sign",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			_, _, _, _, _, ok := parseDSN(in)
			if ok {
				t.Errorf("parseDSN(%q) ok=true, want false", in)
			}
		})
	}
}

func TestCMBuildDSN_DefaultPort(t *testing.T) {
	dsn := cmBuildDSN("h", "", "u", "", "d")
	if !strings.Contains(dsn, ":5432/") {
		t.Errorf("cmBuildDSN with empty port = %q, want :5432/", dsn)
	}
}

func TestCMBuildDSN_NoPasswordOmitsColon(t *testing.T) {
	dsn := cmBuildDSN("h", "5432", "u", "", "d")
	want := "postgres://u@h:5432/d"
	if dsn != want {
		t.Errorf("cmBuildDSN no-pw = %q, want %q", dsn, want)
	}
}
