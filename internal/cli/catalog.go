package cli

import (
	"github.com/spf13/cobra"
)

// catalogCmd surfaces the curated catalog of tools/plugins/skills per harness.
// Catalog entries live in catalog/<harness>/<id>.md and are loaded from an
// embed.FS at compile time.
var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "List and inspect catalog entries (plugins, MCP servers, skills) per harness",
}

var catalogListCmd = &cobra.Command{
	Use:   "list HARNESS",
	Short: "List catalog entries for a harness (claude-code, codex, opencode, pi)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Printf("vibrate catalog list %s: not implemented yet (Phase 2)\n", args[0])
		return nil
	},
}

var catalogShowCmd = &cobra.Command{
	Use:   "show ID",
	Short: "Print a catalog entry's frontmatter + prose body",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Printf("vibrate catalog show %s: not implemented yet (Phase 2)\n", args[0])
		return nil
	},
}

func init() {
	catalogCmd.AddCommand(catalogListCmd)
	catalogCmd.AddCommand(catalogShowCmd)
	rootCmd.AddCommand(catalogCmd)
}
