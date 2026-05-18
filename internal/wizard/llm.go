package wizard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/huh"

	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/harness"
	"github.com/wlame/vibrator/internal/localprovider"
)

// Provider identifiers persisted under [llm].provider in `.vb`. The set
// is closed because each value drives container-env mapping (Phase 4e);
// adding a new one means teaching every harness's LLMEnvVars to
// recognize it.
const (
	providerOpenAI       = "openai"
	providerAnthropic    = "anthropic"
	providerOllama       = "ollama"
	providerLMStudio     = "lmstudio"
	providerOpenAICompat = "openai-compat"
)

// commonModels returns a small curated list of model identifiers for
// the given provider, used to populate the wizard's model picker as a
// starting suggestion. Users can always type their own name via the
// "custom" option.
//
// Keeping these in code (vs. a config file) is intentional: the list
// changes with model releases and updating Go source is the most
// auditable path. We don't need to be exhaustive — power users know
// the model they want.
func commonModels(provider string) []huh.Option[string] {
	switch provider {
	case providerOpenAI, providerOpenAICompat:
		return []huh.Option[string]{
			huh.NewOption("gpt-4o", "gpt-4o"),
			huh.NewOption("gpt-4o-mini", "gpt-4o-mini"),
			huh.NewOption("o1", "o1"),
			huh.NewOption("o3-mini", "o3-mini"),
			huh.NewOption("(custom — enter below)", ""),
		}
	case providerAnthropic:
		return []huh.Option[string]{
			huh.NewOption("claude-3-5-sonnet-20241022", "claude-3-5-sonnet-20241022"),
			huh.NewOption("claude-3-5-haiku-20241022", "claude-3-5-haiku-20241022"),
			huh.NewOption("claude-3-opus-20240229", "claude-3-opus-20240229"),
			huh.NewOption("(custom — enter below)", ""),
		}
	}
	return nil
}

// llmBindings holds the per-flow scratch values that huh inputs bind
// to but that aren't directly stored in config.LLMSpec — the auth
// method choice (env vs paste) and the preset/custom model split. After
// form.Run() completes, commitLLM folds these into pin.LLM.
type llmBindings struct {
	AuthMethod  string // "env" | "value"
	ModelChoice string // selected preset model OR "" meaning "(custom)"
	ModelCustom string // free-text fallback
}

// commitLLM applies post-form fixups: collapses the preset/custom model
// pair into pin.LLM.Model, and clears the unused field of pin.LLM.Auth
// based on the chosen auth method. Idempotent.
func commitLLM(pin *config.Pin, b *llmBindings) {
	if pin == nil || pin.LLM == nil {
		return
	}
	// Model: preset wins; falls back to typed custom value.
	if b.ModelChoice != "" {
		pin.LLM.Model = b.ModelChoice
	} else {
		pin.LLM.Model = strings.TrimSpace(b.ModelCustom)
	}

	// Auth: clear unused field. Local providers (no auth step) get
	// pin.LLM.Auth set to nil entirely so the TOML output omits the
	// section.
	if pin.LLM.Provider == providerOllama || pin.LLM.Provider == providerLMStudio {
		pin.LLM.Auth = nil
		return
	}
	if pin.LLM.Auth == nil {
		return
	}
	switch b.AuthMethod {
	case "env":
		pin.LLM.Auth.Value = ""
	case "value":
		pin.LLM.Auth.Env = ""
	}
	// If both ended up empty (e.g., harness doesn't support LLM and
	// the user skipped all sub-steps), drop Auth so .vb stays clean.
	if pin.LLM.Auth.Env == "" && pin.LLM.Auth.Value == "" {
		pin.LLM.Auth = nil
	}
}

// buildLLMGroups returns the huh Groups for the LLM provider sub-flow.
//
// The flow has three branches, gated via WithHideFunc on each Group:
//
//  1. **Cloud (OpenAI / Anthropic / OpenAI-compat)** — pick model
//     (preset or custom), then auth (env var name OR pasted value).
//
//  2. **Local (Ollama / LM Studio)** — confirm URL (provider default),
//     then pick from enumerated models (or free-text if unreachable).
//     No auth.
//
//  3. **Custom OpenAI-compatible endpoint** — same as cloud but the
//     user supplies the base URL.
//
// All Groups consult harness.SupportsLLMProvider() at runtime via
// WithHideFunc so the entire LLM section disappears for claude-code if
// the user happens to pick it during this run.
//
// Returns the Groups and the bindings struct; the caller must invoke
// commitLLM after form.Run() to fold bindings into pin.LLM.
func buildLLMGroups(pin *config.Pin) ([]*huh.Group, *llmBindings) {
	// Allocate the LLM struct so huh has stable Value(...) pointers to
	// bind. commitLLM cleans up unused fields after the form runs.
	if pin.LLM == nil {
		pin.LLM = &config.LLMSpec{}
	}
	if pin.LLM.Auth == nil {
		pin.LLM.Auth = &config.LLMAuth{}
	}
	b := &llmBindings{}

	// --- Group 1: provider type picker ---
	providerGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("LLM provider").
			Description("Where will the harness route model calls?").
			Options(
				huh.NewOption("Cloud — OpenAI", providerOpenAI),
				huh.NewOption("Cloud — Anthropic", providerAnthropic),
				huh.NewOption("Local — Ollama", providerOllama),
				huh.NewOption("Local — LM Studio", providerLMStudio),
				huh.NewOption("Custom OpenAI-compatible endpoint", providerOpenAICompat),
			).
			Value(&pin.LLM.Provider),
	).WithHideFunc(func() bool { return !harnessSupportsLLM(pin.Harness) })

	// --- Group 2: base URL (only for openai-compat and optionally local) ---
	urlGroup := huh.NewGroup(
		huh.NewInput().
			Title("Base URL").
			Description("OpenAI-compatible endpoint, e.g. https://my-proxy.example.com").
			Value(&pin.LLM.BaseURL),
	).WithHideFunc(func() bool {
		return !harnessSupportsLLM(pin.Harness) ||
			pin.LLM.Provider != providerOpenAICompat
	})

	// --- Group 3: model picker (cloud + openai-compat) ---
	modelPresetGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Model").
			Description("Pick a known model or choose custom to enter your own.").
			OptionsFunc(func() []huh.Option[string] {
				return commonModels(pin.LLM.Provider)
			}, &pin.LLM.Provider).
			Value(&b.ModelChoice),
	).WithHideFunc(func() bool {
		if !harnessSupportsLLM(pin.Harness) {
			return true
		}
		return pin.LLM.Provider != providerOpenAI &&
			pin.LLM.Provider != providerAnthropic &&
			pin.LLM.Provider != providerOpenAICompat
	})

	// --- Group 4: custom model input (when preset = "") ---
	modelCustomGroup := huh.NewGroup(
		huh.NewInput().
			Title("Custom model identifier").
			Description("Model name as the provider expects it.").
			Value(&b.ModelCustom),
	).WithHideFunc(func() bool {
		if !harnessSupportsLLM(pin.Harness) {
			return true
		}
		isCloud := pin.LLM.Provider == providerOpenAI ||
			pin.LLM.Provider == providerAnthropic ||
			pin.LLM.Provider == providerOpenAICompat
		// Only show when user picked "(custom)" in modelPresetGroup.
		return !isCloud || b.ModelChoice != ""
	})

	// --- Group 5: local provider URL + model enumeration ---
	localGroup := huh.NewGroup(
		huh.NewInput().
			Title("Server URL").
			Description("Defaults to the provider's canonical host.docker.internal port.").
			Value(&pin.LLM.BaseURL).
			Placeholder(localProviderDefaultURL(pin.LLM.Provider)),
		huh.NewSelect[string]().
			Title("Model").
			Description("Models found on the local server.").
			OptionsFunc(func() []huh.Option[string] {
				return enumerateLocalModels(pin.LLM.Provider, pin.LLM.BaseURL)
			}, &pin.LLM.BaseURL).
			Value(&pin.LLM.Model),
	).WithHideFunc(func() bool {
		return !harnessSupportsLLM(pin.Harness) ||
			(pin.LLM.Provider != providerOllama && pin.LLM.Provider != providerLMStudio)
	})

	// --- Group 6: auth method (cloud only) ---
	authMethodGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("API key source").
			Description("Where will the wizard get your provider API key?").
			Options(
				huh.NewOption("Use an existing environment variable (recommended)", "env"),
				huh.NewOption("Paste it now — saves to .vb (0600, gitignored)", "value"),
			).
			Value(&b.AuthMethod),
	).WithHideFunc(func() bool {
		if !harnessSupportsLLM(pin.Harness) {
			return true
		}
		return pin.LLM.Provider == providerOllama || pin.LLM.Provider == providerLMStudio
	})

	// --- Group 7: auth env-var name ---
	authEnvGroup := huh.NewGroup(
		huh.NewInput().
			Title("Environment variable name").
			Description("Will be forwarded into the container at `docker run`.").
			Value(&pin.LLM.Auth.Env).
			Placeholder(defaultAuthEnvFor(pin.LLM.Provider)),
	).WithHideFunc(func() bool {
		if !harnessSupportsLLM(pin.Harness) {
			return true
		}
		isLocal := pin.LLM.Provider == providerOllama || pin.LLM.Provider == providerLMStudio
		return isLocal || b.AuthMethod != "env"
	})

	// --- Group 8: auth pasted value ---
	authValueGroup := huh.NewGroup(
		huh.NewInput().
			Title("API key").
			Description("Stored in .vb (mode 0600). NEVER commit this file.").
			EchoMode(huh.EchoModePassword).
			Value(&pin.LLM.Auth.Value),
	).WithHideFunc(func() bool {
		if !harnessSupportsLLM(pin.Harness) {
			return true
		}
		isLocal := pin.LLM.Provider == providerOllama || pin.LLM.Provider == providerLMStudio
		return isLocal || b.AuthMethod != "value"
	})

	// Bindings live in `b`; commitLLM (called by wizard.Run after the
	// form completes) folds them into pin.LLM. We don't use a Group
	// validate hook because huh.Group has no such API in v1.0.0.

	return []*huh.Group{
		providerGroup,
		urlGroup,
		modelPresetGroup,
		modelCustomGroup,
		localGroup,
		authMethodGroup,
		authEnvGroup,
		authValueGroup,
	}, b
}

// harnessSupportsLLM reports whether the chosen harness (by ID) wants
// the LLM provider step. Used by all the WithHideFunc callbacks so the
// whole LLM section vanishes when the harness is claude-code.
func harnessSupportsLLM(harnessID string) bool {
	if harnessID == "" {
		// Harness not yet picked — show LLM section optimistically;
		// hide callback will fire again once harness is bound.
		return true
	}
	h, ok := harness.ByID(harnessID)
	if !ok {
		return false
	}
	return h.SupportsLLMProvider()
}

// localProviderDefaultURL returns the canonical URL for the local
// provider with the given ID, or empty string when the provider isn't
// local.
func localProviderDefaultURL(providerID string) string {
	p, ok := localprovider.ByID(providerID)
	if !ok {
		return ""
	}
	return p.DefaultURL()
}

// enumerateLocalModels probes the local provider for installed models.
// On failure (unreachable / parse error) returns a single fallback
// option asking the user to type a model name manually.
//
// The 1.5s timeout is intentional: we don't want a slow probe to wedge
// the wizard. If the server is healthy this completes in milliseconds.
func enumerateLocalModels(providerID, url string) []huh.Option[string] {
	p, ok := localprovider.ByID(providerID)
	if !ok {
		return nil
	}
	if url == "" {
		url = p.DefaultURL()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	models, err := p.ListLocalModels(ctx, url)
	if err != nil || len(models) == 0 {
		// Fallback: a single explanatory option so the user understands
		// why no models are listed. The value is non-empty so huh's
		// "required" validation passes; user can type a real name in
		// the input that follows.
		return []huh.Option[string]{
			huh.NewOption(fmt.Sprintf("(no models found at %s — enter manually)", url), ""),
		}
	}
	opts := make([]huh.Option[string], 0, len(models))
	for _, m := range models {
		opts = append(opts, huh.NewOption(m, m))
	}
	return opts
}

// defaultAuthEnvFor returns the conventional env var name for the
// given provider — used as a placeholder in the env-name input so users
// rarely need to type it.
func defaultAuthEnvFor(providerID string) string {
	switch providerID {
	case providerOpenAI, providerOpenAICompat:
		return "OPENAI_API_KEY"
	case providerAnthropic:
		return "ANTHROPIC_API_KEY"
	}
	return ""
}
