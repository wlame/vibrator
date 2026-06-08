package wizard

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wlame/vibrator/internal/extensions"
)

// makeEntries builds a tiny but realistic catalogue for picker tests.
// 4 entries across 2 kinds + 1 different-harness entry that must be
// excluded by the harness filter.
func makeEntries() map[string]*extensions.Entry {
	return map[string]*extensions.Entry{
		"claude-code/plug-a": {
			Harness: "claude-code", ID: "plug-a", Kind: extensions.KindPlugin,
			Name: "Plug A", Source: "x", Category: extensions.CategoryCodeIntel,
		},
		"claude-code/plug-b": {
			Harness: "claude-code", ID: "plug-b", Kind: extensions.KindPlugin,
			Name: "Plug B", Source: "x", Category: extensions.CategoryDatabases,
		},
		"claude-code/mcp-c": {
			Harness: "claude-code", ID: "mcp-c", Kind: extensions.KindMCP,
			Name: "MCP C", Source: "x", Default: true,
		},
		"claude-code/mcp-d": {
			Harness: "claude-code", ID: "mcp-d", Kind: extensions.KindMCP,
			Name: "MCP D", Source: "x",
		},
		// Different harness — must NOT appear in claude-code's tabs.
		"codex/wrong-harness": {
			Harness: "codex", ID: "wrong-harness", Kind: extensions.KindPlugin,
			Name: "Should not show", Source: "x",
		},
	}
}

func newTestModel() pickerModel {
	return newPickerModel(pickerInput{
		HarnessID: "claude-code",
		Entries:   makeEntries(),
	})
}

// ── Construction / filtering ────────────────────────────────────────

func TestNewPickerModel_BuildsTabsPerKind(t *testing.T) {
	m := newTestModel()
	if len(m.tabs) != 2 {
		t.Fatalf("got %d tabs, want 2 (plugin + mcp)", len(m.tabs))
	}
	// Order should follow extensions.AllKinds — plugin first, mcp second.
	if m.tabs[0].kind != extensions.KindPlugin {
		t.Errorf("tab 0 kind = %q, want plugin", m.tabs[0].kind)
	}
	if m.tabs[1].kind != extensions.KindMCP {
		t.Errorf("tab 1 kind = %q, want mcp", m.tabs[1].kind)
	}
}

func TestNewPickerModel_FiltersByHarness(t *testing.T) {
	m := newTestModel()
	for _, tab := range m.tabs {
		for _, e := range tab.entries {
			if e.Harness != "claude-code" {
				t.Errorf("entry from harness %q leaked through filter: %s", e.Harness, e.ID)
			}
		}
	}
}

func TestNewPickerModel_SeedsDefaultsAsSelected(t *testing.T) {
	m := newTestModel()
	if !m.selected["mcp-c"] {
		t.Errorf("mcp-c has default:true but isn't pre-selected: %v", m.selected)
	}
	if m.selected["mcp-d"] {
		t.Errorf("mcp-d should NOT be pre-selected (default:false)")
	}
}

func TestNewPickerModel_SeedsInitialSelection(t *testing.T) {
	m := newPickerModel(pickerInput{
		HarnessID:        "claude-code",
		Entries:          makeEntries(),
		InitialSelection: []string{"plug-a"},
	})
	if !m.selected["plug-a"] {
		t.Errorf("plug-a from InitialSelection not seeded: %v", m.selected)
	}
}

func TestNewPickerModel_SkipsEmptyKinds(t *testing.T) {
	// Catalogue with ONLY plugin entries — Skill/MCP/Subagent/Tool tabs
	// should be omitted, not rendered as empty.
	entries := map[string]*extensions.Entry{
		"h/p": {Harness: "h", ID: "p", Kind: extensions.KindPlugin, Name: "P", Source: "x"},
	}
	m := newPickerModel(pickerInput{HarnessID: "h", Entries: entries})
	if len(m.tabs) != 1 {
		t.Fatalf("expected 1 tab (just plugins), got %d", len(m.tabs))
	}
	if m.tabs[0].kind != extensions.KindPlugin {
		t.Errorf("only tab should be plugin, got %q", m.tabs[0].kind)
	}
}

// ── Key handling ────────────────────────────────────────────────────

// sendKey is a tiny helper that drives handleKey with a specific key.
// Constructing tea.KeyMsg by hand avoids spinning up a real tea.Program.
func sendKey(m pickerModel, runes string) (pickerModel, tea.Cmd) {
	if len(runes) == 1 && runes != "" {
		return m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(runes)})
	}
	// Map special keys.
	switch runes {
	case "left":
		return m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	case "right":
		return m.handleKey(tea.KeyMsg{Type: tea.KeyRight})
	case "up":
		return m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	case "down":
		return m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	case "tab":
		return m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	case "shift+tab":
		return m.handleKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	case "enter":
		return m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	case "esc":
		return m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	case "space":
		return m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	case "ctrl+c":
		return m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	}
	return m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(runes)})
}

func TestPickerKeys_LeftRightCycleTabs(t *testing.T) {
	m := newTestModel()
	if m.activeTab != 0 {
		t.Fatalf("initial activeTab = %d, want 0", m.activeTab)
	}
	m, _ = sendKey(m, "right")
	if m.activeTab != 1 {
		t.Errorf("after 'right', activeTab = %d, want 1", m.activeTab)
	}
	m, _ = sendKey(m, "right") // wraps
	if m.activeTab != 0 {
		t.Errorf("after second 'right', wrap-around expected, got %d", m.activeTab)
	}
	m, _ = sendKey(m, "left") // back-wrap
	if m.activeTab != 1 {
		t.Errorf("after 'left' from 0, wrap expected to 1, got %d", m.activeTab)
	}
}

func TestPickerKeys_TabShiftTabSameAsArrows(t *testing.T) {
	m := newTestModel()
	m, _ = sendKey(m, "tab")
	if m.activeTab != 1 {
		t.Errorf("tab should advance tabs, got %d", m.activeTab)
	}
	m, _ = sendKey(m, "shift+tab")
	if m.activeTab != 0 {
		t.Errorf("shift+tab should reverse, got %d", m.activeTab)
	}
}

func TestPickerKeys_UpDownMovesCursor(t *testing.T) {
	m := newTestModel()
	// Initial cursor on tab 0 (plugins, 2 entries) is 0.
	if m.cursors[0] != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.cursors[0])
	}
	m, _ = sendKey(m, "down")
	if m.cursors[0] != 1 {
		t.Errorf("after 'down', cursor = %d, want 1", m.cursors[0])
	}
	// Down at bottom is a no-op (doesn't go past the last row).
	m, _ = sendKey(m, "down")
	if m.cursors[0] != 1 {
		t.Errorf("'down' at bottom should clamp to 1, got %d", m.cursors[0])
	}
	m, _ = sendKey(m, "up")
	if m.cursors[0] != 0 {
		t.Errorf("after 'up', cursor = %d, want 0", m.cursors[0])
	}
	// Up at top is a no-op.
	m, _ = sendKey(m, "up")
	if m.cursors[0] != 0 {
		t.Errorf("'up' at top should clamp to 0, got %d", m.cursors[0])
	}
}

func TestPickerKeys_GAndShiftGJumpToEdges(t *testing.T) {
	m := newTestModel()
	m, _ = sendKey(m, "G") // jump to end
	if m.cursors[0] != 1 {
		t.Errorf("G should jump to end (row 1), got %d", m.cursors[0])
	}
	m, _ = sendKey(m, "g") // jump to top
	if m.cursors[0] != 0 {
		t.Errorf("g should jump to top, got %d", m.cursors[0])
	}
}

func TestPickerKeys_SpaceToggles(t *testing.T) {
	m := newTestModel()
	// plug-a is the first entry of the plugin tab; not pre-selected.
	if m.selected["plug-a"] {
		t.Fatalf("plug-a should not be pre-selected")
	}
	m, _ = sendKey(m, "space")
	if !m.selected["plug-a"] {
		t.Errorf("space should select plug-a, got selected=%v", m.selected)
	}
	m, _ = sendKey(m, "space")
	if m.selected["plug-a"] {
		t.Errorf("second space should deselect plug-a")
	}
}

func TestPickerKeys_ToggleAll(t *testing.T) {
	m := newTestModel()
	// Tab 0 = plugins (2 entries). Neither pre-selected.
	m, _ = sendKey(m, "a")
	if !m.selected["plug-a"] || !m.selected["plug-b"] {
		t.Errorf("'a' should select all in active tab, got %v", m.selected)
	}
	m, _ = sendKey(m, "a")
	if m.selected["plug-a"] || m.selected["plug-b"] {
		t.Errorf("second 'a' should clear all in active tab, got %v", m.selected)
	}
}

func TestPickerKeys_ToggleAllDoesNotAffectOtherTabs(t *testing.T) {
	m := newTestModel()
	// mcp-c is on tab 1 (mcp), pre-selected (default:true).
	if !m.selected["mcp-c"] {
		t.Fatalf("mcp-c should be pre-selected as a default")
	}
	// Apply 'a' while on tab 0 (plugins) — should NOT touch mcp tab.
	m, _ = sendKey(m, "a")
	if !m.selected["mcp-c"] {
		t.Errorf("toggle-all on plugins tab cleared mcp tab's selection — bug")
	}
}

func TestPickerKeys_EnterConfirmsCmdQuit(t *testing.T) {
	m := newTestModel()
	_, cmd := sendKey(m, "enter")
	if cmd == nil {
		t.Errorf("enter should return a tea.Quit cmd")
	}
}

func TestPickerKeys_EscCancels(t *testing.T) {
	m := newTestModel()
	m, _ = sendKey(m, "esc")
	if !m.cancelled {
		t.Errorf("esc should set cancelled=true")
	}
}

func TestPickerKeys_CtrlCCancels(t *testing.T) {
	m := newTestModel()
	m, _ = sendKey(m, "ctrl+c")
	if !m.cancelled {
		t.Errorf("ctrl+c should set cancelled=true")
	}
}

// ── Collect / output shape ──────────────────────────────────────────

func TestPickerModel_CollectSelectedStableOrder(t *testing.T) {
	m := newTestModel()
	// Start: mcp-c is selected (default).
	// Select plug-b too.
	m.selected["plug-b"] = true
	got := m.collectSelected()
	// Stable order: kind iteration (plugin, mcp), each kind sorted
	// by (category, id). Plugins: plug-a (code-intel) before plug-b
	// (databases). Plug-a is unselected though — only plug-b.
	want := []string{"plug-b", "mcp-c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPickerEntriesFor_SortsByCategoryThenID(t *testing.T) {
	entries := map[string]*extensions.Entry{
		"h/z-alpha": {ID: "z-alpha", Name: "Z Alpha", Harness: "h", Kind: extensions.KindMCP, Category: extensions.CategoryCodeIntel},
		"h/a-beta":  {ID: "a-beta", Name: "A Beta", Harness: "h", Kind: extensions.KindMCP, Category: extensions.CategoryDatabases},
		"h/b-gamma": {ID: "b-gamma", Name: "B Gamma", Harness: "h", Kind: extensions.KindMCP, Category: extensions.CategoryCodeIntel},
	}
	got := pickerEntriesFor(entries, "h", extensions.KindMCP)
	// code-intelligence < databases; within code-intelligence,
	// b-gamma < z-alpha by ID. Expected: b-gamma, z-alpha, a-beta.
	want := []string{"b-gamma", "z-alpha", "a-beta"}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i].ID, want[i])
		}
	}
}

// ── Rendering ───────────────────────────────────────────────────────

func TestPickerView_ShowsTabCounts(t *testing.T) {
	// View output should include each tab title with its entry count
	// in parens — this is what surfaced the "I have 32 entries"
	// confusion previously.
	m := newTestModel()
	out := stripANSI(m.View())
	if !strings.Contains(out, "Plugins (2)") {
		t.Errorf("view missing 'Plugins (2)': %q", out)
	}
	if !strings.Contains(out, "MCP servers (2)") {
		t.Errorf("view missing 'MCP servers (2)': %q", out)
	}
}

func TestPickerView_ShowsActiveTabContent(t *testing.T) {
	m := newTestModel()
	out := stripANSI(m.View())
	// Active tab is plugins — both plug-* entries should appear.
	if !strings.Contains(out, "Plug A") {
		t.Errorf("view missing Plug A: %q", out)
	}
	if !strings.Contains(out, "Plug B") {
		t.Errorf("view missing Plug B: %q", out)
	}
	// MCP entries should NOT appear (different tab).
	if strings.Contains(out, "MCP C") {
		t.Errorf("view leaked MCP tab content while plugin tab active: %q", out)
	}
}

func TestPickerView_AfterTabSwitch(t *testing.T) {
	m := newTestModel()
	m, _ = sendKey(m, "right")
	out := stripANSI(m.View())
	if !strings.Contains(out, "MCP C") {
		t.Errorf("after switching to mcp tab, view missing MCP C: %q", out)
	}
	if strings.Contains(out, "Plug A") {
		t.Errorf("after switch, plugin tab content leaked: %q", out)
	}
}

func TestPickerView_SelectionMarker(t *testing.T) {
	// mcp-c is a default — pre-selected. Switch to mcp tab and check
	// its row has the selected marker '✓' rather than the unselected '•'.
	m := newTestModel()
	m, _ = sendKey(m, "right") // tab to MCP
	out := stripANSI(m.View())
	// We can't assume order, but the line with "MCP C" should
	// contain "✓" not "•".
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "MCP C") {
			if !strings.Contains(line, "✓") {
				t.Errorf("default-selected MCP C row missing ✓ marker: %q", line)
			}
			if strings.Contains(line, "•") {
				t.Errorf("default-selected MCP C row should not have unselected • marker: %q", line)
			}
		}
	}
}

func TestPickerView_FooterShowsBindings(t *testing.T) {
	m := newTestModel()
	out := stripANSI(m.View())
	for _, want := range []string{"←/→", "↑/↓", "space", "toggle", "enter", "esc"} {
		if !strings.Contains(out, want) {
			t.Errorf("footer missing %q: %q", want, out)
		}
	}
}

// stripANSI removes ANSI escape sequences from s so substring asserts
// don't have to know about lipgloss formatting. The view always
// contains color codes; tests only care about the literal text.
func stripANSI(s string) string {
	var b strings.Builder
	skip := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0x1b {
			skip = true
			continue
		}
		if skip {
			// Skip until terminator 'm' (or other final byte ASCII 0x40-0x7E).
			if c >= 0x40 && c <= 0x7E {
				skip = false
			}
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// TestRenderDetail_ShowsFocusedEntryNote verifies the detail pane shows
// the focused entry's first body paragraph plus size + source — the
// "conscious choice" affordance for heavy bundles like ECC.
func TestRenderDetail_ShowsFocusedEntryNote(t *testing.T) {
	entries := map[string]*extensions.Entry{
		"claude-code/ecc-developer": {
			Harness: "claude-code", ID: "ecc-developer", Kind: extensions.KindPlugin,
			Name: "ECC (developer)", Source: "https://github.com/affaan-m/ECC",
			SizeMB: 6,
			Body:   "# ECC\n\nEverything Claude Code is a cross-harness bundle of agents and skills.\n",
		},
	}
	m := newPickerModel(pickerInput{HarnessID: "claude-code", Entries: entries})
	m.width = 80

	out := m.renderDetail()
	for _, want := range []string{"cross-harness bundle of agents and skills", "~6MB", "github.com/affaan-m/ECC"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderDetail() missing %q in:\n%s", want, out)
		}
	}
}

// TestRenderDetail_EmptyWhenNoEntries guards the nil-focus path.
func TestRenderDetail_EmptyWhenNoEntries(t *testing.T) {
	m := pickerModel{} // no tabs
	if got := m.renderDetail(); got != "" {
		t.Errorf("renderDetail() with no tabs = %q, want empty", got)
	}
}
