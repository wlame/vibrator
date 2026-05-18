package cli

import (
	"github.com/spf13/cobra"
)

// runCmd is the primary user-facing command. When `vibrate` is invoked with
// no arguments, cobra's default behaviour is to print help — but per the
// product spec, we want `vibrate` (bare) to behave like `vibrate run`
// (resolve .vb / wizard → build/start container → exec).
//
// Phase 1: stub. Phase 4 wires this up to internal/app.Run().
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Build or start the container for the current workspace and exec into it",
	Long: `Resolves the workspace configuration (CLI flags > .vb pin > defaults),
runs the interactive wizard for any unset fields, builds the image if missing,
creates or starts the container, and execs you into it.

This is also the default action when 'vibrate' is invoked with no subcommand.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println("vibrate run: not implemented yet (Phase 4)")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}
