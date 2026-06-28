package hostprobe

import (
	"os"
	"os/exec"
	"path/filepath"
)

// piProber detects Pi (earendil-works/pi-coding-agent) installation.
//
// Pi has no plugin manifest — extensions are npm packages or git
// repos resolved at runtime. We detect installation via two heuristics:
//
//  1. `pi` binary on PATH (the npm-installed CLI).
//  2. ~/.pi/ directory present (user state lives here).
//
// Either signal is sufficient to mark Installed=true.
type piProber struct{}

func (piProber) HarnessID() string { return "pi" }

func (piProber) Probe(homeBase string) (Detected, error) {
	home := filepath.Join(homeBase, ".pi")
	d := Detected{HarnessID: "pi", HomeDir: home}

	if info, err := os.Stat(home); err == nil && info.IsDir() {
		d.Installed = true
		d.Notes = append(d.Notes, "~/.pi/ present")
	}
	if path, err := exec.LookPath("pi"); err == nil {
		d.Installed = true
		d.Notes = append(d.Notes, "pi binary on PATH: "+path)
	}
	return d, nil
}

func init() {
	Register(piProber{})
}
