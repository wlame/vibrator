// Package opencode implements the SST OpenCode harness.
package opencode

import "github.com/wlame/vibrator/internal/harness"

const ID = "opencode"

type opencode struct{}

func New() harness.Harness { return opencode{} }

func (opencode) ID() string   { return ID }
func (opencode) Name() string { return "OpenCode" }

func (opencode) Dockerfile() string {
	// OpenCode publishes a prebuilt binary per-arch on GitHub Releases.
	// Pin to a recent stable version to avoid surprise breakage; bump in
	// the same PR as a extensions refresh.
	return `RUN ARCH=$(dpkg --print-architecture) && \
    OPENCODE_VERSION="0.5.0" && \
    if [ "$ARCH" = "amd64" ]; then OC_ARCH="x86_64"; else OC_ARCH="aarch64"; fi && \
    curl -fsSL --retry 3 --retry-delay 5 \
      "https://github.com/sst/opencode/releases/download/v${OPENCODE_VERSION}/opencode-linux-${OC_ARCH}.tar.gz" \
      -o opencode.tar.gz && \
    tar -xzf opencode.tar.gz opencode && \
    mv opencode /usr/local/bin/ && chmod +x /usr/local/bin/opencode && \
    rm opencode.tar.gz && \
    opencode --version`
}

func (opencode) AuthEnvVars() []string {
	// OpenCode is BYO-provider. The provider key the user has set on the
	// host determines which model gets used. Forward all common ones; the
	// in-container /connect flow handles OAuth-based providers (GitHub
	// Copilot, ChatGPT Plus, etc.) via ~/.local/share/opencode/auth.json
	// which is mounted RW from the host.
	return []string{
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"GEMINI_API_KEY",
		"GROQ_API_KEY",
		"OPENROUTER_API_KEY",
		"DEEPSEEK_API_KEY",
	}
}

func (opencode) HostConfigDir() string {
	// Note: opencode uses ~/.local/share/opencode/ for auth (XDG-style) and
	// ~/.config/opencode/ for config. We surface the parent ~/.local/share/
	// region via selective mount in docker_cmd (Phase 4).
	return "$HOME/.config/opencode"
}

func (opencode) RequiredFeatures() []string {
	// Self-contained binary — no host runtime needed.
	return nil
}

// HostMounts wires the host's OpenCode state into the container. OpenCode
// splits its state across two XDG locations, which is why HostConfigDir()
// alone is insufficient — these descriptors name the exact paths:
//
//   - ~/.local/share/opencode/auth.json (the OAuth/credential store the
//     hostprobe uses as the primary "is it installed?" signal) is mounted
//     READ-WRITE so a `/connect` login inside the container persists back.
//   - ~/.config/opencode (user config: opencode.json, agents, …) is
//     mounted READ-ONLY so the container can't corrupt user-authored
//     config.
//
// OpenCode reads these natively, so no entrypoint support is needed.
func (opencode) HostMounts(_ harness.HostMountContext) []harness.HostMount {
	return []harness.HostMount{
		{HostRel: ".local/share/opencode/auth.json", ContainerRel: ".local/share/opencode/auth.json", ReadOnly: false, Kind: harness.MountFileIfExists},
		{HostRel: ".config/opencode", ContainerRel: ".config/opencode", ReadOnly: true, Kind: harness.MountDirIfExists},
	}
}

// SupportsLLMProvider returns true — OpenCode is BYO-provider across
// ~75+ providers (Anthropic, OpenAI, Gemini, Groq, OpenRouter,
// DeepSeek, and any OpenAI-compatible endpoint).
func (opencode) SupportsLLMProvider() bool { return true }

// LLMEnvVars maps the LLM choice into OpenCode's provider env vars.
// OpenCode looks at provider-specific env vars (ANTHROPIC_API_KEY,
// OPENAI_API_KEY, etc.); it doesn't have a single unified pair like
// Codex's OPENAI_API_KEY+OPENAI_BASE_URL. The mapping below mirrors
// OpenCode's documented conventions as of May 2026.
//
// LaunchCommand returns the argv for OpenCode's TUI. `opencode` (no
// args) opens the agent in the current workspace.
func (opencode) LaunchCommand() []string { return []string{"opencode"} }

// ExtraDirArgs returns nil: this harness has no flag to add extra roots,
// so vibrator just notifies the user of the mounted paths.
func (opencode) ExtraDirArgs([]string) []string { return nil }

// UpdateCommand returns the argv for upgrading OpenCode in place.
// OpenCode is installed from a GitHub Releases tarball (see
// Dockerfile), but the binary has a built-in `opencode upgrade`
// subcommand that re-downloads the newest tarball into
// /usr/local/bin/opencode.
func (opencode) UpdateCommand() []string { return []string{"opencode", "upgrade"} }

// LLMEnvVars maps the LLM choice into OpenCode's provider env vars.
// OpenCode looks at provider-specific env vars (ANTHROPIC_API_KEY,
// OPENAI_API_KEY, GEMINI_API_KEY, …); it doesn't have a single unified
// pair like Codex's OPENAI_API_KEY+OPENAI_BASE_URL. The mapping below
// mirrors OpenCode's documented conventions.
//
// For local providers, OpenCode uses its custom-provider config in
// ~/.config/opencode/opencode.json (NOT env vars). We set the
// OpenAI-compat pair as a hint — power users still need the matching
// opencode.json snippet for provider-specific behavior.
func (opencode) LLMEnvVars(provider, _, baseURL, apiKey string) map[string]string {
	env := map[string]string{}
	switch provider {
	case "":
		return env
	case "openai", "openai-compat":
		if apiKey != "" {
			env["OPENAI_API_KEY"] = apiKey
		}
		if baseURL != "" {
			env["OPENAI_BASE_URL"] = baseURL
		}
	case "anthropic":
		if apiKey != "" {
			env["ANTHROPIC_API_KEY"] = apiKey
		}
	case "gemini":
		// OpenCode reads these provider-specific keys natively (they're
		// declared in AuthEnvVars). Map the resolved key here too so a
		// .vb that pins one of these providers injects the credential —
		// without these cases the key would only reach the container
		// when the matching var happens to be exported on the host.
		if apiKey != "" {
			env["GEMINI_API_KEY"] = apiKey
		}
	case "groq":
		if apiKey != "" {
			env["GROQ_API_KEY"] = apiKey
		}
	case "openrouter":
		if apiKey != "" {
			env["OPENROUTER_API_KEY"] = apiKey
		}
	case "deepseek":
		if apiKey != "" {
			env["DEEPSEEK_API_KEY"] = apiKey
		}
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
	}
	return env
}

func init() {
	harness.Register(New())
}
