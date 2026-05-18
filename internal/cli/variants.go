package cli

import (
	"github.com/spf13/cobra"
)

// variantsCmd manages the set of locally-built vibrator images and their
// associated containers. An image variant is uniquely identified by the
// (harness, profile, feature_fingerprint) tuple, encoded in its tag.
var variantsCmd = &cobra.Command{
	Use:   "variants",
	Short: "List or prune locally-built vibrator image variants and containers",
}

var variantsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all vibrator-managed images with their profile/features/fingerprint",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println("vibrate variants list: not implemented yet (Phase 5)")
		return nil
	},
}

var variantsPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove unused vibrator-managed images and stopped containers",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println("vibrate variants prune: not implemented yet (Phase 5)")
		return nil
	},
}

func init() {
	variantsCmd.AddCommand(variantsListCmd)
	variantsCmd.AddCommand(variantsPruneCmd)
	rootCmd.AddCommand(variantsCmd)
}
