package app

import (
	"testing"

	"github.com/wlame/vibrator/internal/config"
)

func TestIntegrationModeEnvVar(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"serena", "VIBRATOR_INTEGRATION_MODE_SERENA"},
		{"claude-mem", "VIBRATOR_INTEGRATION_MODE_CLAUDE_MEM"},
		{"a.b-c", "VIBRATOR_INTEGRATION_MODE_A_B_C"},
	}
	for _, tc := range tests {
		if got := integrationModeEnvVar(tc.id); got != tc.want {
			t.Errorf("integrationModeEnvVar(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

// buildContainerEnv must forward each pinned integration mode as an env var
// so the container's claude-exec wrapper can pick the right transport.
func TestBuildContainerEnv_ForwardsIntegrationModes(t *testing.T) {
	pin := config.Pin{
		Harness: "claude-code",
		Integrations: map[string]string{
			"serena":     "host",
			"claude-mem": "off",
		},
	}

	env, err := buildContainerEnv(pin, nil)
	if err != nil {
		t.Fatalf("buildContainerEnv: %v", err)
	}

	got := make(map[string]string, len(env))
	for _, e := range env {
		got[e.Name] = e.Value
	}

	wants := map[string]string{
		"VIBRATOR_INTEGRATION_MODE_SERENA":     "host",
		"VIBRATOR_INTEGRATION_MODE_CLAUDE_MEM": "off",
	}
	for name, val := range wants {
		if got[name] != val {
			t.Errorf("env %s = %q, want %q", name, got[name], val)
		}
	}
}

// An empty Integrations map should not emit any mode env vars (absence is
// interpreted as "auto" by the wrapper, so there's nothing to forward).
func TestBuildContainerEnv_NoIntegrationsNoModeVars(t *testing.T) {
	pin := config.Pin{Harness: "claude-code"}

	env, err := buildContainerEnv(pin, nil)
	if err != nil {
		t.Fatalf("buildContainerEnv: %v", err)
	}
	for _, e := range env {
		if len(e.Name) >= 26 && e.Name[:26] == "VIBRATOR_INTEGRATION_MODE_" {
			t.Errorf("unexpected mode env var emitted: %s", e.Name)
		}
	}
}
