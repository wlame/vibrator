package prereq

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/docker"
)

// --- key minting & hashing -----------------------------------------------

func TestMintClaudeMemKey_Format(t *testing.T) {
	// Deterministic RNG → known prefix + 48 hex chars (24 bytes × 2).
	key, err := mintClaudeMemKey(bytes.NewReader(bytes.Repeat([]byte{0xab}, 24)))
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	want := "cmem_" + strings.Repeat("ab", 24)
	if key != want {
		t.Errorf("key = %q, want %q", key, want)
	}
}

func TestMintClaudeMemKey_RandomBytes(t *testing.T) {
	// Two calls with crypto/rand should produce distinct keys with the
	// expected length (prefix + 48 hex chars = 53 chars).
	a, _ := mintClaudeMemKey(nil)
	b, _ := mintClaudeMemKey(nil)
	if a == b {
		t.Errorf("two calls returned identical keys: %s", a)
	}
	if len(a) != len("cmem_")+48 {
		t.Errorf("key length = %d, want %d", len(a), len("cmem_")+48)
	}
}

func TestSha256Hex(t *testing.T) {
	// Empty input → well-known SHA-256 hash of zero bytes.
	const empty = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := sha256Hex(""); got != empty {
		t.Errorf("sha256Hex(\"\") = %q, want %q", got, empty)
	}
}

// --- DSN rewrite -----------------------------------------------------------

func TestRewriteForOneshot(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// localhost without userinfo
		{"postgres://localhost:5432/db", "postgres://host.docker.internal:5432/db"},
		// 127.0.0.1 without userinfo
		{"postgres://127.0.0.1:5432/db", "postgres://host.docker.internal:5432/db"},
		// localhost with userinfo
		{"postgres://user:pass@localhost:5432/db", "postgres://user:pass@host.docker.internal:5432/db"},
		// 127.0.0.1 with userinfo
		{"postgres://u:p@127.0.0.1/db", "postgres://u:p@host.docker.internal/db"},
		// non-localhost host left alone
		{"postgres://db.example.com:5432/x", "postgres://db.example.com:5432/x"},
		// host.docker.internal already in URL — should remain unchanged
		{"postgres://host.docker.internal:5432/db", "postgres://host.docker.internal:5432/db"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := rewriteForOneshot(tc.in); got != tc.want {
				t.Errorf("rewriteForOneshot(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRewriteForHostProbe(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// HTTP URL with port — the common claude-mem server case
		{"http://host.docker.internal:37877", "http://127.0.0.1:37877"},
		// HTTP URL with port and path
		{"http://host.docker.internal:37877/healthz", "http://127.0.0.1:37877/healthz"},
		// Already 127.0.0.1 — left alone
		{"http://127.0.0.1:37877", "http://127.0.0.1:37877"},
		// localhost — left alone (not our concern; this rewriter only
		// handles the inverse direction)
		{"http://localhost:37877", "http://localhost:37877"},
		// Unrelated host — left alone
		{"http://api.example.com:443/v1", "http://api.example.com:443/v1"},
		// With userinfo (uncommon for HTTP, but the regex supports it)
		{"http://u:p@host.docker.internal:8080/", "http://u:p@127.0.0.1:8080/"},
		// postgres-style DSN — also rewritten (the regex is scheme-agnostic)
		{"postgres://u:p@host.docker.internal:5432/db", "postgres://u:p@127.0.0.1:5432/db"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := rewriteForHostProbe(tc.in); got != tc.want {
				t.Errorf("rewriteForHostProbe(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// --- ClaudeMemPrereq factory ----------------------------------------------

func TestClaudeMemPrereq_WithoutConfig_ReturnsFailingVerifier(t *testing.T) {
	p := ClaudeMemPrereq(nil, nil)
	if p == nil {
		t.Fatal("expected non-nil Prereq")
	}
	if p.ID != ClaudeMemPrereqID {
		t.Errorf("ID = %q, want %q", p.ID, ClaudeMemPrereqID)
	}
	r := p.Verifier.Verify(context.Background())
	if r.OK {
		t.Errorf("expected Verify to fail when config is nil")
	}
	if r.Hint == "" {
		t.Errorf("expected setup hint in failing verifier message")
	}
	if p.Bootstrapper != nil {
		t.Errorf("expected nil Bootstrapper when no admin config")
	}
}

func TestClaudeMemPrereq_WithURL_HasReachabilityProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := ClaudeMemPrereq(&ClaudeMemAdminConfig{ServerURL: srv.URL}, nil)
	r := p.Verifier.Verify(context.Background())
	if !r.OK {
		t.Errorf("expected probe to succeed against test server: %v", r)
	}
}

func TestClaudeMemPrereq_WithDBAndDocker_HasBootstrapper(t *testing.T) {
	m := docker.NewMock()
	cfg := &ClaudeMemAdminConfig{
		ServerURL:   "http://host.docker.internal:37877",
		DatabaseURL: "postgres://u:p@localhost:5432/db",
	}
	p := ClaudeMemPrereq(cfg, m)
	if p.Bootstrapper == nil {
		t.Errorf("expected non-nil Bootstrapper when DatabaseURL + docker client are set")
	}
}

// --- Admin config path ----------------------------------------------------

func TestClaudeMemAdminConfigPath_HonorsEnvOverride(t *testing.T) {
	t.Setenv("VIBRATOR_CLAUDE_MEM_CONFIG", "/tmp/override.toml")
	if got := ClaudeMemAdminConfigPath(); got != "/tmp/override.toml" {
		t.Errorf("path = %q, want override", got)
	}
}

func TestClaudeMemAdminConfigPath_HonorsXDG(t *testing.T) {
	t.Setenv("VIBRATOR_CLAUDE_MEM_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/srv/xdg")
	got := ClaudeMemAdminConfigPath()
	if got != "/srv/xdg/vibrator/claude-mem.toml" {
		t.Errorf("path = %q, want XDG path", got)
	}
}

func TestLoadClaudeMemAdminConfig_RoundtripsTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cm.toml")
	if err := os.WriteFile(path, []byte(`
runtime = "server-beta"
server_url = "http://host.docker.internal:37877"
database_url = "postgres://u:p@localhost:5432/cm"
team_name = "my-team"
`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Setenv("VIBRATOR_CLAUDE_MEM_CONFIG", path)

	cfg, err := LoadClaudeMemAdminConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Runtime != "server-beta" {
		t.Errorf("Runtime = %q, want server-beta", cfg.Runtime)
	}
	if cfg.ServerURL == "" || cfg.DatabaseURL == "" || cfg.TeamName != "my-team" {
		t.Errorf("config fields not loaded: %#v", cfg)
	}
}

func TestLoadClaudeMemAdminConfig_MissingFileSurfacesErrNotExist(t *testing.T) {
	t.Setenv("VIBRATOR_CLAUDE_MEM_CONFIG", filepath.Join(t.TempDir(), "nope.toml"))
	_, err := LoadClaudeMemAdminConfig()
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}

func TestSaveClaudeMemAdminConfig_TightensExistingPerms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claude-mem.toml")
	t.Setenv("VIBRATOR_CLAUDE_MEM_CONFIG", path)
	if err := os.WriteFile(path, []byte("# junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveClaudeMemAdminConfig(&ClaudeMemAdminConfig{}); err != nil {
		t.Fatalf("SaveClaudeMemAdminConfig: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 0600", got)
	}
}

// --- Bootstrap flow -------------------------------------------------------

// fakePSQL is a scripted docker.Run handler that returns canned stdout
// values in the order they're listed. Each "call" represents one psql
// invocation; the handler reads stdin (the SQL) and writes the canned
// response to stdout — matching what real psql in -tA mode would produce.
type fakePSQL struct {
	responses []string // stdouts to return, in order
	calls     []psqlCall
}

type psqlCall struct {
	argv []string // the full `docker run` command vector
	sql  string   // the SQL piped on stdin
}

func (f *fakePSQL) handle(_ context.Context, spec docker.RunSpec) error {
	if len(f.responses) == 0 {
		return errors.New("fakePSQL: no more scripted responses")
	}
	// Capture stdin and argv (Image + Cmd) for assertion later.
	var sqlBuf bytes.Buffer
	if spec.Stdin != nil {
		if _, err := io.Copy(&sqlBuf, spec.Stdin); err != nil {
			return err
		}
	}
	argv := append([]string{spec.Image}, spec.Cmd...)
	f.calls = append(f.calls, psqlCall{argv: argv, sql: sqlBuf.String()})

	// Emit the next canned response to the caller's Stdout.
	resp := f.responses[0]
	f.responses = f.responses[1:]
	if spec.Stdout != nil {
		_, _ = io.WriteString(spec.Stdout, resp)
	}
	return nil
}

func TestClaudeMemBootstrap_HappyPath(t *testing.T) {
	mock := docker.NewMock()
	psql := &fakePSQL{
		responses: []string{
			"",                 // team SELECT → not found
			"team-uuid-aaa",    // team INSERT → created
			"",                 // project SELECT → not found
			"project-uuid-bbb", // project INSERT → created
			"",                 // key rotation (no rows returned)
		},
	}
	mock.RunHandler = psql.handle

	b := &ClaudeMemBootstrap{
		Docker:      mock,
		DatabaseURL: "postgres://u:p@localhost:5432/cm",
		// rng deterministic so we know what the key looks like
		rng: bytes.NewReader(bytes.Repeat([]byte{0xcc}, 24)),
	}

	got, err := b.Bootstrap(context.Background(), Workspace{
		Path:        "/Users/wlame/code/vibrator",
		ProjectName: "vibrator",
		Hostname:    "wlame-mbp",
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	// Expected key derivation:
	wantKey := "cmem_" + strings.Repeat("cc", 24)
	if got["api_key"] != wantKey {
		t.Errorf("api_key = %q, want %q", got["api_key"], wantKey)
	}
	if got["team_id"] != "team-uuid-aaa" || got["project_id"] != "project-uuid-bbb" {
		t.Errorf("team/project ids wrong: %v", got)
	}
	if !strings.HasPrefix(got["actor_id"], "vibrator:wlame-mbp:") {
		t.Errorf("actor_id wrong: %q", got["actor_id"])
	}

	// Exactly 5 psql invocations (team select, team insert, project select,
	// project insert, key rotation).
	if len(psql.calls) != 5 {
		t.Fatalf("expected 5 psql calls, got %d", len(psql.calls))
	}

	// Localhost was rewritten on every call.
	for i, c := range psql.calls {
		if !strings.Contains(strings.Join(c.argv, " "), "host.docker.internal") {
			t.Errorf("call %d did not rewrite localhost: argv=%v", i, c.argv)
		}
	}

	// SQL content sanity check: the rotation call uses both UPDATE and INSERT.
	last := psql.calls[4].sql
	if !strings.Contains(last, "UPDATE api_keys") || !strings.Contains(last, "INSERT INTO api_keys") {
		t.Errorf("key rotation SQL missing UPDATE/INSERT: %s", last)
	}
}

func TestClaudeMemBootstrap_ExistingTeamSkipsInsert(t *testing.T) {
	mock := docker.NewMock()
	psql := &fakePSQL{
		responses: []string{
			"existing-team-id", // team SELECT → found, skip INSERT
			"existing-proj-id", // project SELECT → found, skip INSERT
			"",                 // key rotation
		},
	}
	mock.RunHandler = psql.handle

	b := &ClaudeMemBootstrap{
		Docker:      mock,
		DatabaseURL: "postgres://localhost/db",
		rng:         bytes.NewReader(bytes.Repeat([]byte{0x11}, 24)),
	}
	got, err := b.Bootstrap(context.Background(), Workspace{
		Path: "/p", ProjectName: "p", Hostname: "h",
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if got["team_id"] != "existing-team-id" || got["project_id"] != "existing-proj-id" {
		t.Errorf("expected existing ids, got %v", got)
	}
	// Only 3 calls: team SELECT, project SELECT, key rotation.
	if len(psql.calls) != 3 {
		t.Errorf("expected 3 psql calls (skipping inserts), got %d", len(psql.calls))
	}
}

func TestClaudeMemBootstrap_FailsWithoutDocker(t *testing.T) {
	b := &ClaudeMemBootstrap{DatabaseURL: "postgres://x"}
	_, err := b.Bootstrap(context.Background(), Workspace{ProjectName: "p"})
	if err == nil || !strings.Contains(err.Error(), "docker client is nil") {
		t.Errorf("expected nil-docker error, got %v", err)
	}
}

func TestClaudeMemBootstrap_FailsWithoutDatabaseURL(t *testing.T) {
	b := &ClaudeMemBootstrap{Docker: docker.NewMock()}
	_, err := b.Bootstrap(context.Background(), Workspace{ProjectName: "p"})
	if err == nil || !strings.Contains(err.Error(), "DatabaseURL is empty") {
		t.Errorf("expected DatabaseURL error, got %v", err)
	}
}

func TestClaudeMemBootstrap_FailsWithoutProjectName(t *testing.T) {
	b := &ClaudeMemBootstrap{Docker: docker.NewMock(), DatabaseURL: "postgres://x"}
	_, err := b.Bootstrap(context.Background(), Workspace{})
	if err == nil || !strings.Contains(err.Error(), "ProjectName is empty") {
		t.Errorf("expected ProjectName error, got %v", err)
	}
}

func TestClaudeMemBootstrap_PropagatesPsqlError(t *testing.T) {
	mock := docker.NewMock()
	mock.RunHandler = func(_ context.Context, _ docker.RunSpec) error {
		return errors.New("connection refused")
	}
	b := &ClaudeMemBootstrap{
		Docker:      mock,
		DatabaseURL: "postgres://localhost/db",
		rng:         bytes.NewReader(bytes.Repeat([]byte{0}, 24)),
	}
	_, err := b.Bootstrap(context.Background(), Workspace{
		Path: "/p", ProjectName: "p", Hostname: "h",
	})
	if err == nil || !strings.Contains(err.Error(), "lookup team") {
		t.Errorf("expected wrapped lookup-team error, got %v", err)
	}
}
