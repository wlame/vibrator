package dockerfile

import (
	"bytes"
	"strings"
	"testing"
	"testing/fstest"
)

func TestGeneratorHash_DeterministicAndVersionInsensitive(t *testing.T) {
	a := minimalSpec(t)
	a.VibratorVersion = "1.0.0"
	b := minimalSpec(t)
	b.VibratorVersion = "2.9.9-dev"

	ha, err := GeneratorHash(a)
	if err != nil {
		t.Fatalf("GeneratorHash: %v", err)
	}
	hb, err := GeneratorHash(b)
	if err != nil {
		t.Fatalf("GeneratorHash: %v", err)
	}
	if ha != hb {
		t.Errorf("hash must ignore VibratorVersion: %s != %s", ha, hb)
	}
	if len(ha) != 12 {
		t.Errorf("hash length = %d, want 12", len(ha))
	}
	if hc, _ := GeneratorHash(a); hc != ha {
		t.Errorf("hash not deterministic: %s != %s", hc, ha)
	}
}

func TestGeneratorHash_SensitiveToSpec(t *testing.T) {
	a := minimalSpec(t)
	b := minimalSpec(t)
	b.Shell = "bash" // minimalSpec uses zsh
	ha, _ := GeneratorHash(a)
	hb, err := GeneratorHash(b)
	if err != nil {
		t.Fatalf("GeneratorHash: %v", err)
	}
	if ha == hb {
		t.Error("hash must change when the generated Dockerfile changes")
	}
}

// GeneratorHash must fingerprint exactly the files extractTemplatesTo ships
// into the built image — not the whole templates/ tree, which also holds
// contributor-only files like README.md and .gitkeep (see buildcontext.go).
// Mutating the real embedded FS from a test isn't feasible, so this drives
// the actual production walk (hashTemplateFiles) against an in-memory
// fs.FS: it proves editing README.md's content never changes what gets
// hashed, while a real shipped file's content always does. Without this
// shared skipTemplateFile predicate, editing templates/README.md would
// flip GeneratorHash's output despite having zero effect on the image —
// a permanent false "image is stale" warning.
func TestHashTemplateFiles_SkipsNonShippedFiles(t *testing.T) {
	build := func(readme string) string {
		fsys := fstest.MapFS{
			"templates/README.md":             {Data: []byte(readme)},
			"templates/scripts/.gitkeep":      {Data: []byte("")},
			"templates/scripts/entrypoint.sh": {Data: []byte("#!/bin/sh\necho hi\n")},
		}
		var buf bytes.Buffer
		if err := hashTemplateFiles(&buf, fsys, "templates"); err != nil {
			t.Fatalf("hashTemplateFiles: %v", err)
		}
		return buf.String()
	}

	a := build("original docs")
	b := build("totally different docs")

	if a != b {
		t.Error("changing templates/README.md content changed the hash input — " +
			"README.md must be excluded entirely, matching extractTemplatesTo's build-context filtering")
	}
	if !strings.Contains(a, "echo hi") {
		t.Error("hash input is missing entrypoint.sh's contents — a real shipped file was dropped")
	}
	if strings.Contains(a, "docs") {
		t.Error("hash input contains README.md content — it must be skipped entirely, not just have edits ignored")
	}
}

// skipTemplateFile is the shared predicate consulted by BOTH
// extractTemplatesTo (buildcontext.go) and hashTemplateFiles above — this
// pins its exact behavior for the two basenames that must never reach the
// image, and confirms a real shipped file isn't accidentally caught by the
// same rule.
func TestSkipTemplateFile(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"README.md", true},
		{".gitkeep", true},
		{"entrypoint.sh", false},
		{"zshrc", false},
	}
	for _, c := range cases {
		if got := skipTemplateFile(c.name); got != c.want {
			t.Errorf("skipTemplateFile(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
