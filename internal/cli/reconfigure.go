package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	vibrator "github.com/wlame/vibrator"
	"github.com/wlame/vibrator/internal/app"
	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/extensions"
	"github.com/wlame/vibrator/internal/hostprobe"
	"github.com/wlame/vibrator/internal/wizard"
)

type reconfigureFlags struct {
	// keepContainer leaves the old container running/stopped rather than
	// removing it. Useful when the user wants to switch back without rebuilding.
	keepContainer bool

	// dryRun runs the wizard and shows the new configuration but does not
	// write .vb or touch any container.
	dryRun bool
}

var reconfigureFlagsState reconfigureFlags

// reconfigureCmd re-runs the setup wizard for an existing workspace and
// rebuilds the container with the new selections. All credentials and
// tokens stored under [prereqs.*] in .vb are preserved.
var reconfigureCmd = &cobra.Command{
	Use:     "reconfigure",
	Aliases: []string{"reconfig"},
	Short:   "Re-run the setup wizard and rebuild the container",
	Long: `Loads the current .vb, re-runs the interactive setup wizard so you can
choose a different harness, profile, shell, or set of extensions, then
rebuilds the container from scratch with the new selections.

Credentials and tokens stored in .vb (under [prereqs.*]) and any custom
env or feature toggles (with/no) are preserved — only the wizard-controlled
fields (harness, profile, shell, extensions, LLM, integrations) are updated.

The old container is removed by default. Pass --keep-container to leave
it running or stopped so you can switch back without rebuilding.

Pass --dry-run to walk through the wizard and preview the new configuration
without writing to disk or touching any containers.`,
	RunE: runReconfigureCommand,
}

func runReconfigureCommand(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	out := cmd.ErrOrStderr()

	// 1. Require an existing .vb — reconfigure is a modification flow, not
	//    a first-time setup. Point the user at `vibrate` if they haven't set
	//    up the workspace yet.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}
	pinPath, err := config.FindPin(cwd)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("no .vb found — run `vibrate` first to create the workspace pin")
	}
	if err != nil {
		return err
	}
	wsDir := filepath.Dir(pinPath)

	// 2. Load existing pin so we can preserve credentials and custom tweaks.
	oldPin, err := config.Load(pinPath)
	if err != nil {
		return fmt.Errorf("load %s: %w", pinPath, err)
	}
	fmt.Fprintf(out, "→ Loaded pin: %s\n", pinPath)

	// 3. Snapshot the old container name before the spec changes. The
	//    fingerprint encodes harness + features + extensions, so any
	//    meaningful selection change produces a different name.
	oldContainerName, err := app.PinContainerName(*oldPin, wsDir)
	if err != nil {
		// Non-fatal — we warn and skip the container removal step rather
		// than blocking the user from reconfiguring.
		fmt.Fprintf(out, "  warning: couldn't compute old container name (%v) — skipping removal\n", err)
		oldContainerName = ""
	}

	// 4. Build the wizard seed: preserve tokens + custom workspace tweaks,
	//    clear all fields the wizard is responsible for (harness, profile,
	//    shell, extensions, LLM, integrations). wizard.Run starts with
	//    `pin := in.Initial` and only overwrites the fields it asks about,
	//    so Prereqs and Env flow through untouched by design.
	seed := config.Pin{
		Prereqs: oldPin.Prereqs, // minted API keys, project/team IDs, etc.
		Env:     oldPin.Env,     // custom host→container env forwarding
		With:    oldPin.With,    // per-workspace feature additions
		No:      oldPin.No,      // per-workspace feature removals
		// Harness, Profile, Shell, Extensions, LLM, Integrations: all zero
		// so the wizard asks about them from scratch.
	}

	// 5. Run the wizard with the seed as starting state.
	fmt.Fprintf(out, "→ Credentials and tokens from .vb are preserved\n\n")
	home, _ := os.UserHomeDir()
	detected, _ := hostprobe.ProbeAll(home)
	entries, err := extensions.LoadAll(vibrator.ExtensionsFS)
	if err != nil {
		return fmt.Errorf("load extensions: %w", err)
	}
	result, err := wizard.Run(ctx, wizard.Input{
		Initial:      seed,
		WorkspaceDir: wsDir,
		HostDetected: detected,
		Extensions:   entries,
	})
	if err != nil {
		return fmt.Errorf("wizard: %w", err)
	}
	if result.Cancelled {
		fmt.Fprintln(out, "\nWizard cancelled — nothing changed.")
		return nil
	}

	newPin := result.Pin

	// 6. Show what the user picked so they can verify before anything writes.
	fmt.Fprintln(out)
	fmt.Fprintln(out, wizard.Summary(newPin, wsDir))
	fmt.Fprintln(out, "Equivalent command (skip wizard next time):")
	fmt.Fprintln(out, wizard.EquivalentCommand(newPin))
	fmt.Fprintln(out)

	if reconfigureFlagsState.dryRun {
		fmt.Fprintln(out, "→ --dry-run: nothing written or rebuilt.")
		return nil
	}

	// 7. Save the updated pin. Credentials from [prereqs.*] are included
	//    in newPin (carried through the seed) and written back verbatim.
	if err := config.Save(pinPath, &newPin); err != nil {
		return fmt.Errorf("save %s: %w", pinPath, err)
	}
	fmt.Fprintf(out, "→ Saved updated pin: %s\n", pinPath)

	// 8. Handle the old container. By default we remove it — it was built
	//    for a spec that no longer matches the workspace pin, so leaving it
	//    running would be confusing. --keep-container opts out of this.
	if !reconfigureFlagsState.keepContainer && oldContainerName != "" {
		if err := removeOldContainer(ctx, out, oldContainerName); err != nil {
			// Non-fatal: warn but continue. A dangling old container doesn't
			// block the new build.
			fmt.Fprintf(out, "  warning: couldn't remove old container: %v\n", err)
		}
	} else if reconfigureFlagsState.keepContainer && oldContainerName != "" {
		fmt.Fprintf(out, "→ --keep-container: old container %s left as-is\n", oldContainerName)
	}

	// 9. Build and launch with the new spec. NoWizard=true skips the
	//    wizard (we already ran it above and saved the result). Rebuild=true
	//    forces docker build --no-cache so the new extension selection is
	//    baked in fresh.
	fmt.Fprintln(out, "→ Building new container ...")
	return app.Run(ctx, app.Options{
		NoWizard:        true,
		Rebuild:         true,
		VibratorVersion: Version,
		Stdout:          cmd.OutOrStdout(),
		Stderr:          cmd.ErrOrStderr(),
		Stdin:           os.Stdin,
	})
}

// removeOldContainer stops (if running) and removes the named container.
// Prints a status line before removing.
func removeOldContainer(ctx context.Context, out io.Writer, name string) error {
	dc, err := docker.NewCLIClient()
	if err != nil {
		return fmt.Errorf("connect to docker: %w", err)
	}
	status, err := dc.ContainerStatus(ctx, name)
	if err != nil || status == "" {
		// Container doesn't exist or can't be inspected — nothing to do.
		return nil
	}
	fmt.Fprintf(out, "→ Removing old container %s (%s) ...\n", name, status)
	return dc.Remove(ctx, docker.RemoveContainer, name, true)
}

func init() {
	reconfigureCmd.Flags().BoolVar(&reconfigureFlagsState.keepContainer, "keep-container", false,
		"Leave the old container running or stopped instead of removing it.")
	reconfigureCmd.Flags().BoolVar(&reconfigureFlagsState.dryRun, "dry-run", false,
		"Walk through the wizard and preview the new configuration without saving or rebuilding.")
	rootCmd.AddCommand(reconfigureCmd)
}
