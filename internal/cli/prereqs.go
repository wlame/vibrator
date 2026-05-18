package cli

import (
	"github.com/spf13/cobra"
)

// prereqsCmd is a parent command for prerequisite-related operations:
//   - vibrate prereqs status        — probe host stacks, dump resolved wiring
//   - vibrate prereqs bootstrap ID  — run host-side setup (e.g., mint claude-mem key)
var prereqsCmd = &cobra.Command{
	Use:   "prereqs",
	Short: "Inspect and bootstrap host-side prerequisites for selected tools",
}

var prereqsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Probe the host stacks declared by selected catalog entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println("vibrate prereqs status: not implemented yet (Phase 4)")
		return nil
	},
}

var prereqsBootstrapCmd = &cobra.Command{
	Use:   "bootstrap PREREQ_ID",
	Short: "Run the host-side bootstrap for a prerequisite (no container launch)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Printf("vibrate prereqs bootstrap %s: not implemented yet (Phase 4)\n", args[0])
		return nil
	},
}

func init() {
	prereqsCmd.AddCommand(prereqsStatusCmd)
	prereqsCmd.AddCommand(prereqsBootstrapCmd)
	rootCmd.AddCommand(prereqsCmd)
}
