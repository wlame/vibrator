package extensions

import (
	"strings"
	"testing"
	"testing/fstest"
)

// newFS returns an fstest.MapFS rooted such that extensions/<harness>/<id>.md
// is at the path given. Convenience for tests.
func newFS(files map[string]string) fstest.MapFS {
	m := make(fstest.MapFS, len(files))
	for p, content := range files {
		m[p] = &fstest.MapFile{Data: []byte(content)}
	}
	return m
}

// validEntryYAML returns frontmatter+body for a syntactically valid entry.
// Pass in (name, kind, source) to vary fields.
func validEntryYAML(name, kind, source string) string {
	return "---\n" +
		"name: " + name + "\n" +
		"kind: " + kind + "\n" +
		"source: " + source + "\n" +
		"---\n" +
		"# " + name + "\n" +
		"Body.\n"
}

func TestLoadAll_HappyPath(t *testing.T) {
	fsys := newFS(map[string]string{
		"extensions/claude-code/claude-mem.md": validEntryYAML("claude-mem", "plugin", "https://github.com/thedotmack/claude-mem"),
		"extensions/claude-code/context7.md":   validEntryYAML("context7", "mcp", "https://github.com/upstash/context7"),
		"extensions/codex/github.md":           validEntryYAML("GitHub", "plugin", "https://platform.openai.com/codex/plugins/github"),
	})

	got, err := LoadAll(fsys)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("loaded %d entries, want 3 (got: %v)", len(got), keysOf(got))
	}

	cm, ok := got["claude-code/claude-mem"]
	if !ok {
		t.Fatalf("missing claude-code/claude-mem")
	}
	if cm.Name != "claude-mem" {
		t.Errorf("Name = %q", cm.Name)
	}
	if cm.Kind != KindPlugin {
		t.Errorf("Kind = %q, want plugin", cm.Kind)
	}
	if cm.Harness != "claude-code" {
		t.Errorf("Harness = %q", cm.Harness)
	}
	if cm.ID != "claude-mem" {
		t.Errorf("ID = %q", cm.ID)
	}
	if !strings.Contains(cm.Body, "Body.") {
		t.Errorf("Body missing prose, got: %q", cm.Body)
	}
}

func TestLoadAll_RichFrontmatter(t *testing.T) {
	fsys := newFS(map[string]string{
		"extensions/claude-code/claude-mem.md": `---
name: claude-mem
kind: plugin
default: true
size_mb: 5
deps:
  features: [node, postgres-client]
  extensions: []
prereq: claude-mem-server-beta
install: |
  npx -y claude-mem install --yes
auth:
  env: CLAUDE_MEM_SERVER_BETA_API_KEY
source: https://github.com/thedotmack/claude-mem
---

# claude-mem
Persistent memory across sessions.

## Host setup
Run the docker-compose stack...
`,
	})

	got, err := LoadAll(fsys)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	e := got["claude-code/claude-mem"]
	if !e.Default {
		t.Errorf("Default = false, want true")
	}
	if e.SizeMB != 5 {
		t.Errorf("SizeMB = %d, want 5", e.SizeMB)
	}
	if len(e.Deps.Features) != 2 || e.Deps.Features[0] != "node" {
		t.Errorf("Deps.Features = %v", e.Deps.Features)
	}
	if e.Prereq != "claude-mem-server-beta" {
		t.Errorf("Prereq = %q", e.Prereq)
	}
	if e.Auth == nil || e.Auth.Env != "CLAUDE_MEM_SERVER_BETA_API_KEY" {
		t.Errorf("Auth = %+v", e.Auth)
	}
	if !strings.Contains(e.Install, "npx -y claude-mem install") {
		t.Errorf("Install = %q", e.Install)
	}
}

func TestLoadAll_MissingFrontmatter(t *testing.T) {
	fsys := newFS(map[string]string{
		"extensions/claude-code/no-frontmatter.md": "# No frontmatter\nJust a markdown file.\n",
	})
	_, err := LoadAll(fsys)
	if err == nil {
		t.Fatalf("expected error for missing frontmatter")
	}
	if !strings.Contains(err.Error(), "missing frontmatter") {
		t.Errorf("error should mention frontmatter, got: %v", err)
	}
}

func TestLoadAll_UnclosedFrontmatter(t *testing.T) {
	fsys := newFS(map[string]string{
		"extensions/claude-code/unclosed.md": "---\nname: bad\nkind: plugin\n(no closing ---)",
	})
	_, err := LoadAll(fsys)
	if err == nil {
		t.Fatalf("expected error for unclosed frontmatter")
	}
}

func TestLoadAll_MissingRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string // substring expected in error
	}{
		{
			name: "no name",
			yaml: "---\nkind: plugin\nsource: https://x\n---\nbody\n",
			want: "`name` is required",
		},
		{
			name: "no kind",
			yaml: "---\nname: foo\nsource: https://x\n---\nbody\n",
			want: "kind",
		},
		{
			name: "no source",
			yaml: "---\nname: foo\nkind: plugin\n---\nbody\n",
			want: "source",
		},
		{
			name: "bad kind",
			yaml: "---\nname: foo\nkind: not-a-kind\nsource: https://x\n---\n",
			want: "kind",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fsys := newFS(map[string]string{"extensions/claude-code/x.md": tc.yaml})
			_, err := LoadAll(fsys)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error should mention %q, got: %v", tc.want, err)
			}
		})
	}
}

func TestLoadAll_IDMismatchBetweenFilenameAndFrontmatter(t *testing.T) {
	fsys := newFS(map[string]string{
		"extensions/claude-code/foo.md": `---
id: bar
name: Foo
kind: plugin
source: https://x
---
`,
	})
	_, err := LoadAll(fsys)
	if err == nil {
		t.Fatalf("expected error for id mismatch")
	}
	if !strings.Contains(err.Error(), "disagrees with filename") {
		t.Errorf("error should mention id mismatch, got: %v", err)
	}
}

func TestLoadAll_IDFromFrontmatterAgreesWithFilename(t *testing.T) {
	// Belt-and-braces: declaring id: in frontmatter is allowed iff it
	// matches the filename. This should load cleanly.
	fsys := newFS(map[string]string{
		"extensions/claude-code/foo.md": `---
id: foo
name: Foo
kind: plugin
source: https://x
---
body
`,
	})
	got, err := LoadAll(fsys)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if e, ok := got["claude-code/foo"]; !ok || e.ID != "foo" {
		t.Errorf("entry missing or wrong: %+v", e)
	}
}

func TestLoadAll_IgnoresNonMarkdown(t *testing.T) {
	fsys := newFS(map[string]string{
		"extensions/claude-code/x.md":      validEntryYAML("X", "plugin", "https://x"),
		"extensions/claude-code/.gitkeep":  "",
		"extensions/claude-code/notes.txt": "scratch",
	})
	got, err := LoadAll(fsys)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("want 1 entry, got %d", len(got))
	}
}

func TestLoadAll_IgnoresReadmeMd(t *testing.T) {
	fsys := newFS(map[string]string{
		"extensions/claude-code/README.md": "# Notes about this harness\n",
		"extensions/claude-code/foo.md":    validEntryYAML("Foo", "plugin", "https://x"),
	})
	got, err := LoadAll(fsys)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if _, hasReadme := got["claude-code/README"]; hasReadme {
		t.Errorf("README.md should not be an extension")
	}
	if _, hasFoo := got["claude-code/foo"]; !hasFoo {
		t.Errorf("foo should be loaded")
	}
}

func TestLoadAll_EmptyFS(t *testing.T) {
	fsys := newFS(nil)
	// Degenerate case: an FS with no "extensions/" subdir. The real embedded FS
	// always has the dir, so here we only require that LoadAll returns no
	// entries (and doesn't panic) — not a specific error value.
	got, _ := LoadAll(fsys)
	if len(got) != 0 {
		t.Errorf("empty FS should produce no entries, got %d", len(got))
	}
}

func TestLoadForHarness(t *testing.T) {
	fsys := newFS(map[string]string{
		"extensions/claude-code/aa.md": validEntryYAML("aa", "plugin", "https://a"),
		"extensions/claude-code/zz.md": validEntryYAML("zz", "mcp", "https://z"),
		"extensions/claude-code/mm.md": validEntryYAML("mm", "skill", "https://m"),
		"extensions/codex/github.md":   validEntryYAML("github", "plugin", "https://g"),
	})

	got, err := LoadForHarness(fsys, "claude-code")
	if err != nil {
		t.Fatalf("LoadForHarness: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("want 3 entries, got %d", len(got))
	}
	// Verify sorted by ID.
	wantOrder := []string{"aa", "mm", "zz"}
	for i, e := range got {
		if e.ID != wantOrder[i] {
			t.Errorf("entry[%d].ID = %q, want %q", i, e.ID, wantOrder[i])
		}
	}
}

func TestGet(t *testing.T) {
	fsys := newFS(map[string]string{
		"extensions/claude-code/claude-mem.md": validEntryYAML("claude-mem", "plugin", "https://x"),
	})
	got, err := Get(fsys, "claude-code", "claude-mem")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "claude-mem" {
		t.Errorf("ID = %q", got.ID)
	}

	if _, err := Get(fsys, "claude-code", "missing"); err == nil {
		t.Errorf("expected error for missing entry")
	}
}

func TestHarnesses(t *testing.T) {
	fsys := newFS(map[string]string{
		"extensions/claude-code/a.md": validEntryYAML("a", "plugin", "https://x"),
		"extensions/codex/b.md":       validEntryYAML("b", "plugin", "https://x"),
		"extensions/opencode/c.md":    validEntryYAML("c", "plugin", "https://x"),
	})
	got, err := Harnesses(fsys)
	if err != nil {
		t.Fatalf("Harnesses: %v", err)
	}
	want := []string{"claude-code", "codex", "opencode"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, h := range got {
		if h != want[i] {
			t.Errorf("[%d] = %q, want %q", i, h, want[i])
		}
	}
}

func TestValidateAgainstFeatures(t *testing.T) {
	entries := map[string]*Entry{
		"claude-code/needs-node": {
			Harness: "claude-code",
			ID:      "needs-node",
			Deps:    Deps{Features: []string{"node", "python"}},
		},
		"claude-code/needs-ghost": {
			Harness: "claude-code",
			ID:      "needs-ghost",
			Deps:    Deps{Features: []string{"ghost-feature"}},
		},
	}

	// Known: node + python, not ghost.
	known := func(id string) bool {
		return id == "node" || id == "python"
	}

	err := ValidateAgainstFeatures(entries, known)
	if err == nil {
		t.Fatalf("expected error for ghost-feature dep")
	}
	if !strings.Contains(err.Error(), "ghost-feature") {
		t.Errorf("error should mention ghost-feature, got: %v", err)
	}
}

func TestKind_Valid(t *testing.T) {
	for _, k := range AllKinds {
		if !k.Valid() {
			t.Errorf("AllKinds entry %q should be Valid()", k)
		}
	}
	if Kind("not-a-kind").Valid() {
		t.Errorf("unknown kind should not be Valid()")
	}
}

// keysOf returns the keys of m sorted, for test failure diagnostics.
func keysOf(m map[string]*Entry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// parseEntryForTest parses a single entry from raw markdown bytes and returns
// the Entry, or fails the test if parsing fails.
func parseEntryForTest(t *testing.T, harness, id string, src []byte) *Entry {
	t.Helper()
	e, err := parseEntry(harness, id, src)
	if err != nil {
		t.Fatalf("parseEntry(%q, %q): %v", harness, id, err)
	}
	return e
}

// TestEntry_PinnedModelsParsed pins the optional pinned_models frontmatter
// list: present -> parsed in order; absent -> nil. The wizard's
// keep-or-strip question and the build-time strip step are both driven by
// this field, so its spelling is a contract.
func TestEntry_PinnedModelsParsed(t *testing.T) {
	src := []byte(`---
name: Pinned thing
kind: subagent
pinned_models: ["gpt-5.4", "gpt-5.4-mini"]
install: |
  true
source: https://example.com/pinned
---
Body.
`)
	e := parseEntryForTest(t, "codex", "pinned-thing.md", src)
	if got := e.PinnedModels; len(got) != 2 || got[0] != "gpt-5.4" || got[1] != "gpt-5.4-mini" {
		t.Errorf("PinnedModels = %v, want [gpt-5.4 gpt-5.4-mini]", got)
	}

	plain := []byte("---\nname: Plain\nkind: mcp\ninstall: |\n  true\nsource: https://example.com/plain\n---\nBody.\n")
	p := parseEntryForTest(t, "codex", "plain.md", plain)
	if p.PinnedModels != nil {
		t.Errorf("PinnedModels = %v, want nil when absent", p.PinnedModels)
	}
}
