package cli

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/wlame/vibrator/internal/app"
)

// updateCmd implements `vibrate update` — upgrades the harness's own
// CLI to its latest version, in place, without a full image rebuild.
//
// Behaviour summary (full details in app.Update's doc):
//
//   - Container exists                  →  exec the harness's update command
//                                          inside it (start the container first
//                                          if it's stopped). Change lives in
//                                          the container only — restart-safe
//                                          but lost on container removal.
//   - Container missing + image exists  →  build a tiny one-layer image (FROM
//                                          <current-image> RUN <update-cmd>) and
//                                          re-tag it with the same name. Old
//                                          image becomes dangling; cache keeps
//                                          everything below the new layer.
//   - Neither exists                    →  error — run `vibrate` first to
//                                          bootstrap the workspace.
//
// `vibrate update` takes no flags today. The harness, profile, and
// other spec come from the workspace .vb pin. To force a full
// rebuild instead, use `vibrate --rebuild`.
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the harness's CLI in the container or image (no full rebuild)",
	Long: `Updates the harness's own CLI to its latest version, in place.

Decision tree:

  - Container running             →  exec the harness's update command inside it
                                     (e.g. 'claude update' for claude-code).
  - Container exited/stopped/etc. →  start the container, then run the update.
  - Container missing + image yes →  add a 'RUN <update-cmd>' layer on top of the
                                     existing image and re-tag with the same name.
                                     The old image becomes dangling; Docker's
                                     layer cache keeps the rebuild fast.
  - Neither exists                →  error — run 'vibrate' first to bootstrap.

Container updates are NOT persisted into the image — they survive
restart but not container removal. To bake the update into the image,
remove the container and re-run 'vibrate update' (which then takes the
image-layer path). To rebuild everything from the Dockerfile, use
'vibrate --rebuild' instead.`,
	RunE: runUpdateCommand,
}

func runUpdateCommand(cmd *cobra.Command, _ []string) error {
	return app.Update(context.Background(), app.UpdateOptions{
		VibratorVersion: Version,
		Stdin:           os.Stdin,
		Stdout:          cmd.OutOrStdout(),
		Stderr:          cmd.ErrOrStderr(),
	})
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
