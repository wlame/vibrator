package wizard

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/catalog"
	"github.com/wlame/vibrator/internal/config"
	_ "github.com/wlame/vibrator/internal/harness/all" // register built-in harnesses
	"github.com/wlame/vibrator/internal/hostprobe"
)

// --- PlanSteps ------------------------------------------------------------

func TestPlanSteps_AllUnsetShowsEverything(t *testing.T) {
	steps := PlanSteps(Input{})
	if !steps.Harness || !steps.Profile || !steps.Shell || !steps.Catalog {
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
	if !steps.Catalog {
		t.Errorf("Catalog step should always be true, got false")
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

// --- EquivalentCommand ----------------------------------------------------

func TestEquivalentCommand_FullPin(t *testing.T) {
	got := EquivalentCommand(config.Pin{
		Harness: "claude-code",
		Profile: "full",
		Shell:   "zsh",
		With:    []string{"playwright"},
		No:      []string{"aider"},
		Catalog: []string{"context7", "claude-mem", "serena"},
	})
	// Must contain all the flags in stable order.
	mustContain(t, got, "--harness=claude-code")
	mustContain(t, got, "--profile=full")
	mustContain(t, got, "--shell=zsh")
	mustContain(t, got, "--with=playwright")
	mustContain(t, got, "--no=aider")
	// Catalog must be sorted regardless of insertion order.
	mustContain(t, got, "--catalog=claude-mem,context7,serena")
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
		Catalog: []string{"github", "linear"},
	}, "/Users/wlame/dev/x")
	mustContain(t, got, "Workspace: /Users/wlame/dev/x")
	mustContain(t, got, "Harness:   codex")
	mustContain(t, got, "LLM:       openai / gpt-4o")
	mustContain(t, got, "$OPENAI_API_KEY")
	mustContain(t, got, "Catalog:   github, linear")
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

// --- kindBindings ---------------------------------------------------------

func TestKindBindings_AppendAndFlatten(t *testing.T) {
	b := &kindBindings{}
	b.appendByKind(catalog.KindPlugin, "p1")
	b.appendByKind(catalog.KindMCP, "m1")
	b.appendByKind(catalog.KindSkill, "s1")
	b.appendByKind(catalog.KindMCP, "m2")
	b.appendByKind(catalog.KindTool, "t1")

	// flatten preserves Kind order: plugin, skill, mcp, subagent, tool.
	got := b.flatten()
	want := []string{"p1", "s1", "m1", "m2", "t1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("flatten order = %v, want %v", got, want)
	}
}

func TestKindBindings_DedupesAcrossKinds(t *testing.T) {
	b := &kindBindings{}
	b.appendByKind(catalog.KindPlugin, "dup")
	b.appendByKind(catalog.KindSkill, "dup")
	got := b.flatten()
	if len(got) != 1 || got[0] != "dup" {
		t.Errorf("expected single 'dup' after dedupe, got %v", got)
	}
}

// --- preCheckedCatalogIDs -------------------------------------------------

func TestPreCheckedCatalogIDs_MapsViaCatalog(t *testing.T) {
	entries := map[string]*catalog.Entry{
		"claude-code/claude-mem": {
			Harness: "claude-code", ID: "claude-mem",
			Kind: catalog.KindPlugin, Name: "x", Source: "x",
		},
		"claude-code/playwright-mcp": {
			Harness: "claude-code", ID: "playwright-mcp",
			Kind: catalog.KindMCP, Name: "x", Source: "x",
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
	got := preCheckedCatalogIDs("claude-code", entries, detected)
	if !got["claude-mem"] {
		t.Errorf("claude-mem should be pre-checked via direct ID match")
	}
	if !got["playwright-mcp"] {
		t.Errorf("playwright-mcp should be pre-checked via host alias")
	}
}

func TestPreCheckedCatalogIDs_EmptyWhenHarnessNotInstalled(t *testing.T) {
	entries := map[string]*catalog.Entry{
		"claude-code/foo": {Harness: "claude-code", ID: "foo", Kind: catalog.KindPlugin, Name: "x", Source: "x"},
	}
	detected := map[string]hostprobe.Detected{
		"claude-code": {HarnessID: "claude-code", Installed: false},
	}
	got := preCheckedCatalogIDs("claude-code", entries, detected)
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
