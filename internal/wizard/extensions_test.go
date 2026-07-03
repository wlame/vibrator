package wizard

import (
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/extensions"
)

func TestFormatEntryLabel_PlainEntry(t *testing.T) {
	e := &extensions.Entry{ID: "x", Name: "Display"}
	got := formatEntryLabel(e)
	want := "Display — x"
	if got != want {
		t.Errorf("formatEntryLabel = %q, want %q", got, want)
	}
}

func TestEntryNote(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "first paragraph after heading",
			body: "# Title\n\nFirst paragraph line one.\nLine two.\n\nSecond paragraph.\n",
			want: "First paragraph line one. Line two.",
		},
		{
			name: "skips multiple leading headings and blanks",
			body: "\n\n# H1\n\n## H2\n\nThe prose.\n",
			want: "The prose.",
		},
		{
			name: "flattens inline markdown links, keeps source out",
			body: "# ECC\n\n[ECC](https://github.com/affaan-m/ECC) is a bundle.\n",
			want: "ECC is a bundle.",
		},
		{
			name: "empty when body has only a heading",
			body: "# Just a title\n",
			want: "",
		},
		{
			name: "empty body",
			body: "",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := entryNote(&extensions.Entry{Body: tc.body})
			if got != tc.want {
				t.Errorf("entryNote() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatEntryLabel_WithDescription(t *testing.T) {
	// When Description is set, the label uses it as the prefix detail
	// and moves the id to a parenthetical trailer. Pin both halves.
	e := &extensions.Entry{
		ID:          "x",
		Name:        "Foo",
		Description: "Does the thing",
	}
	got := formatEntryLabel(e)
	if !strings.Contains(got, "Foo — Does the thing") {
		t.Errorf("description not in prefix: %q", got)
	}
	if !strings.Contains(got, "(x)") {
		t.Errorf("id should appear as parenthetical trailer: %q", got)
	}
	// Bare " — x" should NOT appear when description path is taken —
	// id-as-anchor is for the fallback path only.
	if strings.Contains(got, "Foo — x") {
		t.Errorf("legacy 'name — id' should not appear when description set: %q", got)
	}
}

func TestFormatEntryLabel_NoDescriptionFallsBack(t *testing.T) {
	e := &extensions.Entry{ID: "x", Name: "Foo"} // no Description
	got := formatEntryLabel(e)
	if got != "Foo — x" {
		t.Errorf("fallback label = %q, want %q", got, "Foo — x")
	}
}

func TestFormatEntryLabel_WithCategory(t *testing.T) {
	e := &extensions.Entry{
		ID:       "x",
		Name:     "Display",
		Category: extensions.CategoryDatabases,
	}
	got := formatEntryLabel(e)
	if !strings.Contains(got, "Databases") {
		t.Errorf("formatEntryLabel = %q, expected to contain category label", got)
	}
}

func TestFormatEntryLabel_LocalOnlyBadge(t *testing.T) {
	e := &extensions.Entry{ID: "x", Name: "Local",
		RuntimeNeeds: extensions.RuntimeNeeds{LocalOnly: true},
	}
	if !strings.Contains(formatEntryLabel(e), "[local]") {
		t.Errorf("missing [local] badge for LocalOnly entry: %q", formatEntryLabel(e))
	}
}

func TestFormatEntryLabel_TokenBadgeFromAuth(t *testing.T) {
	e := &extensions.Entry{
		ID:   "x",
		Name: "GitHub",
		Auth: &extensions.AuthSpec{Env: "GITHUB_PERSONAL_ACCESS_TOKEN"},
	}
	got := formatEntryLabel(e)
	if !strings.Contains(got, "[token: $GITHUB_PERSONAL_ACCESS_TOKEN]") {
		t.Errorf("missing token badge: %q", got)
	}
}

func TestFormatEntryLabel_ThirdPartyBadgeWhenAuthMissing(t *testing.T) {
	// When the entry declares a third-party dep but no auth env, we
	// still want a badge — useful for "user uses OAuth on first run".
	e := &extensions.Entry{
		ID:   "x",
		Name: "Notion",
		RuntimeNeeds: extensions.RuntimeNeeds{
			ThirdPartyAPI: "Notion",
		},
	}
	got := formatEntryLabel(e)
	if !strings.Contains(got, "[3rd-party: Notion]") {
		t.Errorf("missing 3rd-party badge: %q", got)
	}
}

func TestFormatEntryLabel_AuthBadgeWinsOverThirdParty(t *testing.T) {
	// When both auth.env and runtime_needs.third_party_api are set,
	// the token badge is preferred (more actionable for the user).
	e := &extensions.Entry{
		ID:   "x",
		Name: "GitHub",
		Auth: &extensions.AuthSpec{Env: "GITHUB_PERSONAL_ACCESS_TOKEN"},
		RuntimeNeeds: extensions.RuntimeNeeds{
			ThirdPartyAPI: "GitHub",
		},
	}
	got := formatEntryLabel(e)
	if !strings.Contains(got, "[token:") {
		t.Errorf("expected token badge to win: %q", got)
	}
	if strings.Contains(got, "[3rd-party:") {
		t.Errorf("expected 3rd-party badge to be suppressed when auth set: %q", got)
	}
}

func TestFormatEntryLabel_SelfHostedBadge(t *testing.T) {
	e := &extensions.Entry{
		ID:   "claude-mem",
		Name: "claude-mem",
		RuntimeNeeds: extensions.RuntimeNeeds{
			SelfHosted: "claude-mem-stack",
		},
	}
	got := formatEntryLabel(e)
	if !strings.Contains(got, "[host: claude-mem-stack]") {
		t.Errorf("missing host badge: %q", got)
	}
}

func TestFormatEntryLabel_AllBadgesAtOnce(t *testing.T) {
	// Pathological case — pin the badge ordering so changes to it
	// surface here rather than in user-facing wizard renderings.
	e := &extensions.Entry{
		ID:       "kitchen-sink",
		Name:     "Kitchen Sink",
		Category: extensions.CategoryAIIntegration,
		Auth:     &extensions.AuthSpec{Env: "MY_TOKEN"},
		RuntimeNeeds: extensions.RuntimeNeeds{
			LocalOnly:   true, // weird combo but allowed
			SelfHosted:  "my-server",
			OutboundNet: true,
		},
	}
	got := formatEntryLabel(e)
	// Order: local → host → token → net (fixed in runtimeBadges).
	wantOrder := []string{"[local]", "[host:", "[token:", "[net]"}
	pos := 0
	for _, want := range wantOrder {
		idx := strings.Index(got[pos:], want)
		if idx < 0 {
			t.Errorf("badge order broken: missing %q after position %d in %q", want, pos, got)
			continue
		}
		pos += idx + len(want)
	}
}

// The previous tests covered `optionsForKind` (the huh-options
// builder). That function was retired with the huh per-kind groups;
// the same sort + filter contract lives in `pickerEntriesFor` now,
// and the equivalent tests landed in extensions_picker_test.go.

// TestFormatEntryLabel_PinnedModelsBadge pins the picker disclosure: an
// entry that declares pinned_models must show them inline so the user
// knows a hardcoded model ships before selecting it.
func TestFormatEntryLabel_PinnedModelsBadge(t *testing.T) {
	e := &extensions.Entry{
		ID:           "agent-x",
		Name:         "Agent X",
		Description:  "Does agent things",
		PinnedModels: []string{"gpt-5.4"},
	}
	got := formatEntryLabel(e)
	if !strings.Contains(got, "[pins: gpt-5.4]") {
		t.Errorf("label missing pinned-models badge: %q", got)
	}

	plain := &extensions.Entry{ID: "y", Name: "Y"}
	if strings.Contains(formatEntryLabel(plain), "[pins:") {
		t.Errorf("badge must not appear without pinned_models: %q", formatEntryLabel(plain))
	}
}

// TestPinnedModelsIn pins the selection->question predicate: union of the
// selected entries' pinned models, deduplicated, first-seen order; empty
// when no selected entry pins anything (no wizard question).
func TestPinnedModelsIn(t *testing.T) {
	entries := []*extensions.Entry{
		{ID: "a", PinnedModels: []string{"gpt-5.4"}},
		{ID: "b", PinnedModels: []string{"gpt-5.4", "o5"}},
		{ID: "c"},
	}
	if got := pinnedModelsIn(entries, []string{"a", "b", "c"}); len(got) != 2 || got[0] != "gpt-5.4" || got[1] != "o5" {
		t.Errorf("pinnedModelsIn = %v, want [gpt-5.4 o5]", got)
	}
	if got := pinnedModelsIn(entries, []string{"c"}); got != nil {
		t.Errorf("pinnedModelsIn = %v, want nil for pin-free selection", got)
	}
	if got := pinnedModelsIn(entries, nil); got != nil {
		t.Errorf("pinnedModelsIn = %v, want nil for empty selection", got)
	}
}
