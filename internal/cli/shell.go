package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/wlame/vibrator/internal/app"
)

// shellCmd implements `vibrate shell` — the escape hatch that drops
// the user into their shell inside the container rather than launching
// the harness's CLI. Everything else (build if image missing, start
// if container stopped, mount workspace, forward auth env vars) works
// exactly like `vibrate run`.
//
// Useful for:
//   - Debugging the container (poke at config files, check installed
//     packages, run one-off commands).
//   - Installing things by hand the curated extensions catalogue
//     doesn't cover yet.
//   - Driving the harness via subcommands or piped input rather than
//     its interactive TUI.
//
// All flag semantics are inherited from `vibrate run` (same flag set,
// same wizard, same .vb interaction) — only the final exec target
// changes.
var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Build/start the container and drop into the user's shell",
	Long: `Like 'vibrate run', but launches the user's shell inside the container
instead of the harness's CLI. Useful for debugging, installing extras,
or running one-off commands.

All flags from 'vibrate run' are accepted here — the only difference
between the two commands is what gets exec'd as the final step inside
the container.`,
	RunE: runShellCommand,
}

func runShellCommand(cmd *cobra.Command, _ []string) error {
	return app.Run(context.Background(), buildAppOptions(cmd, app.LaunchShell))
}

func init() {
	rootCmd.AddCommand(shellCmd)

	// Share the same flag definitions as `vibrate run` so users can
	// drop the same args either way — only the target differs.
	shellCmd.Flags().AddFlagSet(runCmd.Flags())
}
