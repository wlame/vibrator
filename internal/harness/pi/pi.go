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
	//
	// `pi --version` runs WITHOUT a `|| true` fallthrough on purpose: if
	// the install produces no working `pi` binary (e.g. the package
	// rebrands and the bin name changes) the build must fail here rather
	// than bake a broken image that only errors when the user launches.
	// This matches claude-code and codex, which also verify their binary.
	return `RUN npm install -g @mariozechner/pi-coding-agent \
 && pi --version`
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

// HostMounts wires the host's ~/.pi state into the container. Pi keeps
// its config (agent settings, ~/.pi/agent/mcp.json) and any credential
// state under a single ~/.pi tree, so this is a coarse whole-directory
// passthrough — mounted READ-WRITE so in-container config/login changes
// persist back to the host. MountDirIfExists makes it a no-op for a host
// that has never run Pi.
//
// Coarse on purpose: Pi's on-disk layout is less settled than the other
// harnesses', so a single dir mount avoids missing a state file. Split
// into finer-grained ro/rw mounts once the layout stabilizes.
func (pi) HostMounts(_ harness.HostMountContext) []harness.HostMount {
	return []harness.HostMount{
		{HostRel: ".pi", ContainerRel: ".pi", ReadOnly: false, Kind: harness.MountDirIfExists},
	}
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

// ExtraDirArgs returns nil: this harness has no flag to add extra roots,
// so vibrator just notifies the user of the mounted paths.
func (pi) ExtraDirArgs([]string) []string { return nil }

// UpdateCommand returns the argv for upgrading Pi in place. Pi is an
// npm package (see Dockerfile); re-running install with @latest picks
// up the newest release and overwrites the global bin symlink.
//
// NOTE: Pi rebranded from `@mariozechner/pi-coding-agent` to
// `@earendil-works/pi-coding-agent` in 2026; the install command in
// Dockerfile still uses the legacy name for compatibility. Update
// here uses the SAME package name to avoid a partial migration where
// `npm update` would install the rebranded package alongside the old
// one. Switch both call sites together in a separate change.
func (pi) UpdateCommand() []string {
	return []string{"npm", "install", "-g", "@mariozechner/pi-coding-agent@latest"}
}

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
