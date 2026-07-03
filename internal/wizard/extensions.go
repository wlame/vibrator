package wizard

// Helpers used by the tabbed extension picker (extensions_picker.go).
// The old per-kind huh.Group flow that used to live here was retired
// in favour of the tea.Model picker, which gives the user a single
// tabbed view across all kinds. Helpers below survived because they
// produce data shapes the new picker also consumes (per-entry label
// rendering, kind→display-title mapping, host-detection pre-checks).

import (
	"regexp"
	"sort"
	"strings"

	"github.com/wlame/vibrator/internal/extensions"
	"github.com/wlame/vibrator/internal/hostprobe"
)

// markdownLinkRe matches an inline markdown link `[text](url)` so the
// detail pane can flatten it to just `text` — URLs read as noise in a
// one-line terminal blurb (the source URL is shown separately).
var markdownLinkRe = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)

// entryNote returns the first prose paragraph of the entry's markdown
// body, used by the picker's detail pane to answer "what does this do"
// for the focused entry. The leading H1 title and any blank lines are
// skipped; the first paragraph's lines are joined into one line (the
// picker wraps it to the terminal width). Inline markdown links are
// flattened to their text. Returns "" when the body has no prose.
func entryNote(e *extensions.Entry) string {
	var para []string
	started := false
	for _, raw := range strings.Split(e.Body, "\n") {
		line := strings.TrimSpace(raw)
		if !started {
			// Skip leading blank lines and heading lines until the
			// first real prose line.
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			started = true
		} else if line == "" {
			break // blank line ends the first paragraph
		}
		para = append(para, line)
	}
	return markdownLinkRe.ReplaceAllString(strings.Join(para, " "), "$1")
}

// formatEntryLabel composes the human-readable option label.
// Shape depends on whether the entry declares a Description:
//
//	with description:    "<Name> — <description>  [Category] [badges]  (<id>)"
//	without description: "<Name> — <id>  [Category] [badges]"
//
// The Description path gives users a "what does this do" hint inline;
// the id moves to a parenthetical at the end so `--extensions=<id>` is
// still copy-pasteable. Without a description we fall through to the
// older "name — id" anchor so the option doesn't look empty.
//
// Both the tabbed picker and `vibrate extensions list` could call
// this; the contract is stable.
func formatEntryLabel(e *extensions.Entry) string {
	name := e.Name
	if name == "" {
		name = e.ID
	}

	var label string
	if e.Description != "" {
		label = name + " — " + e.Description
	} else {
		label = name + " — " + e.ID
	}

	if e.Category != "" {
		label += "  [" + extensions.CategoryLabel(e.Category) + "]"
	}
	if b := runtimeBadges(e); b != "" {
		label += "  " + b
	}
	if len(e.PinnedModels) > 0 {
		label += "  [pins: " + strings.Join(e.PinnedModels, ", ") + "]"
	}

	if e.Description != "" {
		// Description path moved the id out of the prefix — add it as
		// a copy-pasteable trailer so users still see what to pass
		// to --extensions.
		label += "  (" + e.ID + ")"
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

// preCheckedExtensionIDs returns the set of extensions IDs that
// should be pre-selected based on host-detected plugins and MCP
// servers for the chosen harness. Used by the picker's startup
// state.
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

// kindDisplayTitle is the user-facing title for a Kind. Used by the
// picker's tab bar.
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

// Anchor for go vet — sort is used by the picker (extensions_picker.go)
// in pickerEntriesFor. Without an import in this file, dropping all
// dead code would dangling-reference an import that we ALSO need —
// but the picker file has its own sort import. Touching package-local
// helpers only.
var _ = sort.Strings // intentional: documents shared sort dep
