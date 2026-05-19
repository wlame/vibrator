package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	vibrator "github.com/wlame/vibrator"
	"github.com/wlame/vibrator/internal/catalog"
	"github.com/wlame/vibrator/internal/hostprobe"
	// Probers register themselves in init() within the hostprobe package;
	// no separate side-effect import needed.
)

// hostprobeCmd reports what vibrator detected on the host for each
// registered harness, plus a mapping back to known catalog entries.
//
// This is primarily a debug / diagnostic command — the same data drives
// the wizard's pre-check behavior, so running it standalone is the easy
// way to verify your wizard will start with the right boxes ticked.
var hostprobeCmd = &cobra.Command{
	Use:   "hostprobe",
	Short: "Show what vibrator detected on the host (plugins, MCPs, configs)",
	Long: `Scans the host for installed harness configs and plugins.

For each registered harness, prints:
  - whether it appears installed (config dir present)
  - which raw plugin / MCP server IDs were found on the host
  - which of those map to catalog entries (used for wizard pre-check)

Useful for confirming that the wizard will start with the right boxes
ticked, or for debugging why an expected plugin wasn't auto-detected.`,
	RunE: runHostprobe,
}

func init() {
	rootCmd.AddCommand(hostprobeCmd)
}

func runHostprobe(cmd *cobra.Command, _ []string) error {
	c := newColors(!isTerminal(cmd.OutOrStdout()))
	out := cmd.OutOrStdout()

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve $HOME: %w", err)
	}

	results, err := hostprobe.ProbeAll(home)
	if err != nil {
		// Per ProbeAll's contract: partial results may still be present.
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", err)
	}

	// Load catalog so we can show the mapping from raw host IDs to our
	// catalog entries. If catalog loading fails we still print the raw IDs.
	entries, catalogErr := catalog.LoadAll(vibrator.CatalogFS)
	if catalogErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: catalog load failed: %v\n", catalogErr)
	}

	for _, id := range hostprobe.HarnessIDs() {
		d := results[id]
		fmt.Fprintf(out, "\n%s%s%s\n", c.bold, id, c.reset)
		fmt.Fprintln(out, strings.Repeat("-", len(id)))
		fmt.Fprintf(out, "  Home dir:    %s\n", d.HomeDir)
		if d.Installed {
			fmt.Fprintf(out, "  Installed:   %s✓ yes%s\n", c.green, c.reset)
		} else {
			fmt.Fprintf(out, "  Installed:   %sno%s\n", c.dim, c.reset)
			continue
		}

		// Raw host-side ids the prober found.
		if len(d.PluginIDs) > 0 {
			fmt.Fprintf(out, "  Plugins/skills (raw, %d):\n", len(d.PluginIDs))
			for _, p := range d.PluginIDs {
				fmt.Fprintf(out, "    - %s\n", p)
			}
		}
		if len(d.MCPServers) > 0 {
			fmt.Fprintf(out, "  MCP servers (raw, %d):\n", len(d.MCPServers))
			for _, m := range d.MCPServers {
				fmt.Fprintf(out, "    - %s\n", m)
			}
		}
		if len(d.Marketplaces) > 0 {
			fmt.Fprintf(out, "  Registered marketplaces (%d):\n", len(d.Marketplaces))
			for _, mp := range d.Marketplaces {
				fmt.Fprintf(out, "    - %s\n", mp)
			}
		}

		// Catalog mapping — what the wizard would pre-check.
		if catalogErr == nil {
			merged := append(append([]string{}, d.PluginIDs...), d.MCPServers...)
			matched := catalog.MatchHostIDs(entries, id, merged)
			if len(matched) > 0 {
				fmt.Fprintf(out, "  %sCatalog matches (%d) — wizard would pre-check:%s\n",
					c.bold, len(matched), c.reset)
				for _, m := range matched {
					fmt.Fprintf(out, "    %s✓%s %s\n", c.green, c.reset, m)
				}
			} else {
				fmt.Fprintf(out, "  %s(no catalog matches for the detected ids)%s\n", c.dim, c.reset)
			}
		}

		if len(d.Notes) > 0 {
			fmt.Fprintf(out, "  %sNotes:%s\n", c.dim, c.reset)
			for _, n := range d.Notes {
				fmt.Fprintf(out, "    %s- %s%s\n", c.dim, n, c.reset)
			}
		}
	}
	fmt.Fprintln(out)
	return nil
}
