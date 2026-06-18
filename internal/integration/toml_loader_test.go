package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/wlame/vibrator/internal/harness/all"
)

const tomlBookmarks = `
[integration]
id       = "bookmarks"
name     = "Bookmarks MCP"
summary  = "Browser bookmark search"
docs     = "https://example.com/bookmarks"
category = "mcp-tools"

[probe]
url             = "http://127.0.0.1:9100/health"
timeout_seconds = 3

[runtime.docker]
image          = "myorg/bookmarks:latest"
container_name = "vibrate-bookmarks"
ports          = ["127.0.0.1:9100:9100"]
volumes        = ["~/.config/bookmarks:/data"]
restart        = "unless-stopped"
add_hosts      = ["host.docker.internal:host-gateway"]

[[wiring]]
harness = "claude-code"

[wiring.mcp]
name = "bookmarks"

[wiring.mcp.http]
url = "http://host.docker.internal:9100/mcp"

[wiring.mcp.stdio]
command = ["bookmarks-server", "stdio"]
`

func writeToml(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestLoadFromDir_RegistersValidTOML(t *testing.T) {
	resetRegistry()
	dir := t.TempDir()
	writeToml(t, dir, "bookmarks.toml", tomlBookmarks)

	n, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if n != 1 {
		t.Fatalf("loaded = %d, want 1", n)
	}

	integ, ok := Get("bookmarks")
	if !ok {
		t.Fatal("Get(bookmarks) = false")
	}
	if integ.Name != "Bookmarks MCP" {
		t.Errorf("Name = %q, want Bookmarks MCP", integ.Name)
	}
	if len(integ.Runtimes) != 1 {
		t.Fatalf("Runtimes = %d, want 1", len(integ.Runtimes))
	}
	if integ.Runtimes[0].Kind() != "docker" {
		t.Errorf("Runtime kind = %q, want docker", integ.Runtimes[0].Kind())
	}
	if integ.ProbeFn == nil {
		t.Fatal("ProbeFn is nil")
	}
	probe, _ := integ.ProbeFn(context.Background())
	if probe.Describe() != "http://127.0.0.1:9100/health" {
		t.Errorf("probe URL = %q", probe.Describe())
	}
	if len(integ.Wiring) != 1 {
		t.Fatalf("Wiring = %d, want 1", len(integ.Wiring))
	}
	w := integ.Wiring[0]
	if w.Harness != "claude-code" || w.MCP == nil ||
		w.MCP.HTTP == nil || w.MCP.HTTP.URL == "" ||
		w.MCP.Stdio == nil || len(w.MCP.Stdio.Command) != 2 {
		t.Errorf("wiring mismatch: %+v", w)
	}
}

func TestLoadFromDir_MissingDirIsSilent(t *testing.T) {
	resetRegistry()
	n, err := LoadFromDir(filepath.Join(t.TempDir(), "doesnt-exist"))
	if err != nil {
		t.Errorf("LoadFromDir on missing: %v (want nil)", err)
	}
	if n != 0 {
		t.Errorf("loaded = %d, want 0", n)
	}
}

func TestLoadFromDir_EmptyDirReturnsZero(t *testing.T) {
	resetRegistry()
	n, err := LoadFromDir(t.TempDir())
	if err != nil {
		t.Errorf("LoadFromDir on empty: %v", err)
	}
	if n != 0 {
		t.Errorf("loaded = %d, want 0", n)
	}
}

func TestLoadFromDir_IgnoresNonTOMLFiles(t *testing.T) {
	resetRegistry()
	dir := t.TempDir()
	writeToml(t, dir, "readme.md", "# not toml")
	writeToml(t, dir, "valid.toml", tomlBookmarks)

	n, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if n != 1 {
		t.Errorf("loaded = %d, want 1 (only valid.toml)", n)
	}
}

func TestLoadFromDir_RecordsParseErrors(t *testing.T) {
	resetRegistry()
	dir := t.TempDir()
	writeToml(t, dir, "bad.toml", "this is not [valid toml")

	n, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v (loader-level errors should be non-fatal)", err)
	}
	if n != 0 {
		t.Errorf("loaded = %d, want 0", n)
	}
	if errs := LoadErrors(); len(errs) != 1 {
		t.Fatalf("LoadErrors = %d, want 1", len(errs))
	}
}

func TestLoadFromDir_RecordsMissingID(t *testing.T) {
	resetRegistry()
	dir := t.TempDir()
	writeToml(t, dir, "noid.toml", `
[integration]
name = "No ID"
`)

	if _, err := LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	errs := LoadErrors()
	if len(errs) != 1 {
		t.Fatalf("LoadErrors = %d, want 1", len(errs))
	}
}

func TestLoadFromDir_AllowsExternalRuntime(t *testing.T) {
	resetRegistry()
	dir := t.TempDir()
	writeToml(t, dir, "ext.toml", `
[integration]
id   = "ext-only"
name = "External Only"

[runtime.external]
instructions = "set it up by hand"
`)

	if _, err := LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	integ, ok := Get("ext-only")
	if !ok {
		t.Fatal("Get(ext-only) = false")
	}
	if len(integ.Runtimes) != 1 || integ.Runtimes[0].Kind() != "external" {
		t.Errorf("runtimes = %+v, want one external", integ.Runtimes)
	}
}

func TestLoadFromDir_DockerWithoutImageIsRejected(t *testing.T) {
	resetRegistry()
	dir := t.TempDir()
	writeToml(t, dir, "broken.toml", `
[integration]
id   = "broken"
name = "Broken"

[runtime.docker]
container_name = "x"
`)

	if _, err := LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if _, ok := Get("broken"); ok {
		t.Error("Get(broken) should be false — image is required")
	}
	if len(LoadErrors()) == 0 {
		t.Error("expected at least one LoadError for missing image")
	}
}

func TestLoadFromDir_DeterministicOrder(t *testing.T) {
	resetRegistry()
	dir := t.TempDir()
	// Reverse alphabetical filenames; loader should sort.
	writeToml(t, dir, "zulu.toml", `
[integration]
id   = "zulu"
name = "Zulu"
`)
	writeToml(t, dir, "alpha.toml", `
[integration]
id   = "alpha"
name = "Alpha"
`)

	if _, err := LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	all := All()
	if len(all) != 2 || all[0].ID != "alpha" || all[1].ID != "zulu" {
		ids := []string{}
		for _, i := range all {
			ids = append(ids, i.ID)
		}
		t.Errorf("registered order = %v, want [alpha zulu]", ids)
	}
}

func TestLoadFromDir_DuplicateIDRecordedAsError(t *testing.T) {
	resetRegistry()
	dir := t.TempDir()
	writeToml(t, dir, "a.toml", `
[integration]
id   = "dup"
name = "First"
`)
	writeToml(t, dir, "b.toml", `
[integration]
id   = "dup"
name = "Second"
`)

	n, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	// First loads, second should fail with a registration error
	// (Register panics on duplicate; the loader catches and records).
	if n != 1 {
		t.Errorf("loaded = %d, want 1", n)
	}
	if len(LoadErrors()) == 0 {
		t.Error("expected duplicate-ID load error")
	}
}

func TestLoadFromDir_RejectsUnknownWiringHarness(t *testing.T) {
	resetRegistry()
	dir := t.TempDir()
	writeToml(t, dir, "bad.toml", `
[integration]
id   = "bad"
name = "Bad"

[[wiring]]
harness = "unknown-harness-id"

[wiring.mcp]
name = "bad-mcp"

[wiring.mcp.http]
url = "http://example.com/mcp"
`)

	n, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if n != 0 {
		t.Errorf("loaded = %d, want 0 (unknown harness should fail)", n)
	}
	errs := LoadErrors()
	if len(errs) != 1 {
		t.Fatalf("LoadErrors = %d, want 1", len(errs))
	}
	errMsg := errs[0].Error()
	if !strings.Contains(errMsg, "unknown-harness-id") {
		t.Errorf("error should name the unknown harness: %v", errMsg)
	}
	if !strings.Contains(errMsg, "claude-code") {
		t.Errorf("error should list valid harnesses: %v", errMsg)
	}
}

func TestLoadFromDir_AcceptsWildcardWiring(t *testing.T) {
	resetRegistry()
	dir := t.TempDir()
	writeToml(t, dir, "wild.toml", `
[integration]
id   = "wild"
name = "Wildcard"

[[wiring]]
harness = "*"

[wiring.mcp]
name = "wild-mcp"

[wiring.mcp.http]
url = "http://example.com/mcp"
`)

	n, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if n != 1 {
		t.Errorf("loaded = %d, want 1", n)
	}
	integ, ok := Get("wild")
	if !ok {
		t.Fatal("Get(wild) should be true")
	}
	if len(integ.Wiring) != 1 || integ.Wiring[0].Harness != "*" {
		t.Errorf("wiring harness = %q, want *", integ.Wiring[0].Harness)
	}
}
