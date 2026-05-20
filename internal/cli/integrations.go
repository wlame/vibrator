package cli

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/wlame/vibrator/internal/integration"
)

var integrationsCmd = &cobra.Command{
	Use:   "integrations",
	Short: "Set up and manage host-side integrations",
	Long: `Interactive setup for host-side integrations that complement vibrator containers.

Subcommands jump directly to a specific integration. Running without a
subcommand shows an interactive picker that enumerates everything in
the integrations registry (built-in + user-defined TOML files).`,
	RunE: runIntegrationsPicker,
}

// Subcommands for the two built-in integrations that need special-cased
// hand-written flows. Serena uses the generic runner; claude-mem still
// has its own setup wrapper that delegates to the runner after the
// admin-config + compose-stack dance.
var integrationsCMCmd = &cobra.Command{
	Use:   "claude-mem",
	Short: "Set up the claude-mem server-beta integration",
	RunE:  runIntegrationsCM,
}

var integrationsSerenaCmd = &cobra.Command{
	Use:   "serena",
	Short: "Manage the Serena MCP host server",
	RunE:  runIntegrationsSerena,
}

func init() {
	integrationsCmd.AddCommand(integrationsCMCmd)
	integrationsCmd.AddCommand(integrationsSerenaCmd)
	rootCmd.AddCommand(integrationsCmd)
}

// runIntegrationsPicker enumerates the registry and dispatches to the
// chosen integration. Built-in integrations with hand-written flows
// (claude-mem) get routed to their dedicated handlers; everything else
// — Serena, future TOML-defined integrations — flows through the
// generic runner.
func runIntegrationsPicker(cmd *cobra.Command, _ []string) error {
	all := integration.All()
	if len(all) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No integrations registered.")
		return nil
	}

	opts := make([]huh.Option[string], 0, len(all))
	for _, i := range all {
		opts = append(opts, huh.NewOption(
			fmt.Sprintf("%s — %s", i.Name, i.Summary), i.ID,
		))
	}

	var chosen string
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Which integration would you like to manage?").
			Options(opts...).
			Value(&chosen),
	)).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
			return nil
		}
		return err
	}

	return dispatchIntegration(cmd, chosen)
}

// dispatchIntegration routes by ID. Hand-written flows for integrations
// that still need bespoke setup are listed explicitly; everything else
// (including any TOML-defined registry entry) goes through the generic
// runIntegration runner.
func dispatchIntegration(cmd *cobra.Command, id string) error {
	switch id {
	case "claude-mem":
		return runIntegrationsCM(cmd, nil)
	}
	integ, ok := integration.Get(id)
	if !ok {
		return fmt.Errorf("integration %q not registered", id)
	}
	return runIntegration(cmd, integ)
}
