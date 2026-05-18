package app

import (
	"reflect"
	"sort"
	"testing"

	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/docker"
	_ "github.com/wlame/vibrator/internal/harness/all" // register built-in harnesses
)

// --- needsWizard ----------------------------------------------------------

func TestNeedsWizard_TrueWhenHarnessEmpty(t *testing.T) {
	if !needsWizard(config.Pin{}) {
		t.Errorf("expected needsWizard=true on empty pin")
	}
}

func TestNeedsWizard_FalseWhenHarnessSet(t *testing.T) {
	if needsWizard(config.Pin{Harness: "claude-code"}) {
		t.Errorf("expected needsWizard=false when harness is set")
	}
}

// --- applyFlagOverrides ---------------------------------------------------

func TestApplyFlagOverrides_NonEmptyFlagsWin(t *testing.T) {
	pin := config.Pin{
		Harness: "claude-code",
		Profile: "minimal",
		Shell:   "bash",
		With:    []string{"old"},
	}
	opts := Options{
		Harness: "codex",
		Profile: "full",
		Shell:   "zsh",
		With:    []string{"playwright"},
	}
	applyFlagOverrides(&pin, opts)
	if pin.Harness != "codex" || pin.Profile != "full" || pin.Shell != "zsh" {
		t.Errorf("flags should override pin, got %+v", pin)
	}
	if !reflect.DeepEqual(pin.With, []string{"playwright"}) {
		t.Errorf("With should be replaced, got %v", pin.With)
	}
}

func TestApplyFlagOverrides_EmptyFlagsLeavePinIntact(t *testing.T) {
	pin := config.Pin{
		Harness: "claude-code",
		Profile: "full",
	}
	applyFlagOverrides(&pin, Options{})
	if pin.Harness != "claude-code" || pin.Profile != "full" {
		t.Errorf("empty flags should not clobber pin: %+v", pin)
	}
}

// --- validatePin ----------------------------------------------------------

func TestValidatePin_FailsWithoutHarness(t *testing.T) {
	if err := validatePin(config.Pin{}); err == nil {
		t.Errorf("expected error for empty harness")
	}
}

func TestValidatePin_FailsOnUnknownHarness(t *testing.T) {
	if err := validatePin(config.Pin{Harness: "not-a-real-harness"}); err == nil {
		t.Errorf("expected error for unknown harness")
	}
}

func TestValidatePin_AcceptsKnownHarness(t *testing.T) {
	if err := validatePin(config.Pin{Harness: "claude-code"}); err != nil {
		t.Errorf("claude-code should validate: %v", err)
	}
}

// --- resolveAPIKey --------------------------------------------------------

func TestResolveAPIKey_LocalProvidersReturnEmpty(t *testing.T) {
	for _, p := range []string{"ollama", "lmstudio"} {
		spec := &config.LLMSpec{Provider: p}
		got, err := resolveAPIKey(spec)
		if err != nil {
			t.Errorf("%s should not error: %v", p, err)
		}
		if got != "" {
			t.Errorf("%s should resolve to empty key, got %q", p, got)
		}
	}
}

func TestResolveAPIKey_PrefersAuthValueOverEnv(t *testing.T) {
	t.Setenv("TEST_LLM_KEY", "from-env")
	spec := &config.LLMSpec{
		Provider: "openai",
		Auth:     &config.LLMAuth{Value: "from-vb", Env: "TEST_LLM_KEY"},
	}
	got, err := resolveAPIKey(spec)
	if err != nil {
		t.Fatalf("resolveAPIKey: %v", err)
	}
	if got != "from-vb" {
		t.Errorf("expected literal value to win, got %q", got)
	}
}

func TestResolveAPIKey_FallsBackToHostEnv(t *testing.T) {
	t.Setenv("TEST_LLM_KEY_2", "from-env-fallback")
	spec := &config.LLMSpec{
		Provider: "openai",
		Auth:     &config.LLMAuth{Env: "TEST_LLM_KEY_2"},
	}
	got, err := resolveAPIKey(spec)
	if err != nil {
		t.Fatalf("resolveAPIKey: %v", err)
	}
	if got != "from-env-fallback" {
		t.Errorf("expected env value, got %q", got)
	}
}

func TestResolveAPIKey_ErrorsWhenEnvUnset(t *testing.T) {
	// Clear the var to ensure unset state.
	t.Setenv("DEFINITELY_UNSET_LLM_KEY_VIBRATE_TEST", "")
	// t.Setenv unsets at end of test; we also need it unset NOW, so
	// rely on os.Unsetenv via Setenv(""). The function reads via
	// os.Getenv which returns "" for empty AND unset values uniformly.
	spec := &config.LLMSpec{
		Provider: "openai",
		Auth:     &config.LLMAuth{Env: "DEFINITELY_UNSET_LLM_KEY_VIBRATE_TEST"},
	}
	_, err := resolveAPIKey(spec)
	if err == nil {
		t.Errorf("expected error for unset env var")
	}
}

func TestResolveAPIKey_ErrorsWhenNoAuthConfigured(t *testing.T) {
	spec := &config.LLMSpec{Provider: "openai", Auth: nil}
	_, err := resolveAPIKey(spec)
	if err == nil {
		t.Errorf("expected error when neither env nor value is set")
	}
}

// --- buildContainerEnv ----------------------------------------------------

func TestBuildContainerEnv_ClaudeCodeForwardsAuthVars(t *testing.T) {
	// Claude Code's AuthEnvVars: CLAUDE_CODE_OAUTH_TOKEN, ANTHROPIC_API_KEY.
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "tok-123")
	t.Setenv("ANTHROPIC_API_KEY", "ak-456")
	got, err := buildContainerEnv(config.Pin{Harness: "claude-code"})
	if err != nil {
		t.Fatalf("buildContainerEnv: %v", err)
	}
	envMap := envToMap(got)
	if envMap["CLAUDE_CODE_OAUTH_TOKEN"] != "tok-123" {
		t.Errorf("expected OAuth token forwarded, got %v", envMap)
	}
	if envMap["ANTHROPIC_API_KEY"] != "ak-456" {
		t.Errorf("expected ANTHROPIC_API_KEY forwarded, got %v", envMap)
	}
}

func TestBuildContainerEnv_CodexWithCloudLLM(t *testing.T) {
	got, err := buildContainerEnv(config.Pin{
		Harness: "codex",
		LLM: &config.LLMSpec{
			Provider: "openai", Model: "gpt-4o",
			Auth: &config.LLMAuth{Value: "sk-test"},
		},
	})
	if err != nil {
		t.Fatalf("buildContainerEnv: %v", err)
	}
	envMap := envToMap(got)
	if envMap["OPENAI_API_KEY"] != "sk-test" {
		t.Errorf("expected OPENAI_API_KEY=sk-test, got %q", envMap["OPENAI_API_KEY"])
	}
}

func TestBuildContainerEnv_CodexWithOllama(t *testing.T) {
	got, err := buildContainerEnv(config.Pin{
		Harness: "codex",
		LLM: &config.LLMSpec{
			Provider: "ollama", Model: "qwen3:32b",
			BaseURL: "http://host.docker.internal:11434",
		},
	})
	if err != nil {
		t.Fatalf("buildContainerEnv: %v", err)
	}
	envMap := envToMap(got)
	if envMap["OPENAI_API_KEY"] != "ollama" {
		t.Errorf("expected literal 'ollama' key, got %q", envMap["OPENAI_API_KEY"])
	}
	if envMap["OPENAI_BASE_URL"] != "http://host.docker.internal:11434/v1" {
		t.Errorf("expected /v1 suffix appended, got %q", envMap["OPENAI_BASE_URL"])
	}
}

func TestBuildContainerEnv_PinEnvOverridesWithDollarIndirection(t *testing.T) {
	t.Setenv("MY_HOST_TOKEN", "host-val")
	got, err := buildContainerEnv(config.Pin{
		Harness: "claude-code",
		Env: map[string]string{
			"LITERAL":   "literal-val",
			"INDIRECT":  "$MY_HOST_TOKEN",
		},
	})
	if err != nil {
		t.Fatalf("buildContainerEnv: %v", err)
	}
	envMap := envToMap(got)
	if envMap["LITERAL"] != "literal-val" {
		t.Errorf("literal env var lost: %v", envMap)
	}
	if envMap["INDIRECT"] != "host-val" {
		t.Errorf("$NAME indirection failed: %v", envMap)
	}
}

func TestBuildContainerEnv_PinEnvWinsOverHarnessAuth(t *testing.T) {
	// pin.Env precedence: a user override should win even over a
	// harness-declared auth env var.
	t.Setenv("ANTHROPIC_API_KEY", "from-host")
	got, _ := buildContainerEnv(config.Pin{
		Harness: "claude-code",
		Env:     map[string]string{"ANTHROPIC_API_KEY": "from-pin"},
	})
	envMap := envToMap(got)
	if envMap["ANTHROPIC_API_KEY"] != "from-pin" {
		t.Errorf("pin.Env should win over harness auth, got %v", envMap)
	}
}

func TestBuildContainerEnv_StableOrder(t *testing.T) {
	// Two calls with the same pin should produce identical (sorted) output.
	pin := config.Pin{
		Harness: "claude-code",
		Env:     map[string]string{"A": "1", "B": "2", "C": "3"},
	}
	a, _ := buildContainerEnv(pin)
	b, _ := buildContainerEnv(pin)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("buildContainerEnv not deterministic: %v vs %v", a, b)
	}
	// And the names should be sorted.
	names := make([]string, len(a))
	for i, ev := range a {
		names[i] = ev.Name
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(names, sorted) {
		t.Errorf("env vars not sorted by name: %v", names)
	}
}

// --- helper ---------------------------------------------------------------

func envToMap(vars []docker.EnvVar) map[string]string {
	m := make(map[string]string, len(vars))
	for _, e := range vars {
		m[e.Name] = e.Value
	}
	return m
}
