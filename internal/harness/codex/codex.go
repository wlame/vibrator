// Package codex implements the OpenAI Codex harness.
package codex

import "github.com/wlame/vibrator/internal/harness"

const ID = "codex"

type codex struct{}

func New() harness.Harness { return codex{} }

func (codex) ID() string   { return ID }
func (codex) Name() string { return "OpenAI Codex" }

func (codex) Dockerfile() string {
	// @openai/codex installs the `codex` binary into /usr/local/bin via the
	// standard npm -g layout. RequiredFeatures pulls in node.
	return `RUN npm install -g @openai/codex \
 && codex --version`
}

func (codex) AuthEnvVars() []string {
	// Codex auths via OAuth (~/.codex/auth.json — vibrator mounts it RW so
	// token refresh persists back to the host) or via OPENAI_API_KEY.
	// Forward the API key as a fallback for ephemeral/CI use; OAuth is
	// preferred for interactive use.
	return []string{"OPENAI_API_KEY"}
}

func (codex) HostConfigDir() string {
	return "$HOME/.codex"
}

func (codex) RequiredFeatures() []string {
	// `npm install -g` is the install path, and the binary re-execs node
	// at runtime for plugin loading.
	return []string{"node"}
}

// SupportsLLMProvider returns true — Codex defaults to OpenAI but can be
// pointed at any OpenAI-compatible endpoint (local Ollama, LM Studio,
// Azure OpenAI, etc.) via OPENAI_BASE_URL.
func (codex) SupportsLLMProvider() bool { return true }

// LaunchCommand returns the argv for the Codex CLI. `codex` (no args)
// opens the agent in the current workspace.
func (codex) LaunchCommand() []string { return []string{"codex"} }

// LLMEnvVars maps the LLM choice into Codex's OPENAI_API_KEY +
// OPENAI_BASE_URL convention. Codex speaks OpenAI's HTTP API, so all
// providers (including Anthropic, Ollama, LM Studio) are reached via
// the same env-var shape — the user is responsible for ensuring the
// chosen endpoint actually exposes a compatible surface.
//
// Local providers (ollama, lmstudio) accept any non-empty key string;
// we send the provider id literal ("ollama" / "lm-studio") so it shows
// up usefully in upstream logs.
func (codex) LLMEnvVars(provider, _, baseURL, apiKey string) map[string]string {
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
		// openai, anthropic, openai-compat — all expect a real key.
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
