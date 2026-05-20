package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserIntegrationsDir_EnvOverride(t *testing.T) {
	t.Setenv("VIBRATOR_INTEGRATIONS_DIR", "/tmp/explicit-override")
	if got := UserIntegrationsDir(); got != "/tmp/explicit-override" {
		t.Errorf("UserIntegrationsDir = %q, want /tmp/explicit-override", got)
	}
}

func TestUserIntegrationsDir_XDG(t *testing.T) {
	t.Setenv("VIBRATOR_INTEGRATIONS_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	got := UserIntegrationsDir()
	want := filepath.Join("/tmp/xdg", "vibrator", "integrations")
	if got != want {
		t.Errorf("UserIntegrationsDir = %q, want %q", got, want)
	}
}

func TestUserIntegrationsDir_HomeFallback(t *testing.T) {
	t.Setenv("VIBRATOR_INTEGRATIONS_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	got := UserIntegrationsDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("UserHomeDir failed — can't check fallback")
	}
	if !strings.HasPrefix(got, home) {
		t.Errorf("UserIntegrationsDir = %q, expected to start with HOME=%q", got, home)
	}
	if !strings.HasSuffix(got, filepath.Join(".config", "vibrator", "integrations")) {
		t.Errorf("UserIntegrationsDir = %q, expected to end with .config/vibrator/integrations", got)
	}
}
