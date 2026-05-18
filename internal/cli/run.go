package cli

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/wlame/vibrator/internal/app"
)

// runFlags holds the flag state for the run/default command.
type runFlags struct {
	harness    string
	profile    string
	shell      string
	with       []string
	no         []string
	catalog    []string
	username   string
	hostUID    int
	hostGID    int
	noWizard   bool
	noSave     bool
	rebuild    bool
}

var runFlagsState runFlags

// runCmd is the primary user-facing command. When `vibrate` is invoked
// with no arguments, cobra runs the root's PersistentPreRun + RunE — we
// wire `vibrate` (bare) to behave like `vibrate run` via the rootCmd's
// RunE in init().
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Build or start the container for the current workspace and exec into it",
	Long: `Resolves the workspace configuration (CLI flags > .vb pin > defaults),
runs the interactive wizard for any unset fields, builds the image if missing,
creates or starts the container, and execs you into it.

This is also the default action when 'vibrate' is invoked with no subcommand.`,
	RunE: runRunCommand,
}

func runRunCommand(cmd *cobra.Command, _ []string) error {
	return app.Run(context.Background(), buildAppOptions(cmd))
}

// buildAppOptions translates the run flag state into an app.Options
// struct. Shared between `vibrate run` (explicit) and the root command
// (bare invocation default).
func buildAppOptions(cmd *cobra.Command) app.Options {
	return app.Options{
		Harness:         runFlagsState.harness,
		Profile:         runFlagsState.profile,
		Shell:           runFlagsState.shell,
		With:            runFlagsState.with,
		No:              runFlagsState.no,
		CatalogIDs:      runFlagsState.catalog,
		Username:        runFlagsState.username,
		HostUID:         runFlagsState.hostUID,
		HostGID:         runFlagsState.hostGID,
		NoWizard:        runFlagsState.noWizard,
		NoSave:          runFlagsState.noSave,
		Rebuild:         runFlagsState.rebuild,
		VibratorVersion: Version,
		Stdout:          cmd.OutOrStdout(),
		Stderr:          cmd.ErrOrStderr(),
		Stdin:           os.Stdin,
	}
}

func init() {
	// Spec flags — mirror build.go.
	runCmd.Flags().StringVar(&runFlagsState.harness, "harness", "",
		"Agent harness to install (claude-code, codex, opencode, pi).")
	runCmd.Flags().StringVar(&runFlagsState.profile, "profile", "",
		"Base profile: minimal, backend, frontend, full.")
	runCmd.Flags().StringVar(&runFlagsState.shell, "shell", "",
		"Default shell: bash, zsh, fish.")
	runCmd.Flags().StringSliceVar(&runFlagsState.with, "with", nil,
		"Features to enable on top of the profile.")
	runCmd.Flags().StringSliceVar(&runFlagsState.no, "no", nil,
		"Features to disable on top of the profile.")
	runCmd.Flags().StringSliceVar(&runFlagsState.catalog, "catalog", nil,
		"Catalog entry IDs to install.")
	runCmd.Flags().StringVar(&runFlagsState.username, "username", "",
		"Unprivileged user created inside the container.")
	runCmd.Flags().IntVar(&runFlagsState.hostUID, "host-uid", 0,
		"Host UID baked at build (default: os.Getuid()).")
	runCmd.Flags().IntVar(&runFlagsState.hostGID, "host-gid", 0,
		"Host GID baked at build (default: os.Getgid()).")
	// Orchestrator-only flags.
	runCmd.Flags().BoolVar(&runFlagsState.noWizard, "no-wizard", false,
		"Skip the interactive wizard; fail if required fields are unset.")
	runCmd.Flags().BoolVar(&runFlagsState.noSave, "no-save", false,
		"Don't write the wizard's result to .vb.")
	runCmd.Flags().BoolVar(&runFlagsState.rebuild, "rebuild", false,
		"Force a fresh `docker build` even when a matching image exists.")

	rootCmd.AddCommand(runCmd)

	// Wire the bare `vibrate` invocation to behave like `vibrate run`.
	// We do this after both commands have init'd by setting RunE on
	// rootCmd. cobra resolves bare-command RunE before printing help
	// when no subcommand is matched.
	rootCmd.RunE = runRunCommand
	// Share flag definitions so `vibrate --harness=...` works the same
	// as `vibrate run --harness=...`.
	rootCmd.Flags().AddFlagSet(runCmd.Flags())
}
