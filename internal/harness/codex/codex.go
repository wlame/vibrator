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

func (codex) RequiredFeatures() []string {
	// `npm install -g` is the install path, and the binary re-execs node
	// at runtime for plugin loading.
	return []string{"node"}
}

// HostMounts wires the host's ~/.codex state into the container. Unlike
// Claude Code (whose entrypoint merges a *.host.json copy), Codex reads
// its config natively, so these mount directly onto the container's real
// config paths — no entrypoint support needed.
//
//   - auth.json is mounted READ-WRITE so an OAuth token refreshed inside
//     the container (or a fresh `codex login`) persists back to the host.
//   - config.toml is mounted READ-ONLY: it's user-authored, so a buggy
//     container can read but not corrupt it.
//
// Both are MountFileIfExists: a host that has never run Codex gets no
// mounts and Codex inside the container starts fresh.
func (codex) HostMounts(_ harness.HostMountContext) []harness.HostMount {
	return []harness.HostMount{
		{HostRel: ".codex/auth.json", ContainerRel: ".codex/auth.json", ReadOnly: false, Kind: harness.MountFileIfExists},
		{HostRel: ".codex/config.toml", ContainerRel: ".codex/config.toml", ReadOnly: true, Kind: harness.MountFileIfExists},
	}
}

// SupportsLLMProvider returns true — Codex defaults to OpenAI but can be
// pointed at any OpenAI-compatible endpoint (local Ollama, LM Studio,
// Azure OpenAI, etc.) via OPENAI_BASE_URL.
func (codex) SupportsLLMProvider() bool { return true }

// LaunchCommand returns the argv for the Codex CLI. `codex` (no args)
// opens the agent in the current workspace.
func (codex) LaunchCommand() []string { return []string{"codex"} }

// ExtraDirArgs returns nil: this harness has no flag to add extra roots,
// so vibrator just notifies the user of the mounted paths.
func (codex) ExtraDirArgs([]string) []string { return nil }

// UpdateCommand returns the argv for upgrading Codex in place. Codex
// installs via `npm install -g @openai/codex` (see Dockerfile); re-
// running install with the @latest tag picks up the newest release
// and overwrites the symlink in /usr/local/bin.
func (codex) UpdateCommand() []string {
	return []string{"npm", "install", "-g", "@openai/codex@latest"}
}

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

// PermissionBypassArgs returns Codex's bypass-approvals flag. Confirmed
// against `codex --help` (codex-cli 0.142.5, 2026-07): "Skip all
// confirmation prompts and execute commands without sandboxing." Container
// is the sandbox, so vibrator runs with it by default.
func (codex) PermissionBypassArgs() []string {
	return []string{"--dangerously-bypass-approvals-and-sandbox"}
}

func init() {
	harness.Register(New())
}
