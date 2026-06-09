package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	vibrator "github.com/wlame/vibrator"
	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/extensions"
	"github.com/wlame/vibrator/internal/hostprobe"
	"github.com/wlame/vibrator/internal/wizard"
)

// wizardCmd runs the interactive setup wizard standalone, without
// proceeding to build/run. Primarily a debug + UX verification surface
// while Phase 4e (the full `vibrate` orchestrator) isn't wired yet.
//
// After Phase 4e this command may be retired — the same flow will run
// automatically as part of `vibrate` when .vb is missing.
var wizardCmd = &cobra.Command{
	Use:   "wizard",
	Short: "Run the setup wizard standalone (does not build or save)",
	Long: `Runs the interactive setup wizard for the current workspace.

Loads any existing .vb in $PWD as starting state, probes the host for
installed plugins (to pre-check the right extension entries), then steps
the user through harness, profile, shell, LLM provider (where
applicable), and extensions selection.

The result is printed as a summary + equivalent CLI command. NOTHING is
written to disk and NO container is built. Use this to preview wizard
UX before running the real ` + "`vibrate`" + ` flow (Phase 4e).`,
	RunE: runWizardStandalone,
}

func init() {
	rootCmd.AddCommand(wizardCmd)
}

func runWizardStandalone(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	// Resolve workspace + existing pin (if any).
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}
	var initial config.Pin
	pinPath, err := config.FindPin(cwd)
	if err == nil {
		loaded, err := config.Load(pinPath)
		if err == nil {
			initial = *loaded
			fmt.Fprintf(cmd.ErrOrStderr(), "Loaded existing pin: %s\n", pinPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Probe the host so the extensions step can pre-check items the user
	// already has installed locally.
	home, _ := os.UserHomeDir()
	hostDetected, _ := hostprobe.ProbeAll(home)

	// Load the extensions so the wizard's extensions step has options to
	// render.
	entries, err := extensions.LoadAll(vibrator.ExtensionsFS)
	if err != nil {
		return fmt.Errorf("load extensions: %w", err)
	}

	// Run the wizard.
	result, err := wizard.Run(context.Background(), wizard.Input{
		Initial:      initial,
		WorkspaceDir: cwd,
		HostDetected: hostDetected,
		Extensions:   entries,
	})
	if err != nil {
		return err
	}
	if result.Cancelled {
		fmt.Fprintln(out, "\nWizard cancelled — nothing saved.")
		return nil
	}

	// Render summary + equivalent command. The full save-and-build
	// flow comes in Phase 4e; for now just show what the user picked.
	fmt.Fprintln(out)
	fmt.Fprintln(out, wizard.Summary(result.Pin, cwd))
	fmt.Fprintln(out, "Equivalent command:")
	fmt.Fprintln(out, wizard.EquivalentCommand(result.Pin))
	fmt.Fprintln(out)
	fmt.Fprintln(out, "(Phase 4c standalone run — nothing was saved or built.)")
	return nil
}
