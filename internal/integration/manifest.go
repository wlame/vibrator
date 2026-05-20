package integration

import (
	"encoding/json"
	"io"
	"sort"
)

// ManifestEntry is the on-disk JSON shape of one integration's wiring
// for one harness. The container reads
// /etc/vibrator/integrations.json (an array of these) at every shell
// entry and uses it to refresh MCP config + env state.
//
// Keep the JSON keys snake_case for compat with the jq-driven reader
// in templates/scripts/claude-exec.sh.
type ManifestEntry struct {
	ID      string            `json:"id"`
	Harness string            `json:"harness"`
	MCP     *ManifestMCP      `json:"mcp,omitempty"`
	EnvVars map[string]string `json:"env,omitempty"`
}

// ManifestMCP is the on-disk shape of an MCPWiring. Both HTTP and
// Stdio MAY be set — the container-side script picks one based on
// reachability of the HTTP URL.
type ManifestMCP struct {
	Name  string            `json:"name"`
	HTTP  *ManifestMCPHTTP  `json:"http,omitempty"`
	Stdio *ManifestMCPStdio `json:"stdio,omitempty"`
}

// ManifestMCPHTTP is the http transport spec.
type ManifestMCPHTTP struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// ManifestMCPStdio is the stdio transport spec.
type ManifestMCPStdio struct {
	Command []string          `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// BuildManifest filters the registry to wirings that apply to harness
// (matching harness ID or "*" wildcard) and returns the serializable
// entry list. Entries with neither MCP nor EnvVars are dropped (they
// would be no-ops in the container).
//
// Output is sorted by (ID, Harness) for deterministic emission — the
// container manifest is a build-time artifact, so byte-identical
// output for identical inputs keeps Docker layer caches warm.
func BuildManifest(harness string) []ManifestEntry {
	all := All()
	var entries []ManifestEntry
	for _, integ := range all {
		for _, w := range integ.Wiring {
			if w.Harness != harness && w.Harness != "*" {
				continue
			}
			e := ManifestEntry{
				ID:      integ.ID,
				Harness: w.Harness,
				EnvVars: w.EnvVars,
			}
			if w.MCP != nil {
				e.MCP = wiringMCPToManifest(w.MCP)
			}
			if e.MCP == nil && len(e.EnvVars) == 0 {
				continue
			}
			entries = append(entries, e)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].ID != entries[j].ID {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].Harness < entries[j].Harness
	})
	return entries
}

// wiringMCPToManifest converts the in-memory MCPWiring to its on-disk
// shape. Returns nil when the wiring has no HTTP and no Stdio (empty
// MCP entries are dropped from the manifest).
func wiringMCPToManifest(m *MCPWiring) *ManifestMCP {
	out := &ManifestMCP{Name: m.Name}
	if m.HTTP != nil {
		out.HTTP = &ManifestMCPHTTP{
			URL:     m.HTTP.URL,
			Headers: m.HTTP.Headers,
		}
	}
	if m.Stdio != nil {
		out.Stdio = &ManifestMCPStdio{
			Command: m.Stdio.Command,
			Args:    m.Stdio.Args,
			Env:     m.Stdio.Env,
		}
	}
	if out.HTTP == nil && out.Stdio == nil {
		return nil
	}
	return out
}

// WriteManifest serializes BuildManifest(harness) as pretty-printed
// JSON to w. Used by the build-context preparation step to materialize
// /etc/vibrator/integrations.json before `docker build`.
//
// Always emits a JSON array — empty slice writes "[]\n" — so the
// container can always `jq '.[]'` without first checking the file
// exists or is non-empty.
func WriteManifest(w io.Writer, harness string) error {
	entries := BuildManifest(harness)
	if entries == nil {
		entries = []ManifestEntry{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}
