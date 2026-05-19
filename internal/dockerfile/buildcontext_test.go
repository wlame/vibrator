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

// Regression: the entrypoint script must not silently die under
// `set -e` when VIBRATOR_VERBOSE is unset (the common case). The
// failure mode is a `log()` function whose last statement is
// `[ -n "$VIBRATOR_VERBOSE" ] && printf …` — the function returns
// non-zero when VERBOSE is empty, set -e fires, container exits 1
// with no output. Always end such functions with `return 0`.
//
// We can't run the entrypoint here (it needs a Linux container), but
// we CAN grep the source for the bad pattern and force a `return 0`
// to be present in every conditional-output function.
func TestEntrypointScript_LogFunctionsAlwaysReturnZero(t *testing.T) {
	dir, cleanup, err := PrepareBuildContext()
	if err != nil {
		t.Fatalf("PrepareBuildContext: %v", err)
	}
	defer cleanup()

	body, err := os.ReadFile(filepath.Join(dir, "scripts", "entrypoint.sh"))
	if err != nil {
		t.Fatalf("read entrypoint.sh: %v", err)
	}
	s := string(body)

	// The script must declare `set -e` (without it, function-return
	// status leakage is harmless — but losing set -e weakens error
	// detection elsewhere). If we ever drop set -e, this test is the
	// place to revisit.
	if !strings.Contains(s, "set -e") {
		t.Fatal("entrypoint.sh no longer declares `set -e` — revisit this test's premise")
	}

	// Every function that gates output on a verbose flag must
	// explicitly `return 0`. Grep for the dangerous shape and assert
	// each such function body contains a return-0.
	// We use a heuristic: any function body containing `VIBRATOR_VERBOSE`
	// in a conditional MUST also contain `return 0`.
	if strings.Contains(s, "VIBRATOR_VERBOSE") && !strings.Contains(s, "return 0") {
		t.Errorf("entrypoint.sh uses VIBRATOR_VERBOSE gating but has no `return 0` — " +
			"this is the classic 'set -e aborts on log() returning non-zero' bug. " +
			"Add `return 0` to the end of any function that does " +
			"`[ -n \"$VIBRATOR_VERBOSE\" ] && printf …` so it doesn't kill the script " +
			"under set -e when verbose is off.")
	}
}

// Regression for Sprint 5 entrypoint additions (C9, C10, C11). Each is
// load-bearing for a specific UX outcome:
//
//   - C9 chmod: host-mounted plugin hooks lose +x and fail silently;
//     `find ... -exec chmod +x` is the self-healing fix.
//   - C10 baked-plugins re-enable: a wholesale copy from host
//     settings.json wipes the image's baked plugins; we re-add them.
//   - C11 workspace parent mkdir: docker creates parents but as root —
//     pre-creating makes them user-writable so `cd ..; git clone` works.
//
// Loose grep matches so refactors don't have to update the test.
func TestEntrypointScript_HasSprint5Additions(t *testing.T) {
	dir, cleanup, err := PrepareBuildContext()
	if err != nil {
		t.Fatalf("PrepareBuildContext: %v", err)
	}
	defer cleanup()

	body, err := os.ReadFile(filepath.Join(dir, "scripts", "entrypoint.sh"))
	if err != nil {
		t.Fatalf("read entrypoint.sh: %v", err)
	}
	s := string(body)

	cases := []struct {
		name, needle string
	}{
		{"C5 GPG socket symlink", "/gpg-agent-extra"},
		{"C5 gpgconf agent-socket query", "agent-socket"},
		{"C7 MCP pruning helper", "_vb_feature_on"},
		{"C7 MCP pruning del", "del(.mcpServers["},
		{"C8 claude-mem env gate", "CLAUDE_MEM_RUNTIME"},
		{"C8 claude-mem healthz probe", "/healthz"},
		{"C8 claude-mem auth probe", "/v1/events"},
		{"C9 plugin re-perm", "chmod +x"},
		{"C9 plugin re-perm path", ".claude/plugins"},
		{"C10 baked plugins", "installed_plugins.json"},
		{"C10 enabledPlugins merge", "enabledPlugins"},
		{"C11 workspace parent mkdir", "WORKSPACE_PARENT"},
	}
	for _, c := range cases {
		if !strings.Contains(s, c.needle) {
			t.Errorf("%s: entrypoint.sh missing %q", c.name, c.needle)
		}
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
		filepath.Join("scripts", "entrypoint.sh"),
		filepath.Join("scripts", "claude-exec.sh"),
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
