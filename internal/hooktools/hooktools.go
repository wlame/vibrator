// Package hooktools detects when Claude Code hook commands invoke a tool
// (node, python, ...) that the resolved image won't provide.
//
// Hooks live in ~/.claude/settings.json as shell command strings. When the
// chosen profile doesn't install the tool a hook shells out to, the hook
// fails on every event with a noisy "<tool>: not found". This package lets
// the orchestrator spot that gap before launch (so it can offer to install
// the backing feature) and backs the container-side guard that skips
// unrunnable hooks at runtime.
//
// Detection is deliberately heuristic: it scans command strings for
// word-boundary references to a known set of tool names and maps each to the
// internal/feature ID that installs it. Base-toolkit tools (jq, rg, git, ...)
// ship in every image and are intentionally not tracked here.
package hooktools

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// toolFeature maps a tool name (as it appears as a command word in a hook)
// to the internal/feature ID that installs it. Several tools can map to one
// feature (npm/npx/bun all come from "node").
//
// Keep the values in sync with internal/feature.Registry — the test
// TestToolFeatureValuesAreKnownFeatures fails if a value drifts from a real
// feature ID. Keep the KEYS in sync with the equivalent grep alternation in
// templates/scripts/entrypoint.sh (the container guard) so host-side and
// container-side detection agree.
var toolFeature = map[string]string{
	"node": "node", "npm": "node", "npx": "node", "bun": "node",
	"python": "python", "python3": "python", "pip": "python", "pip3": "python",
	"uv": "python", "uvx": "python",
	"go":         "go",
	"gh":         "gh",
	"docker":     "docker-cli",
	"psql":       "postgres-client",
	"pg_dump":    "postgres-client",
	"pg_restore": "postgres-client",
	"aider":      "aider",
	"ralphex":    "ralphex",
	"codex":      "codex-cli",
	"playwright": "playwright",
}

// toolRe matches any tracked tool name on a word boundary. Built once from
// the map keys. Word boundaries keep "node" from matching inside "nodemon"
// and let "/usr/local/bin/node" still match (the leading "/" is a boundary).
var toolRe = buildToolRegex()

func buildToolRegex() *regexp.Regexp {
	keys := make([]string, 0, len(toolFeature))
	for k := range toolFeature {
		keys = append(keys, k)
	}
	// Longest-first so the alternation prefers the most specific token
	// (e.g. "python3" before "python"). Word boundaries make this mostly
	// moot, but it keeps the generated pattern unambiguous and stable.
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	return regexp.MustCompile(`\b(` + strings.Join(keys, "|") + `)\b`)
}

// Gap is a missing feature implied by one or more hook commands.
type Gap struct {
	// Feature is the internal/feature ID that would install the tool(s).
	Feature string
	// Tools are the distinct tracked tools referenced, sorted.
	Tools []string
	// Commands are the hook command strings that referenced them, deduped
	// in first-seen order. Useful for showing the user an example.
	Commands []string
}

// Commands extracts every hook command string from a Claude Code
// settings.json blob, in deterministic order. Returns nil on malformed or
// empty input — hook detection is best-effort and never blocks a launch.
func Commands(settingsJSON []byte) []string {
	var s struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(settingsJSON, &s); err != nil {
		return nil
	}

	// Iterate events in sorted order so output is stable across runs.
	events := make([]string, 0, len(s.Hooks))
	for e := range s.Hooks {
		events = append(events, e)
	}
	sort.Strings(events)

	var out []string
	for _, e := range events {
		for _, group := range s.Hooks[e] {
			for _, h := range group.Hooks {
				if strings.TrimSpace(h.Command) != "" {
					out = append(out, h.Command)
				}
			}
		}
	}
	return out
}

// Scan returns the feature gaps implied by commands, given the set of enabled
// feature IDs. A gap is reported when a command references a tracked tool
// whose backing feature is not in enabledFeatures. Gaps are grouped by
// feature and sorted by feature ID for stable output.
func Scan(commands, enabledFeatures []string) []Gap {
	enabled := make(map[string]bool, len(enabledFeatures))
	for _, f := range enabledFeatures {
		enabled[f] = true
	}

	type acc struct {
		tools    map[string]bool
		seenCmd  map[string]bool
		commands []string
	}
	byFeature := map[string]*acc{}

	for _, cmd := range commands {
		for _, tool := range toolRe.FindAllString(cmd, -1) {
			feat := toolFeature[tool]
			if enabled[feat] {
				continue
			}
			a := byFeature[feat]
			if a == nil {
				a = &acc{tools: map[string]bool{}, seenCmd: map[string]bool{}}
				byFeature[feat] = a
			}
			a.tools[tool] = true
			if !a.seenCmd[cmd] {
				a.seenCmd[cmd] = true
				a.commands = append(a.commands, cmd)
			}
		}
	}

	feats := make([]string, 0, len(byFeature))
	for f := range byFeature {
		feats = append(feats, f)
	}
	sort.Strings(feats)

	gaps := make([]Gap, 0, len(feats))
	for _, f := range feats {
		a := byFeature[f]
		tools := make([]string, 0, len(a.tools))
		for t := range a.tools {
			tools = append(tools, t)
		}
		sort.Strings(tools)
		gaps = append(gaps, Gap{Feature: f, Tools: tools, Commands: a.commands})
	}
	return gaps
}
