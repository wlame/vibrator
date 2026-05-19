package hostprobe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// claudeCodeProber detects Claude Code installation state on the host.
//
// Data sources (probed in order, additive):
//
//  1. ~/.claude/plugins/installed_plugins.json
//     The authoritative manifest on recent versions. Schema:
//        { "version": 2,
//          "plugins": { "<id>@<marketplace>": [ {...}, ... ], ... }
//        }
//     Keys are split on "@" — the part before the @ is the plugin id.
//
//  2. ~/.claude/settings.json
//     Legacy `enabledPlugins` object (string→bool). Used as a fallback
//     when plugins/installed_plugins.json is missing or returns no entries.
//
//  3. ~/.claude.json (in $HOME, NOT inside ~/.claude/)
//     Top-level user config. `mcpServers` object's keys are the MCP server
//     names. Not all installs have this file; that's fine.
type claudeCodeProber struct{}

func (claudeCodeProber) HarnessID() string { return "claude-code" }

func (claudeCodeProber) Probe(homeBase string) (Detected, error) {
	home := filepath.Join(homeBase, ".claude")
	d := Detected{HarnessID: "claude-code", HomeDir: home}

	info, err := os.Stat(home)
	if err != nil || !info.IsDir() {
		// Home dir missing — claude-code is not installed (or installed
		// somewhere unconventional). Return early with Installed=false; this
		// is NOT an error condition.
		return d, nil
	}
	d.Installed = true

	// --- Plugins: try the new manifest first, fall back to legacy. ---

	manifestPath := filepath.Join(home, "plugins", "installed_plugins.json")
	if ids, note, err := readClaudeInstalledPlugins(manifestPath); err == nil && len(ids) > 0 {
		d.PluginIDs = append(d.PluginIDs, ids...)
		if note != "" {
			d.Notes = append(d.Notes, note)
		}
	} else {
		settingsPath := filepath.Join(home, "settings.json")
		if ids, note, err := readClaudeEnabledPlugins(settingsPath); err == nil && len(ids) > 0 {
			d.PluginIDs = append(d.PluginIDs, ids...)
			if note != "" {
				d.Notes = append(d.Notes, note)
			}
		}
	}

	// --- MCP servers from ~/.claude.json (not inside ~/.claude/). ---

	mcpPath := filepath.Join(homeBase, ".claude.json")
	if mcps, note, err := readClaudeMCPServers(mcpPath); err == nil && len(mcps) > 0 {
		d.MCPServers = mcps
		if note != "" {
			d.Notes = append(d.Notes, note)
		}
	}

	// --- Registered marketplaces from plugins/known_marketplaces.json ---

	mpPath := filepath.Join(home, "plugins", "known_marketplaces.json")
	if mps, note, err := readClaudeMarketplaces(mpPath); err == nil && len(mps) > 0 {
		d.Marketplaces = mps
		if note != "" {
			d.Notes = append(d.Notes, note)
		}
	}

	sort.Strings(d.PluginIDs)
	sort.Strings(d.MCPServers)
	sort.Strings(d.Marketplaces)
	return d, nil
}

// claudeMarketplacesFile is the shape of `plugins/known_marketplaces.json`.
// Top-level keys are the short marketplace IDs (e.g.
// "claude-plugins-official"); we only need the keys.
type claudeMarketplacesFile map[string]json.RawMessage

// readClaudeMarketplaces returns the short marketplace IDs registered on
// the host. Surfaces the canonical short name (the key of the JSON
// object), NOT the underlying GitHub repo path — that's the
// identifier `claude plugin install <id>@<marketplace>` expects, and
// the one catalog entries should use.
func readClaudeMarketplaces(path string) ([]string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	// The file has shape: {"marketplaces": { ... }} OR top-level object
	// directly. Try the wrapped form first (newer layouts), fall back
	// to plain.
	var wrapped struct {
		Marketplaces claudeMarketplacesFile `json:"marketplaces"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.Marketplaces) > 0 {
		ids := collectMarketplaceIDs(wrapped.Marketplaces)
		return ids, fmt.Sprintf("plugins/known_marketplaces.json: %d marketplaces", len(ids)), nil
	}
	var plain claudeMarketplacesFile
	if err := json.Unmarshal(data, &plain); err != nil {
		return nil, "", fmt.Errorf("decode %s: %w", path, err)
	}
	ids := collectMarketplaceIDs(plain)
	return ids, fmt.Sprintf("plugins/known_marketplaces.json: %d marketplaces", len(ids)), nil
}

// collectMarketplaceIDs extracts sorted top-level keys.
func collectMarketplaceIDs(m claudeMarketplacesFile) []string {
	ids := make([]string, 0, len(m))
	for k := range m {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	return ids
}

// claudeInstalledPluginsFile is the minimal shape we care about from
// `plugins/installed_plugins.json`. We only read the keys of `plugins`;
// the per-key array of install records is left as `json.RawMessage` so
// the parser doesn't fail if Anthropic adds new fields.
type claudeInstalledPluginsFile struct {
	Version int                        `json:"version"`
	Plugins map[string]json.RawMessage `json:"plugins"`
}

// readClaudeInstalledPlugins extracts plugin IDs from the new
// `installed_plugins.json` manifest. Keys look like
// "<id>@<marketplace>"; we keep only the `<id>` portion.
//
// Returns (ids, note, err). The note carries a short human-readable
// summary suitable for Detected.Notes; err is set only when the file
// exists but won't decode.
func readClaudeInstalledPlugins(path string) ([]string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var f claudeInstalledPluginsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, "", fmt.Errorf("decode %s: %w", path, err)
	}

	ids := make([]string, 0, len(f.Plugins))
	for k := range f.Plugins {
		ids = append(ids, splitAtMarketplace(k))
	}
	sort.Strings(ids)
	note := fmt.Sprintf("plugins/installed_plugins.json: %d entries", len(ids))
	return ids, note, nil
}

// claudeSettingsFile captures only the legacy `enabledPlugins` map. Any
// other fields (statusLine, mcpServers, etc.) are ignored.
type claudeSettingsFile struct {
	EnabledPlugins map[string]bool `json:"enabledPlugins"`
}

// readClaudeEnabledPlugins extracts plugin IDs from the legacy
// settings.json::enabledPlugins object. Same `<id>@<marketplace>` key
// shape as installed_plugins.json.
//
// Used only as a fallback when the new manifest is absent or empty.
func readClaudeEnabledPlugins(path string) ([]string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var f claudeSettingsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, "", fmt.Errorf("decode %s: %w", path, err)
	}

	ids := make([]string, 0, len(f.EnabledPlugins))
	for k, on := range f.EnabledPlugins {
		if !on {
			continue
		}
		ids = append(ids, splitAtMarketplace(k))
	}
	sort.Strings(ids)
	note := fmt.Sprintf("settings.json::enabledPlugins: %d entries (legacy fallback)", len(ids))
	return ids, note, nil
}

// claudeRootConfigFile captures only `mcpServers` from ~/.claude.json.
type claudeRootConfigFile struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
}

// readClaudeMCPServers extracts the user-level MCP server names from
// ~/.claude.json's `mcpServers` object. Server identifiers are the keys
// (no marketplace suffix here, unlike plugins).
func readClaudeMCPServers(path string) ([]string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var f claudeRootConfigFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, "", fmt.Errorf("decode %s: %w", path, err)
	}
	names := make([]string, 0, len(f.MCPServers))
	for k := range f.MCPServers {
		names = append(names, k)
	}
	sort.Strings(names)
	note := fmt.Sprintf(".claude.json::mcpServers: %d servers", len(names))
	return names, note, nil
}

// splitAtMarketplace returns the part of an "<id>@<marketplace>" key
// before the `@`. If the key has no `@`, it's returned unchanged.
//
// Examples:
//
//	"context7@claude-plugins-official" → "context7"
//	"claude-mem@thedotmack"            → "claude-mem"
//	"my-local-plugin"                  → "my-local-plugin"
func splitAtMarketplace(key string) string {
	if at := strings.IndexByte(key, '@'); at > 0 {
		return key[:at]
	}
	return key
}

func init() {
	Register(claudeCodeProber{})
}
