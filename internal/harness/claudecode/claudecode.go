// Package claudecode implements the Claude Code harness.
package claudecode

import (
	"strings"

	"github.com/wlame/vibrator/internal/harness"
)

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

// sessionPersistDirs are the per-CC subdirectories that hold in-progress
// conversations, file history, etc. Bind-mounting them means a
// container-side claude session shows up in the host's history list (and
// vice versa) — the same session continuity the bash impl gave by default.
//
// Trade-off: shared mutable state. Concurrent host + container claude runs
// could race on the same JSON files; the bash impl shipped this default-on
// for ~12 months without major complaints, so we follow.
//
// "projects" is NOT here — it's mounted at a per-workspace scope below so
// other projects' transcripts aren't visible inside the container.
var sessionPersistDirs = []string{
	"file-history",
	"sessions",
	"tasks",
	"paste-cache",
}

// encodedProjectDir converts an absolute workspace path to the
// subdirectory name Claude Code uses under ~/.claude/projects/. Claude
// Code replaces every "/" with "-", so the leading slash becomes a
// leading "-": /Users/bob/src → -Users-bob-src.
func encodedProjectDir(wsDir string) string {
	return strings.ReplaceAll(wsDir, "/", "-")
}

// HostMounts declares how host ~/.claude state is wired into the
// container. All entries are conditional (MountFileIfExists /
// MountDirIfExists) except the session-persist dirs (MountDirEnsure),
// which the orchestrator auto-creates so first-run sessions persist.
//
// The container-side entrypoint (templates/scripts/entrypoint.sh) reads
// the *.host.json / rules-host paths to seed the in-container ~/.claude/.
func (claudeCode) HostMounts(ctx harness.HostMountContext) []harness.HostMount {
	mounts := []harness.HostMount{
		// D1: ~/.claude.json → ~/.claude.host.json:ro. Entrypoint extracts
		// OAuth + onboarding fields. Read-only: the container must never
		// modify the host's master config.
		{HostRel: ".claude.json", ContainerRel: ".claude.host.json", ReadOnly: true, Kind: harness.MountFileIfExists},
		// D2: settings.json → settings.host.json:ro. Entrypoint copies with
		// a macOS-path rewrite and re-merges baked plugin hooks.
		{HostRel: ".claude/settings.json", ContainerRel: ".claude/settings.host.json", ReadOnly: true, Kind: harness.MountFileIfExists},
		// D3: rules/ → rules-host:ro. Entrypoint copies *.md from rules-host
		// on every entry so host rule edits take effect next run.
		{HostRel: ".claude/rules", ContainerRel: ".claude/rules-host", ReadOnly: true, Kind: harness.MountDirIfExists},
		// D4: hooks/ → hooks:ro. Read-only on purpose: hooks execute on the
		// HOST next session, so a writable mount would let the containerized
		// agent plant a script that runs outside the sandbox.
		{HostRel: ".claude/hooks", ContainerRel: ".claude/hooks", ReadOnly: true, Kind: harness.MountDirIfExists},
	}

	// D5a: projects/ scoped to THIS workspace's encoded-cwd subdir.
	// Mounting only the workspace-specific subdir keeps other projects'
	// conversation histories out of the container. The workspace is mounted
	// at the same absolute path on both sides, so the encoded name Claude
	// computes inside the container matches the host path exactly.
	encoded := encodedProjectDir(ctx.WorkspaceDir)
	mounts = append(mounts, harness.HostMount{
		HostRel:      ".claude/projects/" + encoded,
		ContainerRel: ".claude/projects/" + encoded,
		Kind:         harness.MountDirEnsure,
	})

	// D5b: remaining session-persist dirs (rw) — mounted wholesale since
	// they have no per-project structure.
	for _, name := range sessionPersistDirs {
		mounts = append(mounts, harness.HostMount{
			HostRel:      ".claude/" + name,
			ContainerRel: ".claude/" + name,
			Kind:         harness.MountDirEnsure,
		})
	}
	return mounts
}

// LaunchCommand returns the argv for Claude Code's CLI. Plain `claude`
// drops the user into the agent's TUI at the workspace.
func (claudeCode) LaunchCommand() []string { return []string{"claude"} }

// UpdateCommand returns the argv for upgrading Claude Code in place.
// The official installer (`claude.ai/install.sh` — see Dockerfile)
// ships a built-in `claude update` that re-downloads the latest
// release into `~/.local/bin/claude`.
func (claudeCode) UpdateCommand() []string { return []string{"claude", "update"} }

func init() {
	harness.Register(New())
}
