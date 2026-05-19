package dockerfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareBuildContext_CreatesAndCleansTempDir(t *testing.T) {
	dir, cleanup, err := PrepareBuildContext()
	if err != nil {
		t.Fatalf("PrepareBuildContext: %v", err)
	}
	// The tempdir must exist mid-build.
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("expected dir at %q, got err=%v info=%v", dir, err, info)
	}

	cleanup()

	// After cleanup the dir should be gone — tests of the cleanup
	// contract matter because callers `defer cleanup()` and a leak
	// here means every vibrate invocation accumulates tempdirs.
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("expected dir %q gone after cleanup, stat err=%v", dir, err)
	}
}

func TestPrepareBuildContext_SkipsReadmeAndGitkeep(t *testing.T) {
	dir, cleanup, err := PrepareBuildContext()
	if err != nil {
		t.Fatalf("PrepareBuildContext: %v", err)
	}
	defer cleanup()

	// Walk the tempdir; nothing named README.md or .gitkeep should
	// have been written (those are layout markers, not container
	// payload).
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		name := info.Name()
		if name == "README.md" || name == ".gitkeep" {
			t.Errorf("unexpected %s at %s — should be filtered out", name, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk tempdir: %v", err)
	}
}

func TestPrepareBuildContext_ExtractsExpectedTemplateFiles(t *testing.T) {
	// Pins the concrete file set that the Dockerfile generator depends
	// on. Drift here = `docker build` fails with "COPY shells/zshrc:
	// no such file or directory". This test catches that BEFORE a
	// build attempt.
	dir, cleanup, err := PrepareBuildContext()
	if err != nil {
		t.Fatalf("PrepareBuildContext: %v", err)
	}
	defer cleanup()

	expected := []string{
		filepath.Join("shells", "bashrc"),
		filepath.Join("shells", "zshrc"),
		filepath.Join("shells", "config.fish"),
		filepath.Join("scripts", "welcome.sh"),
	}
	for _, rel := range expected {
		path := filepath.Join(dir, rel)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected %s in build context, got err: %v", rel, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty — embed wiring broken?", rel)
		}
	}
}

func TestPrepareBuildContext_PreservesSubdirStructure(t *testing.T) {
	// When templates/shells/ and templates/scripts/ contain real
	// files (Sprint 3+), they should land at <ctxdir>/shells/X and
	// <ctxdir>/scripts/Y — i.e. with the leading "templates/" prefix
	// stripped. Today those subdirs may be empty, so this test just
	// asserts that whatever IS extracted has no "templates/" prefix
	// in its path.
	dir, cleanup, err := PrepareBuildContext()
	if err != nil {
		t.Fatalf("PrepareBuildContext: %v", err)
	}
	defer cleanup()

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if strings.HasPrefix(rel, "templates"+string(os.PathSeparator)) {
			t.Errorf("path %s still contains 'templates/' prefix — should be stripped", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk tempdir: %v", err)
	}
}
