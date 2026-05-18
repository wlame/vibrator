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
	// the same PR as a catalog refresh.
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

func init() {
	harness.Register(New())
}
