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
		Harness: "claude-code",
		Profile: "backend",
		Shell:   "zsh",
		With:    []string{"playwright", "audit-toolkit"},
		No:      []string{"aider"},
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

	changed, err := AppendToGitignore(dir)
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
}

func TestAppendToGitignore_IdempotentWhenPresent(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	original := "build/\n.vb\n"
	if err := os.WriteFile(gi, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := AppendToGitignore(dir)
	if err != nil {
		t.Fatalf("AppendToGitignore: %v", err)
	}
	if changed {
		t.Errorf("expected changed=false when .vb already listed")
	}
	content, _ := os.ReadFile(gi)
	if string(content) != original {
		t.Errorf(".gitignore was modified despite presence:\n%s", content)
	}
}

func TestAppendToGitignore_NoFile_DoesNothing(t *testing.T) {
	// No .gitignore at all → no-op, no error. We deliberately don't create
	// .gitignore for projects that don't have one.
	dir := t.TempDir()
	changed, err := AppendToGitignore(dir)
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

func TestAppendToGitignore_NoTrailingNewlineGetsOne(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	// File without trailing newline — common when hand-edited on Windows
	// or pasted from another tool.
	if err := os.WriteFile(gi, []byte("build/"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := AppendToGitignore(dir); err != nil {
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
}
