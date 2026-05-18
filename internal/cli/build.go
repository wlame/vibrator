package cli

import (
	"github.com/spf13/cobra"
)

// buildCmd builds the Docker image without launching a container.
// Phase 1: stub. Phase 3 wires this up to internal/app.Build().
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the Docker image for the resolved workspace spec (no run)",
	Long: `Resolves the workspace spec (CLI flags > .vb pin > defaults) and runs
'docker build' to produce the image. Does not start a container.

Use --rebuild to force a no-cache rebuild.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println("vibrate build: not implemented yet (Phase 3)")
		return nil
	},
}

// dockerfileCmd generates the Dockerfile that `build` WOULD produce, without
// invoking docker. Used for inspection, debugging, and CI.
var dockerfileCmd = &cobra.Command{
	Use:   "build-dockerfile",
	Short: "Generate the Dockerfile for the resolved spec without building",
	Long: `Runs the same template pipeline as 'build' but writes the generated
Dockerfile to a path (or stdout if --out=-) instead of invoking 'docker build'.

The output is byte-deterministic for a given input spec.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println("vibrate build-dockerfile: not implemented yet (Phase 3)")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(dockerfileCmd)
}
