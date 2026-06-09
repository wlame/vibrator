package workspace

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFingerprint_StableAcrossOrder(t *testing.T) {
	a := Spec{
		Harness:    "claude-code",
		Shell:      "zsh",
		Features:   []string{"python", "go", "node"},
		Extensions: []string{"claude-mem", "context7"},
	}
	b := Spec{
		Harness:    "claude-code",
		Shell:      "zsh",
		Features:   []string{"node", "python", "go"}, // reordered
		Extensions: []string{"context7", "claude-mem"},
	}
	if Fingerprint(a) != Fingerprint(b) {
		t.Errorf("reordered features/extensions produced different fingerprints: %s vs %s",
			Fingerprint(a), Fingerprint(b))
	}
}

func TestFingerprint_DifferentInputsDiffer(t *testing.T) {
	base := Spec{Harness: "claude-code", Shell: "zsh", Features: []string{"python"}}

	cases := []struct {
		name   string
		mutate func(Spec) Spec
	}{
		{"harness change", func(s Spec) Spec { s.Harness = "codex"; return s }},
		{"shell change", func(s Spec) Spec { s.Shell = "fish"; return s }},
		{"feature added", func(s Spec) Spec { s.Features = append(s.Features, "go"); return s }},
		{"feature removed", func(s Spec) Spec { s.Features = nil; return s }},
		{"extensions added", func(s Spec) Spec { s.Extensions = []string{"claude-mem"}; return s }},
	}
	baseFp := Fingerprint(base)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Fingerprint(tc.mutate(base)); got == baseFp {
				t.Errorf("expected different fingerprint, got identical %s", got)
			}
		})
	}
}

func TestFingerprint_CaseInsensitiveEnums(t *testing.T) {
	a := Spec{Harness: "claude-code", Shell: "zsh"}
	b := Spec{Harness: "Claude-Code", Shell: "ZSH"}
	if Fingerprint(a) != Fingerprint(b) {
		t.Errorf("case differences in enums should not affect fingerprint: %s vs %s",
			Fingerprint(a), Fingerprint(b))
	}
}

func TestFingerprint_ProfileIsNotPartOfHash(t *testing.T) {
	// Profile is just a label — the resolved features list is what matters.
	a := Spec{Harness: "claude-code", Profile: "backend", Features: []string{"python", "go"}}
	b := Spec{Harness: "claude-code", Profile: "full", Features: []string{"python", "go"}}
	if Fingerprint(a) != Fingerprint(b) {
		t.Errorf("profile label affected fingerprint: %s vs %s", Fingerprint(a), Fingerprint(b))
	}
}

func TestFingerprint_EmptySpec(t *testing.T) {
	got := Fingerprint(Spec{})
	if got != "00000000" {
		t.Errorf("want sentinel 00000000 for empty spec, got %s", got)
	}
}

func TestFingerprint_Length(t *testing.T) {
	got := Fingerprint(Spec{Harness: "claude-code", Shell: "zsh", Features: []string{"python"}})
	if len(got) != 8 {
		t.Errorf("fingerprint length = %d, want 8 (%s)", len(got), got)
	}
}

func TestImageName(t *testing.T) {
	cases := []struct {
		name string
		spec Spec
		fp   string
		want string
	}{
		{
			name: "happy path",
			spec: Spec{Harness: "claude-code", Profile: "backend"},
			fp:   "a1b2c3d4",
			want: "vb-claude-code-backend-a1b2c3d4:latest",
		},
		{
			name: "missing harness falls back to 'unknown'",
			spec: Spec{Profile: "minimal"},
			fp:   "00000000",
			want: "vb-unknown-minimal-00000000:latest",
		},
		{
			name: "missing profile falls back to 'default'",
			spec: Spec{Harness: "codex"},
			fp:   "11111111",
			want: "vb-codex-default-11111111:latest",
		},
		{
			name: "uppercase harness gets lowercased",
			spec: Spec{Harness: "Claude-Code", Profile: "Full"},
			fp:   "ffffffff",
			want: "vb-claude-code-full-ffffffff:latest",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ImageName(tc.spec, tc.fp); got != tc.want {
				t.Errorf("ImageName = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestImageName_IncludesUsernameWhenSet(t *testing.T) {
	// F4: image tag must carry the username so two users on the same
	// host (or two `--username` overrides) don't trample one another's
	// images. The image's USER stage bakes in the host UID, so sharing
	// is incorrect by design.
	spec := Spec{Harness: "claude-code", Profile: "backend", Username: "alice"}
	got := ImageName(spec, "a1b2c3d4")
	want := "vb-claude-code-backend-alice-a1b2c3d4:latest"
	if got != want {
		t.Errorf("ImageName = %q, want %q", got, want)
	}
}

func TestFingerprint_DifferentUsersDiffer(t *testing.T) {
	// F4 enforcement at the fingerprint level (belt + suspenders with
	// the tag segment): two specs identical except for Username MUST
	// produce different fingerprints — otherwise image caches would
	// collide on disk even if tags differ.
	a := Fingerprint(Spec{Harness: "claude-code", Shell: "zsh", Username: "alice"})
	b := Fingerprint(Spec{Harness: "claude-code", Shell: "zsh", Username: "bob"})
	if a == b {
		t.Errorf("alice and bob produced identical fingerprint %s — Username must affect canonical", a)
	}
}

func TestContainerName_StablePerWorkspace(t *testing.T) {
	got1 := ContainerName("/home/wlame/dev/vibrator", "abc12345")
	got2 := ContainerName("/home/wlame/dev/vibrator", "abc12345")
	if got1 != got2 {
		t.Errorf("not stable: %s vs %s", got1, got2)
	}
	if !strings.HasPrefix(got1, "vb-vibrator-") {
		t.Errorf("name should begin vb-vibrator-, got %s", got1)
	}
	if !strings.HasSuffix(got1, "-abc12345") {
		t.Errorf("name should end -abc12345, got %s", got1)
	}
}

func TestContainerName_DisambiguatesSameBasename(t *testing.T) {
	a := ContainerName("/home/wlame/work/foo", "abc12345")
	b := ContainerName("/home/wlame/play/foo", "abc12345")
	if a == b {
		t.Errorf("workspaces with same basename collided: %s", a)
	}
}

func TestContainerName_SanitizesSpecialChars(t *testing.T) {
	got := ContainerName("/tmp/My Project (test)", "abc12345")
	// Spaces and parens must not appear in container names.
	if strings.ContainsAny(got, " ()") {
		t.Errorf("special chars leaked into name: %s", got)
	}
}

func TestContainerName_NeverEmptyBasename(t *testing.T) {
	// edge case: filepath.Base("/") = "/", which sanitizes to "" — should
	// fall back to "ws" rather than producing "vb--<hash>-<fp>".
	got := ContainerName("/", "abc12345")
	if !strings.Contains(got, "vb-ws-") {
		t.Errorf("root path should produce vb-ws-... fallback, got %s", got)
	}
}

func TestHostname_PrefixedAndSanitized(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		// Common case — plain workspace name passes through lowercased.
		{"/Users/wlame/dev/vibrator", "vibrate-vibrator"},
		{"/home/alice/projects/foo-bar", "vibrate-foo-bar"},

		// Underscores in the basename are NOT RFC 1123-legal in hostnames
		// (even though some resolvers accept them) — replaced with '-'.
		{"/home/u/my_project", "vibrate-my-project"},

		// Spaces and parens get sanitized to '-'.
		{"/tmp/My Project", "vibrate-my-project"},
		{"/tmp/repo (work)", "vibrate-repo--work"},

		// Numeric-only basename is legal (RFC 1123 allows digits).
		{"/tmp/2026", "vibrate-2026"},

		// Empty/root basename falls back to "workspace".
		{"/", "vibrate-workspace"},
		{"", "vibrate-workspace"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := Hostname(tc.path)
			if got != tc.want {
				t.Errorf("Hostname(%q) = %q, want %q", tc.path, got, tc.want)
			}
			if len(got) > 63 {
				t.Errorf("Hostname(%q) = %q is %d chars, must be ≤ 63 (RFC 1123 label)",
					tc.path, got, len(got))
			}
		})
	}
}

func TestHostname_TruncationStripsTrailingHyphen(t *testing.T) {
	// 80 hyphens — sanitization preserves them as valid follow-chars,
	// but a 63-char truncation could leave a trailing one. Strip it.
	long := "/tmp/" + strings.Repeat("a-", 40)
	got := Hostname(long)
	if strings.HasSuffix(got, "-") {
		t.Errorf("Hostname result ends with '-': %q", got)
	}
}

func TestSanitizeTagSegment(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"claude-code", "claude-code"},
		{"Claude-Code", "claude-code"},
		{"with spaces", "with-spaces"},
		{"weird/chars", "weird-chars"},
		{"-leading-dash", "leading-dash"},
		{".leading.dot", "leading.dot"},
	}
	for _, tc := range cases {
		if got := sanitizeTagSegment(tc.in); got != tc.want {
			t.Errorf("sanitizeTagSegment(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestImageName_FormatExample(t *testing.T) {
	// Concrete end-to-end example matching what the README documents.
	spec := Spec{
		Harness:    "claude-code",
		Profile:    "backend",
		Shell:      "zsh",
		Features:   []string{"python", "go", "node"},
		Extensions: []string{"claude-mem", "context7"},
	}
	fp := Fingerprint(spec)
	img := ImageName(spec, fp)
	if !strings.HasPrefix(img, "vb-claude-code-backend-") {
		t.Errorf("expected vb-claude-code-backend-* prefix, got %s", img)
	}
	if !strings.HasSuffix(img, ":latest") {
		t.Errorf("expected :latest suffix, got %s", img)
	}
}

func TestContainerName_AbsolutePathResolution(t *testing.T) {
	// Pass a path that needs filepath.Abs to resolve; result should be
	// indistinguishable from passing the absolute equivalent.
	abs, _ := filepath.Abs("./")
	a := ContainerName("./", "abc12345")
	b := ContainerName(abs, "abc12345")
	if a != b {
		t.Errorf("relative and absolute path produced different names: %s vs %s", a, b)
	}
}
