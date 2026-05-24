// Package cli wires up all cobra subcommands for the `vibrate` binary.
//
// The root command and each subcommand live in their own files
// (run.go, build.go, dockerfile.go, ...). Each file's init() registers
// its command with rootCmd.
package cli

import (
	"github.com/spf13/cobra"
)

// Version is overridden via -ldflags "-X github.com/wlame/vibrator/internal/cli.Version=v0.0.1"
// during release builds. The default keeps `go run` and `go install` self-describing.
var Version = "dev"

// rootCmd is the top-level cobra command for `vibrate`.
// Subcommands attach themselves via init() in their own files.
var rootCmd = &cobra.Command{
	Use:   "vibrate",
	Short: "Run AI coding agents in dockerized isolation",
	Long: `vibrate runs AI coding agents (Claude Code, Codex, OpenCode, Pi) in
isolated Docker containers per workspace, with declarative profile + extensions
configuration via .vb.

Running 'vibrate' with no subcommand builds (if needed), starts, and
launches the harness's CLI (claude / codex / opencode / pi) directly
inside the container for the current workspace. Use 'vibrate shell'
to drop into a shell instead.`,

	// SilenceUsage stops cobra from dumping the full usage text on every
	// runtime error — we'd rather show a focused error message. Errors are
	// still returned and main() handles the exit code.
	SilenceUsage:  true,
	SilenceErrors: true,

	// Version flag wiring — `vibrate --version` and `vibrate version`.
	Version: Version,
}

// Execute runs the root command. Called from cmd/vibrate/main.go.
func Execute() error {
	// Re-bind Version here so test code that mutates the package var
	// before calling Execute() sees the updated value.
	rootCmd.Version = Version
	return rootCmd.Execute()
}
