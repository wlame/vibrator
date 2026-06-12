package mount

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantPath string
		wantRO   bool
		wantErr  bool
	}{
		{"bare path is read-only", "/data/refs", "/data/refs", true, false},
		{"explicit ro", "/data/refs:ro", "/data/refs", true, false},
		{"explicit rw", "/work/lib:rw", "/work/lib", false, false},
		{"relative path kept as-is", "refs", "refs", true, false},
		{"trailing slash preserved", "/data/refs/:rw", "/data/refs/", false, false},
		{"unknown suffix errors", "/data/refs:xy", "", false, true},
		{"empty path errors", "", "", false, true},
		{"empty path with mode errors", ":rw", "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q): want error, got nil", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q): unexpected error: %v", tt.raw, err)
			}
			if got.HostPath != tt.wantPath || got.ReadOnly != tt.wantRO {
				t.Fatalf("Parse(%q) = {%q, ro=%v}, want {%q, ro=%v}",
					tt.raw, got.HostPath, got.ReadOnly, tt.wantPath, tt.wantRO)
			}
		})
	}
}

func TestResolveAll(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	ws := t.TempDir()
	file := filepath.Join(dirA, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("resolves dirs and modes", func(t *testing.T) {
		got, err := ResolveAll([]string{dirA, dirB + ":rw"}, ws)
		if err != nil {
			t.Fatalf("ResolveAll: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d resolved, want 2", len(got))
		}
		if got[0].Path != dirA || !got[0].ReadOnly {
			t.Fatalf("entry 0 = %+v", got[0])
		}
		if got[1].Path != dirB || got[1].ReadOnly {
			t.Fatalf("entry 1 = %+v", got[1])
		}
	})

	t.Run("missing path errors", func(t *testing.T) {
		if _, err := ResolveAll([]string{filepath.Join(dirA, "nope")}, ws); err == nil {
			t.Fatal("want error for missing path, got nil")
		}
	})

	t.Run("regular file errors", func(t *testing.T) {
		if _, err := ResolveAll([]string{file}, ws); err == nil {
			t.Fatal("want error for non-directory, got nil")
		}
	})

	t.Run("relative path resolved against cwd", func(t *testing.T) {
		// dirA's base name, resolved from dirA's parent as cwd.
		parent := filepath.Dir(dirA)
		cwd, _ := os.Getwd()
		t.Cleanup(func() { _ = os.Chdir(cwd) })
		if err := os.Chdir(parent); err != nil {
			t.Fatal(err)
		}
		got, err := ResolveAll([]string{filepath.Base(dirA)}, ws)
		if err != nil {
			t.Fatalf("ResolveAll relative: %v", err)
		}
		if got[0].Path != dirA {
			t.Fatalf("relative resolved to %q, want %q", got[0].Path, dirA)
		}
	})

	t.Run("workspace path dropped", func(t *testing.T) {
		got, err := ResolveAll([]string{ws, dirA}, ws)
		if err != nil {
			t.Fatalf("ResolveAll: %v", err)
		}
		if len(got) != 1 || got[0].Path != dirA {
			t.Fatalf("workspace not dropped: %+v", got)
		}
	})

	t.Run("exact duplicate collapsed", func(t *testing.T) {
		got, err := ResolveAll([]string{dirA, dirA}, ws)
		if err != nil {
			t.Fatalf("ResolveAll: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("duplicate not collapsed: %+v", got)
		}
	})

	t.Run("conflicting modes error", func(t *testing.T) {
		if _, err := ResolveAll([]string{dirA, dirA + ":rw"}, ws); err == nil {
			t.Fatal("want error for conflicting modes, got nil")
		}
	})
}

func TestFingerprint(t *testing.T) {
	a := []Resolved{{Path: "/x", ReadOnly: true}, {Path: "/y", ReadOnly: false}}
	b := []Resolved{{Path: "/y", ReadOnly: false}, {Path: "/x", ReadOnly: true}} // reordered

	if Fingerprint(nil) != "" {
		t.Fatal("empty set must fingerprint to empty string")
	}
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatal("fingerprint must be order-independent")
	}
	// Mode flip changes the fingerprint.
	c := []Resolved{{Path: "/x", ReadOnly: false}, {Path: "/y", ReadOnly: false}}
	if Fingerprint(a) == Fingerprint(c) {
		t.Fatal("mode change must change the fingerprint")
	}
	// Added path changes the fingerprint.
	d := append(append([]Resolved{}, a...), Resolved{Path: "/z", ReadOnly: true})
	if Fingerprint(a) == Fingerprint(d) {
		t.Fatal("added path must change the fingerprint")
	}
}
