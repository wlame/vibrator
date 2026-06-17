package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomic0600_TightensExistingPerms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vb")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteFileAtomic0600(path, []byte("secret = \"x\"\n")); err != nil {
		t.Fatalf("WriteFileAtomic0600: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 0600 (os.WriteFile keeps a pre-existing file's mode; the atomic helper must not)", got)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "secret = \"x\"\n" {
		t.Errorf("content = %q", data)
	}

	// No temp litter.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".vb-tmp-") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}

func TestSave_TightensExistingPerms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vb")
	if err := os.WriteFile(path, []byte("# junk"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &Pin{Harness: "claude-code", Prereqs: map[string]map[string]string{
		"claude-mem-server-beta": {"api_key": "cmem_secret"},
	}}
	if err := Save(path, p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode after Save = %o, want 0600", got)
	}
}
