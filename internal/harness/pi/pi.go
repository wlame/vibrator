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

func init() {
	harness.Register(New())
}
