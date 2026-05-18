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

func init() {
	harness.Register(New())
}
