// Package claudecode implements the Claude Code harness.
package claudecode

import "github.com/wlame/vibrator/internal/harness"

// ID is exported so other packages can refer to "the claude-code harness"
// without typo risk.
const ID = "claude-code"

type claudeCode struct{}

// New returns the singleton. The harness has no per-instance state.
func New() harness.Harness { return claudeCode{} }

func (claudeCode) ID() string   { return ID }
func (claudeCode) Name() string { return "Claude Code" }

func (claudeCode) Dockerfile() string {
	// Claude installs to ~/.local/bin/claude — running as root, that's
	// /root/.local/bin/claude. We symlink to /usr/local/bin so the binary
	// is on PATH for every user (the unprivileged user we create later).
	return `RUN curl -fsSL --retry 3 --retry-delay 5 https://claude.ai/install.sh | bash \
 && ln -sf /root/.local/bin/claude /usr/local/bin/claude \
 && claude --version`
}

func (claudeCode) AuthEnvVars() []string {
	// CLAUDE_CODE_OAUTH_TOKEN is the preferred long-lived auth (browser-OAuth
	// session token from `claude setup-token`). ANTHROPIC_API_KEY is the
	// alternative API-key flow. Forward both; whichever the user has set
	// gets used by the binary.
	return []string{
		"CLAUDE_CODE_OAUTH_TOKEN",
		"ANTHROPIC_API_KEY",
	}
}

func (claudeCode) HostConfigDir() string {
	// Claude Code's per-user config and plugin install location. Vibrator
	// selectively mounts settings.json, rules/, hooks/, projects/, etc.
	return "$HOME/.claude"
}

func (claudeCode) RequiredFeatures() []string {
	// Claude itself is a single binary — no runtime deps. But its
	// integration with claude-mem plugin walks Python (uv for serena MCP)
	// and Node (claude-mem plugin scripts re-exec into bun); those come in
	// as catalog entry deps, not harness deps.
	return nil
}

func init() {
	harness.Register(New())
}
