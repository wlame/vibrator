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
	// Installed from the @earendil-works scope (the project's home since
	// its 2026 rebrand; the legacy @mariozechner package froze at 0.73.1)
	// and pinned to a verified release so image builds are reproducible.
	// UpdateCommand tracks @latest on the same scope, so `vibrate update`
	// can advance past the pin in place.
	//
	// `pi --version` runs WITHOUT a `|| true` fallthrough on purpose: if
	// the install produces no working `pi` binary (e.g. the bin name
	// changes again) the build must fail here rather than bake a broken
	// image that only errors when the user launches. This matches
	// claude-code and codex, which also verify their binary.
	return `RUN npm install -g @earendil-works/pi-coding-agent@0.80.6 \
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
// everything under one tree (~/.pi/agent/: mcp.json, settings.json,
// auth.json, sessions/, bin/, themes/, ...), which vibrator's extensions
// also bake into at image build. The host tree therefore mounts as a
// READ-ONLY SIDECAR at ~/.pi.host, not over the real ~/.pi — a direct
// mount would shadow every baked artifact (and, mounted rw as it once
// was, let the container corrupt the host's real .pi). At container
// startup, pi-materialize.sh (entrypoint-gated) copies the baked
// snapshot over ~/.pi, copies the sidecar's files over that (host wins
// per-file; agent/bin excluded — pi's managed fd/rg binaries are
// arch-specific, the container keeps its own), and jq-merges
// agent/mcp.json and agent/settings.json (host wins per-key; the
// settings packages array is unioned).
//
// Two host paths keep today's read-write behavior via granular mounts:
//
//   - ~/.pi/agent/auth.json (rw) so in-container logins persist back.
//   - ~/.pi/agent/sessions (MountDirEnsure) for cross-recreation session
//     history, parity with codex/opencode session mounts.
//
// The materializer never writes through either mount (it excludes both
// paths from its copies), so the host tree outside them is untouchable
// by the container.
func (pi) HostMounts(_ harness.HostMountContext) []harness.HostMount {
	return []harness.HostMount{
		{HostRel: ".pi", ContainerRel: ".pi.host", ReadOnly: true, Kind: harness.MountDirIfExists},
		{HostRel: ".pi/agent/auth.json", ContainerRel: ".pi/agent/auth.json", ReadOnly: false, Kind: harness.MountFileIfExists},
		{HostRel: ".pi/agent/sessions", ContainerRel: ".pi/agent/sessions", Kind: harness.MountDirEnsure},
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
// npm package (see Dockerfile); installs pin a verified release, while
// update tracks @latest on the same @earendil-works scope so the two
// call sites can never point at different packages.
func (pi) UpdateCommand() []string {
	return []string{"npm", "install", "-g", "@earendil-works/pi-coding-agent@latest"}
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

// PermissionBypassArgs returns nil — Pi's core CLI has no permission-prompt
// system to bypass in the first place (its README states plainly: "No
// permission popups. Run in a container..."). Third-party extensions add
// permission gates, but those aren't part of the harness's own CLI surface.
// If a future core release adds a native bypass flag, return it here and
// its image gets the YOLO alias for free.
func (pi) PermissionBypassArgs() []string { return nil }

// LoginFlow returns nil: Pi has no documented browser-auth flow to wire
// `vibrate --login` against — it's BYOK via env vars (see AuthEnvVars) or
// ~/.pi config, not an interactive OAuth login command. nil until upstream
// ships one.
func (pi) LoginFlow() *harness.LoginFlow { return nil }

func init() {
	harness.Register(New())
}
