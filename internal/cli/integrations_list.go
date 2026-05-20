package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wlame/vibrator/internal/integration"

	// Side-effect imports — each sub-package registers its Integration
	// in its init(). Adding a new built-in integration: drop a new
	// blank import line here. User-defined TOML integrations are
	// loaded dynamically (see init() below).
	_ "github.com/wlame/vibrator/internal/integration/claudemem"
	_ "github.com/wlame/vibrator/internal/integration/serena"
)

// integrationsListCmd implements `vibrate integrations list`. It
// enumerates the registry, including each integration's identity,
// available host runtimes, and current reachability.
var integrationsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered integrations and their current status",
	RunE:  runIntegrationsList,
}

func init() {
	integrationsCmd.AddCommand(integrationsListCmd)

	// Load any user-defined TOML integrations from the standard config
	// directory. Errors are non-fatal — the loader records them so the
	// `list` command can surface them at the bottom of the output.
	_, _ = integration.LoadFromDir(integration.UserIntegrationsDir())
}

func runIntegrationsList(cmd *cobra.Command, _ []string) error {
	c := newColors(!isTerminal(cmd.OutOrStdout()))
	out := cmd.OutOrStdout()

	all := integration.All()
	fmt.Fprintf(out, "\n%sRegistered integrations%s  (%d)\n", c.bold, c.reset, len(all))
	fmt.Fprintln(out, strings.Repeat("─", 42))

	for _, i := range all {
		renderIntegration(out, i, c)
	}

	// Surface any TOML load errors so users can spot a bad descriptor
	// quickly. Common cases: missing required field, typo in keys.
	if errs := integration.LoadErrors(); len(errs) > 0 {
		fmt.Fprintf(out, "\n%sLoad errors%s\n", c.yellow, c.reset)
		fmt.Fprintln(out, strings.Repeat("─", 42))
		for _, e := range errs {
			fmt.Fprintf(out, "  %s✗%s %s\n", c.red, c.reset, e.Error())
		}
	}
	fmt.Fprintln(out)
	return nil
}

// renderIntegration prints one Integration's identity block + status.
// Field layout is fixed-width for readability — keep label widths in
// sync if you add new rows.
func renderIntegration(out io.Writer, i *integration.Integration, c colors) {
	fmt.Fprintf(out, "\n  %s%s%s  (%s)\n", c.bold, i.Name, c.reset, i.ID)
	fmt.Fprintf(out, "    %s\n", i.Summary)
	if i.Category != "" {
		fmt.Fprintf(out, "    %sCategory:%s %s\n", c.dim, c.reset, i.Category)
	}
	if i.DocsURL != "" {
		fmt.Fprintf(out, "    %sDocs:%s     %s\n", c.dim, c.reset, i.DocsURL)
	}
	if len(i.Runtimes) > 0 {
		kinds := make([]string, 0, len(i.Runtimes))
		for _, r := range i.Runtimes {
			kinds = append(kinds, r.Kind())
		}
		fmt.Fprintf(out, "    %sRuntimes:%s %s\n", c.dim, c.reset, strings.Join(kinds, ", "))
	}
	if i.AdminConfig != nil && i.AdminConfig.Path != "" {
		fmt.Fprintf(out, "    %sConfig:%s   %s\n", c.dim, c.reset, i.AdminConfig.Path)
	}
	if probeDesc := safeProbeDescribe(i); probeDesc != "" {
		fmt.Fprintf(out, "    %sProbe:%s    %s\n", c.dim, c.reset, probeDesc)
	}
	renderStatus(out, i, c)
}

// renderStatus checks each runtime's Status() in turn, then falls back
// to a Probe.Check if nothing local is running — that's how we detect
// "externally managed but reachable" cases.
func renderStatus(out io.Writer, i *integration.Integration, c colors) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for _, r := range i.Runtimes {
		rs, err := r.Status(ctx)
		if err != nil {
			continue
		}
		if rs.Running {
			fmt.Fprintf(out, "    %sStatus:%s   %s✓ running%s (%s)\n",
				c.dim, c.reset, c.green, c.reset, r.Kind())
			return
		}
	}

	// Not running in any known mode — try the probe to catch the
	// "user runs it themselves and we can still see it" case.
	if i.ProbeFn != nil {
		probe, err := i.ProbeFn(ctx)
		if err == nil && probe != nil {
			if checkErr := probe.Check(ctx); checkErr == nil {
				fmt.Fprintf(out, "    %sStatus:%s   %s✓ reachable (external)%s\n",
					c.dim, c.reset, c.green, c.reset)
				return
			}
		}
	}

	fmt.Fprintf(out, "    %sStatus:%s   %s✗ not running%s\n", c.dim, c.reset, c.dim, c.reset)
}

// safeProbeDescribe returns the probe's Describe() string, or "" if
// ProbeFn isn't set / errors / returns nil. Used for the Probe row in
// the `list` output — purely informational.
func safeProbeDescribe(i *integration.Integration) string {
	if i.ProbeFn == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	probe, err := i.ProbeFn(ctx)
	if err != nil || probe == nil {
		return ""
	}
	return probe.Describe()
}
