// Package pi implements the Pi (earendil-works/pi by Mario Zechner) harness.
package pi

import "github.com/wlame/vibrator/internal/harness"

const ID = "pi"

type pi struct{}

func New() harness.Harness { return pi{} }

func (pi) ID() string   { return ID }
func (pi) Name() string { return "Pi (pi-coding-agent)" }

func (pi) Dockerfile() string {
	// pi-coding-agent ships as an npm package. RequiredFeatures pulls in node.
	return `RUN npm install -g @mariozechner/pi-coding-agent \
 && pi --version || true`
}

func (pi) AuthEnvVars() []string {
	// Pi is BYOK and provider-agnostic; supports 20+ providers. Forward
	// the same broad set as opencode plus a few pi-specific ones.
	return []string{
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"GEMINI_API_KEY",
		"GROQ_API_KEY",
		"OPENROUTER_API_KEY",
		"XAI_API_KEY",
		"DEEPSEEK_API_KEY",
		"HF_TOKEN",
	}
}

func (pi) HostConfigDir() string {
	return "$HOME/.pi"
}

func (pi) RequiredFeatures() []string {
	return []string{"node"}
}

// SupportsLLMProvider returns true — Pi is provider-agnostic via
// OpenAI-compatible endpoints; users routinely point it at local
// Ollama or remote OpenAI/Anthropic via custom base URLs.
func (pi) SupportsLLMProvider() bool { return true }

// LaunchCommand returns the argv for the Pi coding agent. `pi` (no
// args) opens the agent in the current workspace using the
// configured provider.
func (pi) LaunchCommand() []string { return []string{"pi"} }

// LLMEnvVars maps the LLM choice into Pi's OpenAI-compatible env vars.
// Pi reads OPENAI_API_KEY + OPENAI_BASE_URL (plus a few provider-
// specific shortcuts for direct endpoints — kept as authEnvVars).
// Same shape as Codex; differences are deferred until Pi diverges.
func (pi) LLMEnvVars(provider, _, baseURL, apiKey string) map[string]string {
	env := map[string]string{}
	switch provider {
	case "":
		return env
	case "ollama":
		env["OPENAI_API_KEY"] = "ollama"
		if baseURL != "" {
			env["OPENAI_BASE_URL"] = baseURL + "/v1"
		}
	case "lmstudio":
		env["OPENAI_API_KEY"] = "lm-studio"
		if baseURL != "" {
			env["OPENAI_BASE_URL"] = baseURL + "/v1"
		}
	default:
		if apiKey != "" {
			env["OPENAI_API_KEY"] = apiKey
		}
		if baseURL != "" {
			env["OPENAI_BASE_URL"] = baseURL
		}
	}
	return env
}

func init() {
	harness.Register(New())
}
