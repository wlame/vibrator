package integration

import (
	"os"
	"path/filepath"
)

// UserIntegrationsDir returns the directory holding user-defined TOML
// integration descriptors. Resolution order:
//
//  1. $VIBRATOR_INTEGRATIONS_DIR (test/override hook)
//  2. $XDG_CONFIG_HOME/vibrator/integrations
//  3. $HOME/.config/vibrator/integrations
//
// Returns "" when no home or config dir is resolvable — the loader
// treats that as "no user integrations" and silently skips loading.
func UserIntegrationsDir() string {
	if override := os.Getenv("VIBRATOR_INTEGRATIONS_DIR"); override != "" {
		return override
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "vibrator", "integrations")
	}
	home := homeOrEmpty()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "vibrator", "integrations")
}

// homeOrEmpty returns os.UserHomeDir or "" on error. Shared by
// UserIntegrationsDir and the docker runtime's "~/" volume expansion.
func homeOrEmpty() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}
