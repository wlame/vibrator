package wizard

import (
	"fmt"

	"github.com/charmbracelet/huh"

	"github.com/wlame/vibrator/internal/catalog"
	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/hostprobe"
)

// kindBindings holds the per-Kind slices that huh's MultiSelect inputs
// bind to. We use separate slices (one per Kind) so each form Group
// shows only entries of one kind — easier for users to scan than a
// flat list. After the form completes, mergeKindBindings flattens them
// back into pin.Catalog.
type kindBindings struct {
	Plugin   []string
	Skill    []string
	MCP      []string
	Subagent []string
	Tool     []string
}

// buildCatalogGroups assembles the catalog selection step as a chain
// of MultiSelect groups — one per Kind — so the user navigates through
// "plugins", then "skills", then "MCP servers", etc. Each Group is
// filtered to entries belonging to the chosen harness and is skipped
// entirely when no entries match.
//
// Host-detected pre-checks: entries whose ID matches a host-detected
// raw ID (via catalog.MatchHostIDs) are pre-selected, so users with
// existing setups don't have to re-check the things they already have.
func buildCatalogGroups(pin *config.Pin, entries map[string]*catalog.Entry,
	hostDetected map[string]hostprobe.Detected,
) ([]*huh.Group, *kindBindings) {
	if pin.Harness == "" {
		// Harness not yet known — wizard layer skips this builder until
		// harness has been selected.
		return nil, nil
	}

	// Pre-compute the set of catalog IDs the host already provides, so
	// per-kind MultiSelects can pre-check them. Raw host IDs come from
	// hostprobe; catalog.MatchHostIDs translates them through
	// HostAliases.
	preChecked := preCheckedCatalogIDs(pin.Harness, entries, hostDetected)

	// Seed per-kind bindings:
	//   - pre-checked items (host-detected)
	//   - items the user already passed via --catalog (carry-over)
	bindings := &kindBindings{}
	carryOver := setFrom(pin.Catalog)
	for _, e := range entries {
		if e.Harness != pin.Harness {
			continue
		}
		if !preChecked[e.ID] && !carryOver[e.ID] {
			continue
		}
		bindings.appendByKind(e.Kind, e.ID)
	}

	// Build one Group per Kind, in display order (AllKinds).
	var groups []*huh.Group
	for _, kind := range catalog.AllKinds {
		opts := optionsForKind(entries, pin.Harness, kind)
		if len(opts) == 0 {
			continue
		}

		title := kindDisplayTitle(kind)
		desc := fmt.Sprintf("Pre-checked items were detected on your host (~/.%s).", pin.Harness)

		groups = append(groups, huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title(title).
				Description(desc).
				Options(opts...).
				Value(bindings.sliceForKind(kind)),
		))
	}

	// Bindings live in `bindings`; the caller folds them into
	// pin.Catalog after the form completes by calling bindings.flatten().
	return groups, bindings
}

// appendByKind appends id to the appropriate per-kind slice.
func (b *kindBindings) appendByKind(k catalog.Kind, id string) {
	switch k {
	case catalog.KindPlugin:
		b.Plugin = append(b.Plugin, id)
	case catalog.KindSkill:
		b.Skill = append(b.Skill, id)
	case catalog.KindMCP:
		b.MCP = append(b.MCP, id)
	case catalog.KindSubagent:
		b.Subagent = append(b.Subagent, id)
	case catalog.KindTool:
		b.Tool = append(b.Tool, id)
	}
}

// sliceForKind returns a pointer to the per-kind slice. huh's
// MultiSelect needs a *[]string to bind to.
func (b *kindBindings) sliceForKind(k catalog.Kind) *[]string {
	switch k {
	case catalog.KindPlugin:
		return &b.Plugin
	case catalog.KindSkill:
		return &b.Skill
	case catalog.KindMCP:
		return &b.MCP
	case catalog.KindSubagent:
		return &b.Subagent
	case catalog.KindTool:
		return &b.Tool
	}
	return nil
}

// flatten concatenates the per-kind slices into one Catalog list,
// preserving Kind iteration order (plugin → skill → mcp → subagent → tool).
// Duplicates are removed (shouldn't happen in practice since each kind
// is disjoint, but cheap insurance).
func (b *kindBindings) flatten() []string {
	out := make([]string, 0,
		len(b.Plugin)+len(b.Skill)+len(b.MCP)+len(b.Subagent)+len(b.Tool))
	out = append(out, b.Plugin...)
	out = append(out, b.Skill...)
	out = append(out, b.MCP...)
	out = append(out, b.Subagent...)
	out = append(out, b.Tool...)
	return dedupe(out)
}

// optionsForKind filters entries by harness + kind and returns huh
// options labeled "Name — id" for legibility.
func optionsForKind(entries map[string]*catalog.Entry, harnessID string,
	kind catalog.Kind,
) []huh.Option[string] {
	var opts []huh.Option[string]
	for _, e := range entries {
		if e.Harness != harnessID || e.Kind != kind {
			continue
		}
		label := e.Name
		if label == "" {
			label = e.ID
		}
		opts = append(opts, huh.NewOption(label+" — "+e.ID, e.ID))
	}
	return opts
}

// preCheckedCatalogIDs returns the set of catalog IDs that should be
// pre-selected based on host-detected plugins and MCP servers for the
// chosen harness.
func preCheckedCatalogIDs(harnessID string, entries map[string]*catalog.Entry,
	hostDetected map[string]hostprobe.Detected,
) map[string]bool {
	out := make(map[string]bool)
	if hostDetected == nil {
		return out
	}
	d, ok := hostDetected[harnessID]
	if !ok || !d.Installed {
		return out
	}
	merged := append(append([]string{}, d.PluginIDs...), d.MCPServers...)
	for _, id := range catalog.MatchHostIDs(entries, harnessID, merged) {
		out[id] = true
	}
	return out
}

// kindDisplayTitle is the user-facing title for a Kind.
func kindDisplayTitle(k catalog.Kind) string {
	switch k {
	case catalog.KindPlugin:
		return "Plugins"
	case catalog.KindSkill:
		return "Skills"
	case catalog.KindMCP:
		return "MCP servers"
	case catalog.KindSubagent:
		return "Subagents"
	case catalog.KindTool:
		return "Tools"
	}
	return string(k)
}

// setFrom returns a presence map for stable membership checks.
func setFrom(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, x := range items {
		m[x] = true
	}
	return m
}

// dedupe removes duplicates while preserving first-occurrence order.
// Catalog lists are small (< 50 entries), so an O(n) map-based pass is
// fine.
func dedupe(items []string) []string {
	if len(items) <= 1 {
		return items
	}
	seen := make(map[string]bool, len(items))
	out := items[:0:len(items)]
	for _, x := range items {
		if seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	return out
}
