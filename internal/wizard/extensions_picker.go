package wizard

// Tabbed multi-select picker for extension entries.
//
// Replaces the previous "one huh.Group per Kind" UX, where the user
// had to walk through 5 separate full-screen pages (Plugins → Skills
// → MCPs → Subagents → Tools) and didn't realize there was anything
// past the first one. The picker shows all kinds as TABS at the top;
// left/right cycles between them, up/down moves the cursor, space
// toggles. Selections persist across tab switches.
//
// Implemented as a custom Bubble Tea program (huh v1 has no tab-pane
// component) — but only for this one wizard step. The harness /
// profile / shell / LLM steps still use huh.
//
// Visual roughly:
//
//	  Extensions for claude-code
//
//	  Plugins (10)  MCP servers (19)  Skills (1)  Subagents (2)  Tools (0)
//	  ────────────
//
//	    ✓  cc-thingz — bundle of small CC helpers
//	  ▶ ✓  claude-mem — Persistent memory across sessions    [host: ...]
//	    •  frontend-design — frontend-design
//	    ...
//
//	  ←/→ switch tab   ↑/↓ scroll   space toggle   a toggle all
//	  enter confirm    esc cancel

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/wlame/vibrator/internal/extensions"
	"github.com/wlame/vibrator/internal/hostprobe"
)

// pickerInput bundles everything the picker needs to construct its
// internal state. Kept separate from the model so unit tests can
// construct models directly without going through Run().
type pickerInput struct {
	// HarnessID filters which entries appear (entry.Harness must
	// match).
	HarnessID string

	// Entries is the full catalogue map (key is "<harness>/<id>"),
	// typically the result of extensions.LoadAll.
	Entries map[string]*extensions.Entry

	// HostDetected is the hostprobe result keyed by harness ID. The
	// picker uses it to pre-check entries the user already has
	// installed on the host.
	HostDetected map[string]hostprobe.Detected

	// InitialSelection is the list of entry IDs already in the pin
	// (e.g., carried over from --extensions flag or a prior session).
	// They start checked.
	InitialSelection []string
}

// pickerResult is what RunPicker returns.
type pickerResult struct {
	// SelectedIDs is the union of every checked entry across every
	// tab, in stable order (kind → category → id).
	SelectedIDs []string

	// Cancelled is true when the user hit esc / ctrl-c. Callers
	// should treat that as "user backed out — leave pin.Extensions
	// alone".
	Cancelled bool
}

// RunPicker launches the tabbed picker as a Bubble Tea program and
// blocks until the user confirms or cancels. ctx cancellation closes
// the program gracefully.
//
// Returns (zero result, nil) when no entries match the harness — the
// caller should treat that as "no picker shown, no selection
// changed". This keeps the wizard graceful for harnesses with empty
// extension folders.
func RunPicker(ctx context.Context, in pickerInput) (pickerResult, error) {
	m := newPickerModel(in)
	if len(m.tabs) == 0 {
		return pickerResult{SelectedIDs: in.InitialSelection}, nil
	}

	prog := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	final, err := prog.Run()
	if err != nil {
		return pickerResult{}, fmt.Errorf("picker: %w", err)
	}
	fm := final.(pickerModel)
	if fm.cancelled {
		return pickerResult{Cancelled: true}, nil
	}
	return pickerResult{SelectedIDs: fm.collectSelected()}, nil
}

// ── Model ───────────────────────────────────────────────────────────

// pickerTab is one tab in the bar — represents all entries of one Kind.
type pickerTab struct {
	kind    extensions.Kind
	title   string              // "Plugins"
	entries []*extensions.Entry // pre-sorted (category → id)
}

// pickerModel is the Bubble Tea state for the picker. All fields are
// unexported — RunPicker is the only public entry point.
type pickerModel struct {
	harnessID string
	tabs      []pickerTab

	activeTab int       // index into tabs
	cursors   []int     // cursor row per tab (parallels tabs)
	selected  map[string]bool

	// Terminal size; updated on tea.WindowSizeMsg. Renderer uses
	// these to truncate / paginate.
	width  int
	height int

	cancelled bool
}

// newPickerModel filters + sorts entries per kind, seeds the initial
// selection from defaults + host detection + carry-over, and returns
// a ready-to-run model. Pure — no I/O, easy to unit-test.
func newPickerModel(in pickerInput) pickerModel {
	preChecked := preCheckedExtensionIDs(in.HarnessID, in.Entries, in.HostDetected)
	carryOver := setFrom(in.InitialSelection)

	selected := map[string]bool{}
	for id, ok := range preChecked {
		if ok {
			selected[id] = true
		}
	}
	for id := range carryOver {
		selected[id] = true
	}

	var tabs []pickerTab
	for _, k := range extensions.AllKinds {
		ents := pickerEntriesFor(in.Entries, in.HarnessID, k)
		// Empty kinds are skipped — there's no point showing an
		// empty tab the user can't do anything with.
		if len(ents) == 0 {
			continue
		}
		// Seed any default-on entries (not already covered above).
		for _, e := range ents {
			if e.Default {
				selected[e.ID] = true
			}
		}
		tabs = append(tabs, pickerTab{
			kind:    k,
			title:   kindDisplayTitle(k),
			entries: ents,
		})
	}

	return pickerModel{
		harnessID: in.HarnessID,
		tabs:      tabs,
		cursors:   make([]int, len(tabs)),
		selected:  selected,
	}
}

// pickerEntriesFor returns the entries for one (harness, kind) pair,
// sorted by (category, id) — same order as the old huh-based picker
// used. Tests pin this for stability.
func pickerEntriesFor(entries map[string]*extensions.Entry,
	harness string, kind extensions.Kind,
) []*extensions.Entry {
	var matched []*extensions.Entry
	for _, e := range entries {
		if e.Harness == harness && e.Kind == kind {
			matched = append(matched, e)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		ci, cj := string(matched[i].Category), string(matched[j].Category)
		if ci != cj {
			return ci < cj
		}
		return matched[i].ID < matched[j].ID
	})
	return matched
}

// collectSelected returns every selected entry across all tabs,
// in stable order (kind → category → id). Used to populate
// pin.Extensions on confirm.
func (m pickerModel) collectSelected() []string {
	var out []string
	for _, t := range m.tabs {
		for _, e := range t.entries {
			if m.selected[e.ID] {
				out = append(out, e.ID)
			}
		}
	}
	return out
}

// ── tea.Model interface ─────────────────────────────────────────────

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey is split out so unit tests can drive it without
// constructing a real tea.Program. Returns the updated model and
// any command (mostly tea.Quit on confirm/cancel).
func (m pickerModel) handleKey(msg tea.KeyMsg) (pickerModel, tea.Cmd) {
	if len(m.tabs) == 0 {
		// Defensive — RunPicker would have returned early, but a
		// hand-constructed model could end up here.
		m.cancelled = true
		return m, tea.Quit
	}

	switch msg.String() {
	case "ctrl+c", "esc", "q":
		m.cancelled = true
		return m, tea.Quit

	case "enter":
		return m, tea.Quit

	case "left", "h", "shift+tab":
		m.activeTab = (m.activeTab - 1 + len(m.tabs)) % len(m.tabs)

	case "right", "l", "tab":
		m.activeTab = (m.activeTab + 1) % len(m.tabs)

	case "up", "k":
		if m.cursors[m.activeTab] > 0 {
			m.cursors[m.activeTab]--
		}

	case "down", "j":
		if m.cursors[m.activeTab] < len(m.tabs[m.activeTab].entries)-1 {
			m.cursors[m.activeTab]++
		}

	case "g":
		m.cursors[m.activeTab] = 0

	case "G":
		m.cursors[m.activeTab] = len(m.tabs[m.activeTab].entries) - 1

	case " ", "x":
		// Toggle the entry under the cursor in the active tab.
		t := m.tabs[m.activeTab]
		if c := m.cursors[m.activeTab]; c >= 0 && c < len(t.entries) {
			id := t.entries[c].ID
			m.selected[id] = !m.selected[id]
		}

	case "a":
		// Toggle ALL entries in the active tab — if all are already
		// selected, deselect; otherwise select. Matches the "select
		// all / clear" UX users expect.
		t := m.tabs[m.activeTab]
		allSelected := true
		for _, e := range t.entries {
			if !m.selected[e.ID] {
				allSelected = false
				break
			}
		}
		for _, e := range t.entries {
			m.selected[e.ID] = !allSelected
		}
	}
	return m, nil
}

// ── Rendering ───────────────────────────────────────────────────────

var (
	pickerHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFDF5")).
				Background(lipgloss.Color("#7D56F4")).
				Padding(0, 1)

	pickerTabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	pickerTabInactive = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Padding(0, 1)

	pickerSelectedRow = lipgloss.NewStyle().Foreground(lipgloss.Color("#39FF14"))
	pickerCursorRow   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	pickerNormalRow   = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	pickerHintStyle   = lipgloss.NewStyle().Faint(true).Italic(true)
	pickerFooterStyle = lipgloss.NewStyle().Faint(true)
)

func (m pickerModel) View() string {
	if len(m.tabs) == 0 {
		return "(no extensions to choose from)\n"
	}
	var b strings.Builder
	b.WriteString(pickerHeaderStyle.Render(
		fmt.Sprintf("Extensions for %s", m.harnessID),
	))
	b.WriteString("\n\n")
	b.WriteString(m.renderTabs())
	b.WriteString("\n")
	b.WriteString(m.renderList())
	b.WriteString("\n")
	b.WriteString(m.renderFooter())
	return b.String()
}

// renderTabs draws the kind tabs at the top. The active tab is
// highlighted; each tab shows its entry count.
func (m pickerModel) renderTabs() string {
	parts := make([]string, 0, len(m.tabs))
	for i, t := range m.tabs {
		label := fmt.Sprintf("%s (%d)", t.title, len(t.entries))
		if i == m.activeTab {
			parts = append(parts, pickerTabActive.Render(label))
		} else {
			parts = append(parts, pickerTabInactive.Render(label))
		}
	}
	return strings.Join(parts, " ")
}

// renderList draws the items of the active tab with cursor + check
// markers. Selected items use a colored row; the cursor row gets a
// "▶" prefix.
func (m pickerModel) renderList() string {
	t := m.tabs[m.activeTab]
	cursor := m.cursors[m.activeTab]
	var b strings.Builder
	for i, e := range t.entries {
		var (
			pointer string
			mark    string
			style   lipgloss.Style
		)
		if i == cursor {
			pointer = "▶ "
			style = pickerCursorRow
		} else {
			pointer = "  "
			style = pickerNormalRow
		}
		if m.selected[e.ID] {
			mark = "✓ "
			if i != cursor {
				style = pickerSelectedRow
			}
		} else {
			mark = "• "
		}
		row := pointer + mark + formatEntryLabel(e)
		b.WriteString(style.Render(row))
		b.WriteString("\n")
	}
	if len(t.entries) == 0 {
		b.WriteString(pickerHintStyle.Render("  (no entries)\n"))
	}
	return b.String()
}

// renderFooter prints the keybinding cheat-sheet. Always shown so
// new users can discover navigation.
func (m pickerModel) renderFooter() string {
	count := 0
	for id := range m.selected {
		if m.selected[id] {
			count++
		}
	}
	left := pickerFooterStyle.Render(
		"←/→ switch tab    ↑/↓ scroll    space toggle    a toggle-all    enter confirm    esc cancel",
	)
	right := pickerFooterStyle.Render(fmt.Sprintf("%d selected", count))
	return left + "\n" + right
}
