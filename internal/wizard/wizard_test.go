package wizard

import (
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/extensions"
	_ "github.com/wlame/vibrator/internal/harness/all" // register built-in harnesses
	"github.com/wlame/vibrator/internal/hostprobe"
)

// --- PlanSteps ------------------------------------------------------------

func TestPlanSteps_AllUnsetShowsEverything(t *testing.T) {
	steps := PlanSteps(Input{})
	if !steps.Harness || !steps.Profile || !steps.Shell || !steps.Extensions {
		t.Errorf("expected all steps true on empty pin, got %+v", steps)
	}
	// LLM step is true initially (harness unknown — decided at runtime).
	if !steps.LLM {
		t.Errorf("expected LLM step true when harness unset, got false")
	}
}

func TestPlanSteps_SkipsAlreadySetFields(t *testing.T) {
	steps := PlanSteps(Input{
		Initial: config.Pin{
			Harness: "claude-code",
			Profile: "full",
			Shell:   "zsh",
		},
	})
	if steps.Harness || steps.Profile || steps.Shell {
		t.Errorf("expected harness/profile/shell skipped, got %+v", steps)
	}
	if !steps.Extensions {
		t.Errorf("Extensions step should always be true, got false")
	}
}

func TestPlanSteps_LLM_SkippedForClaudeCode(t *testing.T) {
	// claude-code returns SupportsLLMProvider()=false, so the LLM step
	// should be hidden when the harness is locked in as claude-code.
	steps := PlanSteps(Input{Initial: config.Pin{Harness: "claude-code"}})
	if steps.LLM {
		t.Errorf("LLM step should be hidden for claude-code, got true")
	}
}

func TestPlanSteps_LLM_ShownForCodex(t *testing.T) {
	steps := PlanSteps(Input{Initial: config.Pin{Harness: "codex"}})
	if !steps.LLM {
		t.Errorf("LLM step should show for codex, got false")
	}
}

func TestPlanSteps_LLM_SkippedWhenAlreadyPinned(t *testing.T) {
	steps := PlanSteps(Input{
		Initial: config.Pin{
			Harness: "codex",
			LLM:     &config.LLMSpec{Provider: "openai", Model: "gpt-4o"},
		},
	})
	if steps.LLM {
		t.Errorf("LLM step should be hidden when pin already has [llm], got true")
	}
}

// --- PlanSteps: Serena hosting --------------------------------------------

func TestPlanSteps_SerenaHosting_ShownWhenHarnessUnset(t *testing.T) {
	// Harness undecided — defer the gate to the form's HideFunc, so the
	// step is planned.
	steps := PlanSteps(Input{})
	if !steps.SerenaHosting {
		t.Errorf("SerenaHosting should be planned when harness is unset, got false")
	}
}

func TestPlanSteps_SerenaHosting_ShownForClaudeCode(t *testing.T) {
	steps := PlanSteps(Input{Initial: config.Pin{Harness: "claude-code"}})
	if !steps.SerenaHosting {
		t.Errorf("SerenaHosting should show for claude-code, got false")
	}
}

func TestPlanSteps_SerenaHosting_HiddenForNonSerenaHarness(t *testing.T) {
	steps := PlanSteps(Input{Initial: config.Pin{Harness: "codex"}})
	if steps.SerenaHosting {
		t.Errorf("SerenaHosting should be hidden for codex, got true")
	}
}

func TestPlanSteps_SerenaHosting_HiddenWhenAlreadyPinned(t *testing.T) {
	steps := PlanSteps(Input{
		Initial: config.Pin{
			Harness:      "claude-code",
			Integrations: map[string]string{"serena": "host"},
		},
	})
	if steps.SerenaHosting {
		t.Errorf("SerenaHosting should be hidden when already pinned, got true")
	}
}

// --- EquivalentCommand ----------------------------------------------------

func TestEquivalentCommand_FullPin(t *testing.T) {
	got := EquivalentCommand(config.Pin{
		Harness:    "claude-code",
		Profile:    "full",
		Shell:      "zsh",
		With:       []string{"playwright"},
		No:         []string{"aider"},
		Extensions: []string{"context7", "claude-mem", "serena"},
	})
	// Must contain all the flags in stable order.
	mustContain(t, got, "--harness=claude-code")
	mustContain(t, got, "--profile=full")
	mustContain(t, got, "--shell=zsh")
	mustContain(t, got, "--with=playwright")
	mustContain(t, got, "--no=aider")
	// Extensions must be sorted regardless of insertion order.
	mustContain(t, got, "--extensions=claude-mem,context7,serena")
}

func TestEquivalentCommand_WithEnvAuth(t *testing.T) {
	got := EquivalentCommand(config.Pin{
		Harness: "codex",
		LLM: &config.LLMSpec{
			Provider: "openai",
			Model:    "gpt-4o",
			Auth:     &config.LLMAuth{Env: "OPENAI_API_KEY"},
		},
	})
	mustContain(t, got, "--llm-provider=openai")
	mustContain(t, got, "--llm-model=gpt-4o")
	mustContain(t, got, "--llm-auth-env=OPENAI_API_KEY")
	if strings.Contains(got, "pasted") {
		t.Errorf("env-auth path should not mention pasted-key note: %s", got)
	}
}

func TestEquivalentCommand_WithPastedKey_NoteAdded(t *testing.T) {
	got := EquivalentCommand(config.Pin{
		Harness: "codex",
		LLM: &config.LLMSpec{
			Provider: "openai",
			Model:    "gpt-4o",
			Auth:     &config.LLMAuth{Value: "sk-secret"},
		},
	})
	if strings.Contains(got, "sk-secret") {
		t.Errorf("EquivalentCommand must NEVER reveal pasted keys: %s", got)
	}
	mustContain(t, got, "API key was pasted")
}

func TestEquivalentCommand_LocalProvider_NoAuthFlags(t *testing.T) {
	got := EquivalentCommand(config.Pin{
		Harness: "pi",
		LLM: &config.LLMSpec{
			Provider: "ollama",
			Model:    "qwen3:32b",
			BaseURL:  "http://host.docker.internal:11434",
		},
	})
	mustContain(t, got, "--llm-provider=ollama")
	mustContain(t, got, "--llm-model=qwen3:32b")
	mustContain(t, got, "--llm-base-url=http://host.docker.internal:11434")
	if strings.Contains(got, "--llm-auth-") {
		t.Errorf("local providers should have no auth flag, got %s", got)
	}
}

func TestEquivalentCommand_NoYolo(t *testing.T) {
	got := EquivalentCommand(config.Pin{
		Harness: "claude-code",
		NoYolo:  true,
	})
	mustContain(t, got, "--no-yolo")
}

// --- Summary --------------------------------------------------------------

func TestSummary_RendersAllSections(t *testing.T) {
	got := Summary(config.Pin{
		Harness: "codex",
		Profile: "backend",
		Shell:   "zsh",
		LLM: &config.LLMSpec{
			Provider: "openai", Model: "gpt-4o",
			Auth: &config.LLMAuth{Env: "OPENAI_API_KEY"},
		},
		Extensions: []string{"github", "linear"},
	}, "/Users/wlame/dev/x")
	mustContain(t, got, "Workspace: /Users/wlame/dev/x")
	mustContain(t, got, "Harness:   codex")
	mustContain(t, got, "LLM:       openai / gpt-4o")
	mustContain(t, got, "$OPENAI_API_KEY")
	mustContain(t, got, "Extensions:   github, linear")
}

func TestSummary_PastedKeyIsNotShown(t *testing.T) {
	got := Summary(config.Pin{
		Harness: "codex",
		LLM: &config.LLMSpec{
			Provider: "openai", Model: "gpt-4o",
			Auth: &config.LLMAuth{Value: "sk-secret-key"},
		},
	}, "")
	if strings.Contains(got, "sk-secret-key") {
		t.Errorf("Summary leaked pasted key: %s", got)
	}
	mustContain(t, got, "pasted")
}

// --- commitLLM ------------------------------------------------------------

func TestCommitLLM_PresetModelWinsOverCustom(t *testing.T) {
	pin := &config.Pin{LLM: &config.LLMSpec{Provider: "openai", Auth: &config.LLMAuth{Env: "X"}}}
	b := &llmBindings{ModelChoice: "gpt-4o", ModelCustom: "ignored"}
	commitLLM(pin, b)
	if pin.LLM.Model != "gpt-4o" {
		t.Errorf("preset should win, got %q", pin.LLM.Model)
	}
}

func TestCommitLLM_CustomWhenPresetIsEmpty(t *testing.T) {
	pin := &config.Pin{LLM: &config.LLMSpec{Provider: "openai", Auth: &config.LLMAuth{Env: "X"}}}
	b := &llmBindings{ModelChoice: "", ModelCustom: " my-custom-model "}
	commitLLM(pin, b)
	if pin.LLM.Model != "my-custom-model" {
		t.Errorf("custom (trimmed) should win, got %q", pin.LLM.Model)
	}
}

func TestCommitLLM_ClearsUnusedAuthField(t *testing.T) {
	// Simulate a user who toggled between auth methods mid-form, leaving
	// both Env and Value populated. commit should clear the unused one.
	pin := &config.Pin{LLM: &config.LLMSpec{
		Provider: "openai",
		Auth:     &config.LLMAuth{Env: "OPENAI_API_KEY", Value: "sk-leftover"},
	}}
	commitLLM(pin, &llmBindings{AuthMethod: "env", ModelChoice: "gpt-4o"})
	if pin.LLM.Auth.Value != "" {
		t.Errorf("env path should clear Value, got %q", pin.LLM.Auth.Value)
	}
	if pin.LLM.Auth.Env != "OPENAI_API_KEY" {
		t.Errorf("env path should retain Env, got %q", pin.LLM.Auth.Env)
	}
}

func TestCommitLLM_LocalProviderClearsAuthEntirely(t *testing.T) {
	pin := &config.Pin{LLM: &config.LLMSpec{
		Provider: "ollama",
		Auth:     &config.LLMAuth{Env: "stray"}, // shouldn't be here for local
	}}
	commitLLM(pin, &llmBindings{ModelChoice: "qwen3:32b"})
	if pin.LLM.Auth != nil {
		t.Errorf("local provider should clear Auth entirely, got %+v", pin.LLM.Auth)
	}
}

func TestCommitLLM_DropsEmptyAuth(t *testing.T) {
	// If neither Env nor Value was supplied, drop the Auth struct so the
	// resulting .vb doesn't have an empty [llm.auth] section.
	pin := &config.Pin{LLM: &config.LLMSpec{Provider: "openai", Auth: &config.LLMAuth{}}}
	commitLLM(pin, &llmBindings{ModelChoice: "gpt-4o"}) // no AuthMethod
	if pin.LLM.Auth != nil {
		t.Errorf("empty Auth should be cleared, got %+v", pin.LLM.Auth)
	}
}

// The previous tests covered the kindBindings helper that fed huh's
// per-kind MultiSelect groups. That whole flow was replaced by the
// tabbed extensions_picker; selection collection is now exercised by
// extensions_picker_test.go's TestPickerModel_Collect* tests.

// --- preCheckedExtensionIDs -------------------------------------------------

func TestPreCheckedExtensionIDs_MapsViaExtensions(t *testing.T) {
	entries := map[string]*extensions.Entry{
		"claude-code/claude-mem": {
			Harness: "claude-code", ID: "claude-mem",
			Kind: extensions.KindPlugin, Name: "x", Source: "x",
		},
		"claude-code/playwright-mcp": {
			Harness: "claude-code", ID: "playwright-mcp",
			Kind: extensions.KindMCP, Name: "x", Source: "x",
			HostAliases: []string{"playwright"},
		},
	}
	detected := map[string]hostprobe.Detected{
		"claude-code": {
			HarnessID:  "claude-code",
			Installed:  true,
			PluginIDs:  []string{"claude-mem", "playwright"},
			MCPServers: []string{},
		},
	}
	got := preCheckedExtensionIDs("claude-code", entries, detected)
	if !got["claude-mem"] {
		t.Errorf("claude-mem should be pre-checked via direct ID match")
	}
	if !got["playwright-mcp"] {
		t.Errorf("playwright-mcp should be pre-checked via host alias")
	}
}

func TestPreCheckedExtensionIDs_EmptyWhenHarnessNotInstalled(t *testing.T) {
	entries := map[string]*extensions.Entry{
		"claude-code/foo": {Harness: "claude-code", ID: "foo", Kind: extensions.KindPlugin, Name: "x", Source: "x"},
	}
	detected := map[string]hostprobe.Detected{
		"claude-code": {HarnessID: "claude-code", Installed: false},
	}
	got := preCheckedExtensionIDs("claude-code", entries, detected)
	if len(got) != 0 {
		t.Errorf("expected empty pre-check set when harness not installed, got %v", got)
	}
}

// --- helper ---------------------------------------------------------------

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("output missing %q\n--- output ---\n%s", needle, haystack)
	}
}
