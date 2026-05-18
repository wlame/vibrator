package hostprobe

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// codexProber detects OpenAI Codex CLI installation state on the host.
//
// Codex's plugin / skills system is still in flux as of May 2026, so
// detection is intentionally minimal:
//
//  1. ~/.codex/ existence → marks Installed=true.
//
//  2. ~/.codex/skills/ contents → each subdir or .md file becomes a
//     PluginIDs entry (basename, sans extension). This is best-effort:
//     Codex may move/rename this in future releases.
//
//  3. ~/.codex/config.toml → existence noted but not parsed (the
//     interesting catalog entries we ship are app integrations like
//     github, linear, slack — those aren't tracked in config.toml today).
type codexProber struct{}

func (codexProber) HarnessID() string { return "codex" }

func (codexProber) Probe(homeBase string) (Detected, error) {
	home := filepath.Join(homeBase, ".codex")
	d := Detected{HarnessID: "codex", HomeDir: home}

	info, err := os.Stat(home)
	if err != nil || !info.IsDir() {
		return d, nil // not installed
	}
	d.Installed = true

	if ids := readCodexSkills(filepath.Join(home, "skills")); len(ids) > 0 {
		d.PluginIDs = append(d.PluginIDs, ids...)
		d.Notes = append(d.Notes, fmt.Sprintf("~/.codex/skills: %d entries", len(ids)))
	}
	if _, err := os.Stat(filepath.Join(home, "config.toml")); err == nil {
		d.Notes = append(d.Notes, "~/.codex/config.toml present")
	}

	sort.Strings(d.PluginIDs)
	return d, nil
}

// readCodexSkills returns the basenames of entries under
// ~/.codex/skills/. Both directories (typical skill layout: one dir per
// skill) and `.md` files (single-file skills) count. Errors and
// unreadable dirs are silently ignored — host scanning is best-effort.
func readCodexSkills(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue // skip dotfiles
		}
		if e.IsDir() {
			out = append(out, name)
		} else if strings.HasSuffix(name, ".md") {
			out = append(out, strings.TrimSuffix(name, ".md"))
		}
	}
	return out
}

func init() {
	Register(codexProber{})
}
