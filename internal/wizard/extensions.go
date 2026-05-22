package wizard

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/wlame/vibrator/internal/extensions"
	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/hostprobe"
)

// kindBindings holds the per-Kind slices that huh's MultiSelect inputs
// bind to. We use separate slices (one per Kind) so each form Group
// shows only entries of one kind — easier for users to scan than a
// flat list. After the form completes, mergeKindBindings flattens them
// back into pin.Extensions.
type kindBindings struct {
	Plugin   []string
	Skill    []string
	MCP      []string
	Subagent []string
	Tool     []string
}

// buildExtensionGroups assembles the extensions selection step as a chain
// of MultiSelect groups — one per Kind — so the user navigates through
// "plugins", then "skills", then "MCP servers", etc. Each Group is
// filtered to entries belonging to the chosen harness and is skipped
// entirely when no entries match.
//
// Host-detected pre-checks: entries whose ID matches a host-detected
// raw ID (via extensions.MatchHostIDs) are pre-selected, so users with
// existing setups don't have to re-check the things they already have.
func buildExtensionGroups(pin *config.Pin, entries map[string]*extensions.Entry,
	hostDetected map[string]hostprobe.Detected,
) ([]*huh.Group, *kindBindings) {
	if pin.Harness == "" {
		// Harness not yet known — wizard layer skips this builder until
		// harness has been selected.
		return nil, nil
	}

	// Pre-compute the set of extensions IDs the host already provides, so
	// per-kind MultiSelects can pre-check them. Raw host IDs come from
	// hostprobe; extensions.MatchHostIDs translates them through
	// HostAliases.
	preChecked := preCheckedExtensionIDs(pin.Harness, entries, hostDetected)

	// Seed per-kind bindings:
	//   - pre-checked items (host-detected)
	//   - items the user already passed via --extensions (carry-over)
	bindings := &kindBindings{}
	carryOver := setFrom(pin.Extensions)
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
	for _, kind := range extensions.AllKinds {
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
	// pin.Extensions after the form completes by calling bindings.flatten().
	return groups, bindings
}

// appendByKind appends id to the appropriate per-kind slice.
func (b *kindBindings) appendByKind(k extensions.Kind, id string) {
	switch k {
	case extensions.KindPlugin:
		b.Plugin = append(b.Plugin, id)
	case extensions.KindSkill:
		b.Skill = append(b.Skill, id)
	case extensions.KindMCP:
		b.MCP = append(b.MCP, id)
	case extensions.KindSubagent:
		b.Subagent = append(b.Subagent, id)
	case extensions.KindTool:
		b.Tool = append(b.Tool, id)
	}
}

// sliceForKind returns a pointer to the per-kind slice. huh's
// MultiSelect needs a *[]string to bind to.
func (b *kindBindings) sliceForKind(k extensions.Kind) *[]string {
	switch k {
	case extensions.KindPlugin:
		return &b.Plugin
	case extensions.KindSkill:
		return &b.Skill
	case extensions.KindMCP:
		return &b.MCP
	case extensions.KindSubagent:
		return &b.Subagent
	case extensions.KindTool:
		return &b.Tool
	}
	return nil
}

// flatten concatenates the per-kind slices into one Extensions list,
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
// options labeled "Name — id  [badges]" for legibility. Badges encode
// the runtime needs in a compact form so users can spot:
//
//   - [local]              — no network, no host services
//   - [host: <integration>] — depends on a host-side service
//   - [token: $ENV]         — needs a third-party API credential
//   - [net]                 — makes outbound network calls
//
// Multiple badges concatenate. Sort by category name (then by ID
// within category) so users see related entries grouped — important
// when there are 30+ entries per harness.
func optionsForKind(entries map[string]*extensions.Entry, harnessID string,
	kind extensions.Kind,
) []huh.Option[string] {
	type entryWithSort struct {
		entry *extensions.Entry
		cat   string
	}
	var matched []entryWithSort
	for _, e := range entries {
		if e.Harness != harnessID || e.Kind != kind {
			continue
		}
		matched = append(matched, entryWithSort{
			entry: e,
			cat:   string(e.Category),
		})
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].cat != matched[j].cat {
			return matched[i].cat < matched[j].cat
		}
		return matched[i].entry.ID < matched[j].entry.ID
	})

	opts := make([]huh.Option[string], 0, len(matched))
	for _, m := range matched {
		opts = append(opts, huh.NewOption(formatEntryLabel(m.entry), m.entry.ID))
	}
	return opts
}

// formatEntryLabel composes the human-readable option label:
//
//	"<Name> — <id>  <category-tag> <runtime-badges>"
//
// Used by the wizard and exported for shared tests.
func formatEntryLabel(e *extensions.Entry) string {
	name := e.Name
	if name == "" {
		name = e.ID
	}
	label := name + " — " + e.ID
	if e.Category != "" {
		label += "  [" + extensions.CategoryLabel(e.Category) + "]"
	}
	badges := runtimeBadges(e)
	if badges != "" {
		label += "  " + badges
	}
	return label
}

// runtimeBadges renders the RuntimeNeeds into bracketed tags. Order is
// fixed (local → host → token → net) so equivalent entries always
// produce the same string — easier to spot patterns when scanning a
// list of 30+ options.
func runtimeBadges(e *extensions.Entry) string {
	var badges []string
	if e.RuntimeNeeds.LocalOnly {
		badges = append(badges, "[local]")
	}
	if e.RuntimeNeeds.SelfHosted != "" {
		badges = append(badges, "[host: "+e.RuntimeNeeds.SelfHosted+"]")
	}
	if e.Auth != nil && e.Auth.Env != "" {
		badges = append(badges, "[token: $"+e.Auth.Env+"]")
	} else if e.RuntimeNeeds.ThirdPartyAPI != "" {
		badges = append(badges, "[3rd-party: "+e.RuntimeNeeds.ThirdPartyAPI+"]")
	}
	if e.RuntimeNeeds.OutboundNet {
		badges = append(badges, "[net]")
	}
	return strings.Join(badges, " ")
}

// preCheckedExtensionIDs returns the set of extensions IDs that should be
// pre-selected based on host-detected plugins and MCP servers for the
// chosen harness.
func preCheckedExtensionIDs(harnessID string, entries map[string]*extensions.Entry,
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
	for _, id := range extensions.MatchHostIDs(entries, harnessID, merged) {
		out[id] = true
	}
	return out
}

// kindDisplayTitle is the user-facing title for a Kind.
func kindDisplayTitle(k extensions.Kind) string {
	switch k {
	case extensions.KindPlugin:
		return "Plugins"
	case extensions.KindSkill:
		return "Skills"
	case extensions.KindMCP:
		return "MCP servers"
	case extensions.KindSubagent:
		return "Subagents"
	case extensions.KindTool:
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
// Extensions lists are small (< 50 entries), so an O(n) map-based pass is
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
