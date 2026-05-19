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
	// Stage 3 runs as the unprivileged user (USER switched at end of
	// Stage 2), so install.sh's $HOME-based default puts claude in
	// /home/$USERNAME/.local/bin/claude — owned by that user, no /root
	// traversal needed.
	//
	// The sudo'd symlink into /usr/local/bin keeps claude on PATH for
	// every user/session (useful for root sub-invocations and for
	// scripts that don't source the user's shell rc). NOPASSWD sudo is
	// granted to the user in the user-creation block.
	return `RUN curl -fsSL --retry 3 --retry-delay 5 https://claude.ai/install.sh | bash \
 && sudo ln -sf "$HOME/.local/bin/claude" /usr/local/bin/claude \
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
	// as extensions entry deps, not harness deps.
	return nil
}

// SupportsLLMProvider returns false — Claude Code is Anthropic-only.
// Authentication is handled by AuthEnvVars (CLAUDE_CODE_OAUTH_TOKEN /
// ANTHROPIC_API_KEY); there's no provider/model decision for the wizard
// to surface.
func (claudeCode) SupportsLLMProvider() bool { return false }

// LLMEnvVars returns nil — see SupportsLLMProvider.
func (claudeCode) LLMEnvVars(_, _, _, _ string) map[string]string { return nil }

func init() {
	harness.Register(New())
}
