package cli

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var integrationsCmd = &cobra.Command{
	Use:   "integrations",
	Short: "Set up and manage host-side integrations",
	Long: `Interactive setup for host-side integrations that complement vibrator containers.

Subcommands jump directly to a specific integration. Running without a
subcommand shows an interactive picker.`,
	RunE: runIntegrations,
}

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

// runIntegrations shows a huh picker and dispatches to the chosen integration.
func runIntegrations(cmd *cobra.Command, _ []string) error {
	var chosen string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Which integration would you like to set up?").
				Options(
					huh.NewOption("claude-mem — persistent memory (server-beta runtime)", "claude-mem"),
					huh.NewOption("Serena MCP — host server status & management", "serena"),
				).
				Value(&chosen),
		),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
			return nil
		}
		return err
	}

	switch chosen {
	case "claude-mem":
		return runIntegrationsCM(cmd, nil)
	case "serena":
		return runIntegrationsSerena(cmd, nil)
	}
	return nil
}
