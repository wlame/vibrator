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

func TestOptionsForKind_SortsByCategoryThenID(t *testing.T) {
	entries := map[string]*extensions.Entry{
		"h/z-alpha": {ID: "z-alpha", Name: "Z Alpha", Harness: "h", Kind: extensions.KindMCP, Category: extensions.CategoryCodeIntel},
		"h/a-beta":  {ID: "a-beta", Name: "A Beta", Harness: "h", Kind: extensions.KindMCP, Category: extensions.CategoryDatabases},
		"h/b-gamma": {ID: "b-gamma", Name: "B Gamma", Harness: "h", Kind: extensions.KindMCP, Category: extensions.CategoryCodeIntel},
	}
	opts := optionsForKind(entries, "h", extensions.KindMCP)
	// Sort key is the raw category string; alphabetical:
	//   code-intelligence < databases.
	// Within code-intelligence: b-gamma < z-alpha (by ID).
	// Expected order: b-gamma, z-alpha, a-beta.
	wantIDs := []string{"b-gamma", "z-alpha", "a-beta"}
	if len(opts) != len(wantIDs) {
		t.Fatalf("got %d options, want %d", len(opts), len(wantIDs))
	}
	for i, opt := range opts {
		if opt.Value != wantIDs[i] {
			t.Errorf("opts[%d].Value = %q, want %q", i, opt.Value, wantIDs[i])
		}
	}
}

func TestOptionsForKind_FiltersByHarnessAndKind(t *testing.T) {
	entries := map[string]*extensions.Entry{
		"h1/a": {ID: "a", Name: "A", Harness: "h1", Kind: extensions.KindMCP},
		"h2/b": {ID: "b", Name: "B", Harness: "h2", Kind: extensions.KindMCP},
		"h1/c": {ID: "c", Name: "C", Harness: "h1", Kind: extensions.KindPlugin},
	}
	opts := optionsForKind(entries, "h1", extensions.KindMCP)
	if len(opts) != 1 {
		t.Fatalf("got %d, want 1 (only h1/a is MCP for h1)", len(opts))
	}
	if opts[0].Value != "a" {
		t.Errorf("got %q, want a", opts[0].Value)
	}
}
