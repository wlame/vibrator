package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPin_RoundtripScalarsAndLists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vb")

	want := &Pin{
		Harness:    "claude-code",
		Profile:    "backend",
		Shell:      "zsh",
		With:       []string{"playwright", "audit-toolkit"},
		No:         []string{"aider"},
		Extensions: []string{"claude-mem", "context7", "serena"},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("roundtrip mismatch\n want: %+v\n got:  %+v", want, got)
	}
}

func TestPin_RoundtripHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vb")

	want := &Pin{
		Harness: "claude-code",
		Hooks: &HookPrefs{
			AcknowledgedMissing: []string{"node", "python"},
		},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("roundtrip mismatch\n want: %+v\n got:  %+v", want, got)
	}
}

func TestPin_IsEmptyWithHooks(t *testing.T) {
	if (Pin{}).IsEmpty() != true {
		t.Fatal("zero pin should be empty")
	}
	p := Pin{Hooks: &HookPrefs{AcknowledgedMissing: []string{"node"}}}
	if p.IsEmpty() {
		t.Error("pin with hook prefs should not be empty")
	}
}

func TestPin_RoundtripIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vb")

	want := &Pin{
		Harness: "claude-code",
		Identity: &Identity{
			Name:  "Ada Alias",
			Email: "ada+vibe@example.com",
		},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// The [identity] table must actually be written.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "[identity]") {
		t.Errorf("expected [identity] table in .vb, got:\n%s", raw)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("roundtrip mismatch\n want: %+v\n got:  %+v", want, got)
	}
}

func TestPin_IsEmptyWithIdentity(t *testing.T) {
	p := Pin{Identity: &Identity{Email: "x@y.z"}}
	if p.IsEmpty() {
		t.Error("pin with identity override should not be empty")
	}
}

func TestPin_RoundtripIntegrations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vb")

	want := &Pin{
		Harness: "claude-code",
		Integrations: map[string]string{
			"serena":     "host",
			"claude-mem": "off",
		},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("roundtrip mismatch\n want: %+v\n got:  %+v", want, got)
	}
}

func TestPin_IntegrationMode(t *testing.T) {
	tests := []struct {
		name string
		pin  Pin
		id   string
		want string
	}{
		{"unset defaults to auto", Pin{}, "serena", IntegrationAuto},
		{"explicit host", Pin{Integrations: map[string]string{"serena": "host"}}, "serena", IntegrationHost},
		{"explicit local", Pin{Integrations: map[string]string{"serena": "local"}}, "serena", IntegrationLocal},
		{"explicit off", Pin{Integrations: map[string]string{"serena": "off"}}, "serena", IntegrationOff},
		{"unknown value falls back to auto", Pin{Integrations: map[string]string{"serena": "bogus"}}, "serena", IntegrationAuto},
		{"different id unaffected", Pin{Integrations: map[string]string{"serena": "host"}}, "claude-mem", IntegrationAuto},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.pin.IntegrationMode(tc.id); got != tc.want {
				t.Errorf("IntegrationMode(%q) = %q, want %q", tc.id, got, tc.want)
			}
		})
	}
}

func TestPin_RoundtripPrereqsAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vb")

	want := &Pin{
		Harness: "claude-code",
		Prereqs: map[string]map[string]string{
			"claude-mem-server-beta": {
				"api_key":    "cmem_deadbeef",
				"team_id":    "team-123",
				"project_id": "proj-456",
			},
			"some-other": {"key": "value"},
		},
		Env: map[string]string{
			"ANTHROPIC_API_KEY": "$ANTHROPIC_API_KEY",
			"FOO":               "bar",
		},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("roundtrip mismatch\n want: %+v\n got:  %+v", want, got)
	}
}

func TestPin_SaveModeIs0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vb")
	if err := Save(path, &Pin{Harness: "claude-code"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// Mask out high bits — we care that group/other have zero access.
	mode := info.Mode().Perm()
	if mode != 0o600 {
		t.Errorf("want mode 0600, got %#o", mode)
	}
}

func TestPin_SaveProducesStableOutput(t *testing.T) {
	// Two saves of equivalent pins should yield byte-identical files.
	// This is what enables `.vb` to be safe to commit (when no prereq
	// secrets are present): random map iteration doesn't reorder keys.
	dir := t.TempDir()
	pathA := filepath.Join(dir, ".vb.a")
	pathB := filepath.Join(dir, ".vb.b")

	p := &Pin{
		Harness: "claude-code",
		Prereqs: map[string]map[string]string{
			"z-prereq": {"z": "1", "a": "2"},
			"a-prereq": {"b": "3", "a": "4"},
		},
		Env: map[string]string{"Z_VAR": "1", "A_VAR": "2"},
	}
	if err := Save(pathA, p); err != nil {
		t.Fatal(err)
	}
	if err := Save(pathB, p); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(pathA)
	b, _ := os.ReadFile(pathB)
	if string(a) != string(b) {
		t.Errorf("two saves of equivalent pins differ:\n--- a ---\n%s\n--- b ---\n%s", a, b)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("want os.ErrNotExist, got %v", err)
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vb")
	_ = os.WriteFile(path, []byte("not toml = = !!!"), 0o600)
	if _, err := Load(path); err == nil {
		t.Errorf("expected decode error, got nil")
	}
}

func TestPin_RoundtripLLMSpec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vb")

	// Cloud provider with env-var auth (Approach C path 1).
	cloudPin := &Pin{
		Harness: "codex",
		LLM: &LLMSpec{
			Provider: "openai",
			Model:    "gpt-4o",
			Auth:     &LLMAuth{Env: "OPENAI_API_KEY"},
		},
	}
	if err := Save(path, cloudPin); err != nil {
		t.Fatalf("Save cloud: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load cloud: %v", err)
	}
	if got.LLM == nil || got.LLM.Provider != "openai" || got.LLM.Model != "gpt-4o" {
		t.Errorf("cloud roundtrip lost data: %#v", got.LLM)
	}
	if got.LLM.Auth == nil || got.LLM.Auth.Env != "OPENAI_API_KEY" || got.LLM.Auth.Value != "" {
		t.Errorf("cloud auth roundtrip wrong: %#v", got.LLM.Auth)
	}

	// Local provider (no auth — Ollama doesn't need a key).
	localPin := &Pin{
		Harness: "pi",
		LLM: &LLMSpec{
			Provider: "ollama",
			Model:    "qwen3:32b",
			BaseURL:  "http://host.docker.internal:11434",
		},
	}
	if err := Save(path, localPin); err != nil {
		t.Fatalf("Save local: %v", err)
	}
	got, err = Load(path)
	if err != nil {
		t.Fatalf("Load local: %v", err)
	}
	if got.LLM.Auth != nil {
		t.Errorf("local provider should have nil Auth, got %#v", got.LLM.Auth)
	}
	if got.LLM.BaseURL == "" {
		t.Errorf("BaseURL should round-trip for local provider")
	}
}

func TestIsEmpty(t *testing.T) {
	if !(&Pin{}).IsEmpty() {
		t.Errorf("zero pin should be empty")
	}
	if (&Pin{Harness: "x"}).IsEmpty() {
		t.Errorf("pin with harness should not be empty")
	}
	if (&Pin{Extensions: []string{"x"}}).IsEmpty() {
		t.Errorf("pin with extensions entry should not be empty")
	}
	if (&Pin{LLM: &LLMSpec{Provider: "ollama", Model: "qwen3"}}).IsEmpty() {
		t.Errorf("pin with LLM should not be empty")
	}
}

func TestFindPin_AtRoot(t *testing.T) {
	dir := t.TempDir()
	pinPath := filepath.Join(dir, ".vb")
	if err := os.WriteFile(pinPath, []byte("harness = \"claude-code\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := FindPin(dir)
	if err != nil {
		t.Fatalf("FindPin: %v", err)
	}
	// Both paths may differ in symlink resolution (macOS /private prefix).
	// Compare the trailing path component for robustness.
	if filepath.Base(got) != PinFileName {
		t.Errorf("want pin file %s, got %s", PinFileName, got)
	}
}

func TestFindPin_WalksUp(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	pinPath := filepath.Join(root, ".vb")
	if err := os.WriteFile(pinPath, []byte("harness = \"codex\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := FindPin(deep)
	if err != nil {
		t.Fatalf("FindPin: %v", err)
	}
	if !strings.HasSuffix(got, PinFileName) {
		t.Errorf("want path ending in %s, got %s", PinFileName, got)
	}
}

func TestFindPin_NotFound(t *testing.T) {
	// Walk up from a dir with no .vb anywhere along the path. The walk stops
	// at filesystem root with ErrNotExist.
	dir := t.TempDir()
	_, err := FindPin(dir)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("want os.ErrNotExist, got %v", err)
	}
}

func TestAppendToGitignore_AddsLineWhenMissing(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gi, []byte("build/\nnode_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := AppendToGitignore(dir, false)
	if err != nil {
		t.Fatalf("AppendToGitignore: %v", err)
	}
	if !changed {
		t.Errorf("expected changed=true")
	}
	content, _ := os.ReadFile(gi)
	if !strings.Contains(string(content), "\n.vb\n") {
		t.Errorf("expected .vb in .gitignore, got:\n%s", content)
	}
	if !strings.Contains(string(content), "\n.vb.lock\n") {
		t.Errorf("expected .vb.lock in .gitignore, got:\n%s", content)
	}
}

// TestAppendToGitignore_IdempotentWhenPresent covers the "both lines
// already present" case: neither ".vb" nor ".vb.lock" needs adding, so the
// file must come back byte-identical.
func TestAppendToGitignore_IdempotentWhenPresent(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	original := "build/\n.vb\n.vb.lock\n"
	if err := os.WriteFile(gi, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := AppendToGitignore(dir, false)
	if err != nil {
		t.Fatalf("AppendToGitignore: %v", err)
	}
	if changed {
		t.Errorf("expected changed=false when .vb and .vb.lock are already listed")
	}
	content, _ := os.ReadFile(gi)
	if string(content) != original {
		t.Errorf(".gitignore was modified despite presence:\n%s", content)
	}
}

// TestAppendToGitignore_AddsLockLineWhenOnlyPinPresent covers a .gitignore
// written before .vb.lock existed (older vibrator, or hand-edited): only
// ".vb" is listed, so the call must add ".vb.lock" and report changed=true.
// Also verifies no duplicate headers: if .vb is already present (with or
// without header), we add only the missing line and no extra header.
func TestAppendToGitignore_AddsLockLineWhenOnlyPinPresent(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	// Start with a .gitignore that already has the header and .vb from a
	// previous run (e.g., from a prior version that wrote both).
	initial := "build/\n# vibrator workspace pin — may contain plaintext prereq tokens\n.vb\n"
	if err := os.WriteFile(gi, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := AppendToGitignore(dir, false)
	if err != nil {
		t.Fatalf("AppendToGitignore: %v", err)
	}
	if !changed {
		t.Errorf("expected changed=true when .vb.lock is still missing")
	}
	content, _ := os.ReadFile(gi)
	contentStr := string(content)
	if !strings.Contains(contentStr, "\n.vb.lock\n") {
		t.Errorf("expected .vb.lock added to .gitignore, got:\n%s", contentStr)
	}

	// Verify exactly one header (no duplicate when only .vb.lock was added).
	headerCount := strings.Count(contentStr, "# vibrator workspace pin")
	if headerCount != 1 {
		t.Errorf("expected exactly 1 header, got %d:\n%s", headerCount, contentStr)
	}

	// Second call: both now present — no further change.
	changed, err = AppendToGitignore(dir, false)
	if err != nil {
		t.Fatalf("AppendToGitignore (2nd): %v", err)
	}
	if changed {
		t.Errorf("expected changed=false once both lines are present")
	}
}

func TestAppendToGitignore_NoFile_DoesNothing(t *testing.T) {
	// No .gitignore at all → no-op, no error. We deliberately don't create
	// .gitignore for projects that don't have one.
	dir := t.TempDir()
	changed, err := AppendToGitignore(dir, false)
	if err != nil {
		t.Fatalf("AppendToGitignore: %v", err)
	}
	if changed {
		t.Errorf("expected changed=false when .gitignore is missing")
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected .gitignore to remain absent, got err=%v", err)
	}
}

func TestPin_NoYoloRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vb")
	if err := Save(path, &Pin{Harness: "claude-code", NoYolo: true}); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.NoYolo {
		t.Error("no_yolo did not roundtrip")
	}
}

func TestIsEmpty_NoYolo(t *testing.T) {
	if (Pin{NoYolo: true}).IsEmpty() {
		t.Error("a pin with NoYolo set is not empty")
	}
}

// TestPin_StripPinnedModelsRoundTrip pins the new field's TOML shape:
// true survives a save/load cycle; false is omitted entirely (zero value
// = keep pins, so old .vb files behave identically).
func TestPin_StripPinnedModelsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vb")
	if err := Save(path, &Pin{Harness: "codex", StripPinnedModels: true}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "strip_pinned_models = true") {
		t.Errorf("serialized pin missing strip_pinned_models: %s", raw)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.StripPinnedModels {
		t.Error("StripPinnedModels lost in round-trip")
	}

	offPath := filepath.Join(dir, ".vb.off")
	if err := Save(offPath, &Pin{Harness: "codex"}); err != nil {
		t.Fatal(err)
	}
	offRaw, err := os.ReadFile(offPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(offRaw), "strip_pinned_models") {
		t.Errorf("zero value must be omitted: %s", offRaw)
	}
}

func TestPinMountsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vb")

	want := &Pin{
		Harness: "claude-code",
		Mounts:  []string{"/data/refs", "/work/lib:rw"},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Mounts) != 2 || got.Mounts[0] != "/data/refs" || got.Mounts[1] != "/work/lib:rw" {
		t.Fatalf("Mounts round-trip = %#v", got.Mounts)
	}
}

func TestAppendToGitignore_NoTrailingNewlineGetsOne(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	// File without trailing newline — common when hand-edited on Windows
	// or pasted from another tool.
	if err := os.WriteFile(gi, []byte("build/"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := AppendToGitignore(dir, false); err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(gi)
	// Original line must remain intact AND .vb must follow on its own line.
	if !strings.HasPrefix(string(content), "build/\n") {
		t.Errorf("clobbered original content:\n%s", content)
	}
	if !strings.Contains(string(content), "\n.vb\n") {
		t.Errorf("missing .vb entry:\n%s", content)
	}
	if !strings.Contains(string(content), "\n.vb.lock\n") {
		t.Errorf("missing .vb.lock entry:\n%s", content)
	}
}

func TestHasSecrets(t *testing.T) {
	cases := []struct {
		name string
		pin  Pin
		want bool
	}{
		{"empty", Pin{}, false},
		{"plain fields only", Pin{Harness: "codex", Shell: "zsh"}, false},
		{"llm env-var auth is not a secret", Pin{LLM: &LLMSpec{Auth: &LLMAuth{Env: "OPENAI_API_KEY"}}}, false},
		{"pasted llm key", Pin{LLM: &LLMSpec{Auth: &LLMAuth{Value: "sk-live"}}}, true},
		{"prereq token", Pin{Prereqs: map[string]map[string]string{"claude-mem-server-beta": {"api_key": "x"}}}, true},
	}
	for _, tc := range cases {
		if got := tc.pin.HasSecrets(); got != tc.want {
			t.Errorf("%s: HasSecrets() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAppendToGitignore_CreatesFileWhenAsked(t *testing.T) {
	dir := t.TempDir()
	changed, err := AppendToGitignore(dir, true)
	if err != nil || !changed {
		t.Fatalf("AppendToGitignore(create) = %v, %v; want true, nil", changed, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "\n.vb\n") && !strings.HasPrefix(content, ".vb\n") {
		t.Errorf(".gitignore missing .vb line: %q", content)
	}
	if !strings.Contains(content, "\n.vb.lock\n") {
		t.Errorf(".gitignore missing .vb.lock line: %q", content)
	}
	// Idempotent on the second call.
	changed, err = AppendToGitignore(dir, true)
	if err != nil || changed {
		t.Errorf("second call = %v, %v; want false, nil", changed, err)
	}
}

func TestAppendToGitignore_NoCreateWithoutFlag(t *testing.T) {
	dir := t.TempDir()
	changed, err := AppendToGitignore(dir, false)
	if err != nil || changed {
		t.Fatalf("= %v, %v; want false, nil", changed, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Error(".gitignore was created despite createIfMissing=false")
	}
}
