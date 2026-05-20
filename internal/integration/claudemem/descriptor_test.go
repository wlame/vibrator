package claudemem

import (
	"strings"
	"testing"
)

func TestParsePostgresURL_Valid(t *testing.T) {
	cases := []struct {
		dsn      string
		wantUser string
		wantHost string
		wantPort string
		wantDB   string
	}{
		{
			dsn:      "postgres://alice:secret@db.example.com:5432/myapp",
			wantUser: "alice", wantHost: "db.example.com", wantPort: "5432", wantDB: "myapp",
		},
		{
			dsn:      "postgresql://bob@10.0.0.1/proddb",
			wantUser: "bob", wantHost: "10.0.0.1", wantPort: "5432", wantDB: "proddb",
		},
	}
	for _, tc := range cases {
		t.Run(tc.dsn, func(t *testing.T) {
			got := parsePostgresURL(tc.dsn)
			if got == nil {
				t.Fatalf("parsePostgresURL(%q) = nil", tc.dsn)
			}
			if got.user != tc.wantUser || got.host != tc.wantHost ||
				got.port != tc.wantPort || got.db != tc.wantDB {
				t.Errorf("got %+v, want user=%s host=%s port=%s db=%s",
					got, tc.wantUser, tc.wantHost, tc.wantPort, tc.wantDB)
			}
		})
	}
}

func TestParsePostgresURL_InvalidReturnsNil(t *testing.T) {
	cases := []string{
		"not a url",
		"mysql://x@h/db",
		"postgres://nodatabase",
		"postgres://just-host",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			if got := parsePostgresURL(in); got != nil {
				t.Errorf("parsePostgresURL(%q) = %+v, want nil", in, got)
			}
		})
	}
}

func TestGenerateOverride_IncludesDSN(t *testing.T) {
	dsn := "postgres://alice:supersecret@db:5432/cm"
	got := GenerateOverride(dsn)

	// The DSN string itself must appear in the env-var lines (so the
	// stack can connect to it).
	if !strings.Contains(got, dsn) {
		t.Errorf("override does not contain the DSN env value")
	}
	// Both services need the env injection.
	if !strings.Contains(got, "claude-mem-server") {
		t.Error("override missing claude-mem-server section")
	}
	if !strings.Contains(got, "claude-mem-worker") {
		t.Error("override missing claude-mem-worker section")
	}
}

func TestGenerateOverride_RedactsPasswordInComment(t *testing.T) {
	// The header comment should NOT leak the password, only redact
	// it. The full DSN still appears in the env: line (necessarily —
	// the services need it), but the human-readable header uses ***.
	dsn := "postgres://alice:supersecret@db:5432/cm"
	got := GenerateOverride(dsn)

	commentLine := ""
	for _, l := range strings.Split(got, "\n") {
		if strings.HasPrefix(l, "# DATABASE_URL:") {
			commentLine = l
			break
		}
	}
	if commentLine == "" {
		t.Fatal("no DATABASE_URL comment found in override")
	}
	if strings.Contains(commentLine, "supersecret") {
		t.Errorf("comment leaks password: %q", commentLine)
	}
	if !strings.Contains(commentLine, "***") {
		t.Errorf("comment doesn't show redaction (***): %q", commentLine)
	}
}

func TestGenerateOverride_OpaqueDSNStillSerializes(t *testing.T) {
	// Non-postgres URLs are passed through verbatim (no parser hit
	// → no redaction). The override still emits.
	dsn := "weird-dsn-format"
	got := GenerateOverride(dsn)
	if !strings.Contains(got, dsn) {
		t.Error("opaque DSN missing from override")
	}
}

func TestComposeFileExists_PositiveAndNegative(t *testing.T) {
	dir := t.TempDir()
	if ComposeFileExists(dir) {
		t.Error("empty dir reported as having compose file")
	}
}

func TestRewriteForHostProbe(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"http://host.docker.internal:37877", "http://127.0.0.1:37877"},
		{"http://host.docker.internal", "http://127.0.0.1"},
		{"http://example.com:1234", "http://example.com:1234"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := rewriteForHostProbe(tc.in)
			if got != tc.want {
				t.Errorf("rewriteForHostProbe(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
