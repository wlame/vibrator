package cli

import (
	"github.com/spf13/cobra"

	"github.com/wlame/vibrator/internal/runtime"
)

// runtimeCmd is a diagnostic command for the docker-runtime detection layer.
// Useful for users debugging "why is vibrate looking at the wrong socket"
// problems on macOS, where Docker Desktop / OrbStack / Colima / Rancher /
// Podman all use different socket paths.
var runtimeCmd = &cobra.Command{
	Use:   "runtime",
	Short: "Inspect the auto-detected Docker runtime and socket path",
}

// Flags scoped to `vibrate runtime detect`.
var (
	runtimeDetectSocketOverride string
	runtimeDetectColimaProfile  string
)

var runtimeDetectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Print the detected Docker runtime and socket path, then exit",
	Long: `Probes the host for a Docker runtime in this priority order:
  1. --docker-socket flag or $VIBRATOR_DOCKER_SOCKET
  2. $DOCKER_HOST (if it points at a unix socket)
  3. 'docker context inspect' active context
  4. Well-known socket paths (Desktop / OrbStack / Colima / Rancher / Podman / native)

The first reachable socket wins. Exits non-zero if no runtime can be found.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		got, err := runtime.Detect(runtime.Options{
			SocketOverride: runtimeDetectSocketOverride,
			ColimaProfile:  runtimeDetectColimaProfile,
		})
		if err != nil {
			return err
		}
		cmd.Printf("Runtime: %s\nSocket:  %s\nSource:  %s\n", got.Runtime, got.Socket, got.Source)
		return nil
	},
}

func init() {
	runtimeDetectCmd.Flags().StringVar(&runtimeDetectSocketOverride, "docker-socket", "",
		"Force this socket path (equivalent to $VIBRATOR_DOCKER_SOCKET)")
	runtimeDetectCmd.Flags().StringVar(&runtimeDetectColimaProfile, "colima-profile", "",
		"Colima VM profile to look under ~/.colima/<profile>/ (default: $COLIMA_PROFILE or 'default')")

	runtimeCmd.AddCommand(runtimeDetectCmd)
	rootCmd.AddCommand(runtimeCmd)
}
