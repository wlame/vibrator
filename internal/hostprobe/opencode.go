package hostprobe

import (
	"os"
	"path/filepath"
)

// openCodeProber detects OpenCode (sst/opencode) installation state.
//
// OpenCode has no marketplace; per-user agents live in `~/.opencode/` or
// `~/.config/opencode/` and the auth file at `~/.local/share/opencode/auth.json`
// is the canonical "is OpenCode installed?" signal.
//
// Plugin enumeration is intentionally not attempted yet — agents are
// arbitrary markdown files at user-chosen paths. We'll grow this when
// the extensions has enough opencode-targeted entries to warrant it.
type openCodeProber struct{}

func (openCodeProber) HarnessID() string { return "opencode" }

func (openCodeProber) Probe(homeBase string) (Detected, error) {
	// Priority order: ~/.opencode, ~/.config/opencode, ~/.local/share/opencode
	candidates := []string{
		filepath.Join(homeBase, ".opencode"),
		filepath.Join(homeBase, ".config", "opencode"),
		filepath.Join(homeBase, ".local", "share", "opencode"),
	}
	d := Detected{HarnessID: "opencode", HomeDir: candidates[0]}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			d.Installed = true
			d.HomeDir = c
			break
		}
	}
	return d, nil
}

func init() {
	Register(openCodeProber{})
}
