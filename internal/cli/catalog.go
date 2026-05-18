package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	vibrator "github.com/wlame/vibrator"
	"github.com/wlame/vibrator/internal/catalog"
)

// catalogCmd surfaces the curated catalog of tools/plugins/skills per harness.
// Catalog entries live in catalog/<harness>/<id>.md and are loaded from an
// embed.FS at compile time.
var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "List and inspect catalog entries (plugins, MCP servers, skills) per harness",
}

// catalogListCmd lists either harnesses (no args) or entries within a harness.
var catalogListCmd = &cobra.Command{
	Use:   "list [HARNESS]",
	Short: "List catalog harnesses, or entries within one harness",
	Long: `Without arguments, lists known harnesses (claude-code, codex, opencode, pi).
With a harness argument, lists the catalog entries for that harness.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCatalogList,
}

// catalogShowCmd prints one entry's frontmatter + prose body.
var catalogShowCmd = &cobra.Command{
	Use:   "show ID",
	Short: "Print a catalog entry's metadata + prose body",
	Long: `Print the frontmatter metadata + markdown body for one entry.

Accepts either "<harness>/<id>" (unambiguous) or just "<id>" (looked up
across all harnesses; errors if multiple harnesses contain the id).`,
	Args: cobra.ExactArgs(1),
	RunE: runCatalogShow,
}

func init() {
	catalogCmd.AddCommand(catalogListCmd)
	catalogCmd.AddCommand(catalogShowCmd)
	rootCmd.AddCommand(catalogCmd)
}

func runCatalogList(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return printHarnesses(cmd)
	}
	return printEntries(cmd, args[0])
}

// printHarnesses lists every harness that has at least one catalog entry,
// plus the count.
func printHarnesses(cmd *cobra.Command) error {
	entries, err := catalog.LoadAll(vibrator.CatalogFS)
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}
	if len(entries) == 0 {
		cmd.Println("(catalog is empty)")
		return nil
	}

	// Tally entries per harness.
	counts := make(map[string]int)
	for _, e := range entries {
		counts[e.Harness]++
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "HARNESS\tENTRIES")
	for _, h := range sortedKeys(counts) {
		fmt.Fprintf(w, "%s\t%d\n", h, counts[h])
	}
	return w.Flush()
}

// printEntries lists the entries under a specific harness, grouped by kind.
func printEntries(cmd *cobra.Command, harness string) error {
	entries, err := catalog.LoadForHarness(vibrator.CatalogFS, harness)
	if err != nil {
		return fmt.Errorf("load harness %q: %w", harness, err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("no catalog entries for harness %q (run `vibrate catalog list` to see known harnesses)", harness)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tKIND\tDEFAULT\tSIZE\tNAME")
	for _, e := range entries {
		defaultMark := ""
		if e.Default {
			defaultMark = "✓"
		}
		size := ""
		if e.SizeMB > 0 {
			size = fmt.Sprintf("%dMB", e.SizeMB)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			e.ID, e.Kind, defaultMark, size, e.Name)
	}
	return w.Flush()
}

// runCatalogShow handles both "harness/id" and "id" lookup forms.
func runCatalogShow(cmd *cobra.Command, args []string) error {
	all, err := catalog.LoadAll(vibrator.CatalogFS)
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}

	q := args[0]

	// Form A: harness/id — direct map lookup.
	if strings.Contains(q, "/") {
		entry, ok := all[q]
		if !ok {
			return fmt.Errorf("catalog entry %q not found", q)
		}
		return renderEntry(cmd, entry)
	}

	// Form B: bare id — search across harnesses. Ambiguous (multi-harness
	// hits) is an error so the user picks explicitly.
	var matches []*catalog.Entry
	for _, e := range all {
		if e.ID == q {
			matches = append(matches, e)
		}
	}
	switch len(matches) {
	case 0:
		return fmt.Errorf("catalog entry %q not found", q)
	case 1:
		return renderEntry(cmd, matches[0])
	default:
		var keys []string
		for _, m := range matches {
			keys = append(keys, m.Key())
		}
		return fmt.Errorf("ambiguous id %q matches multiple entries: %v (specify as harness/id)", q, keys)
	}
}

// renderEntry prints a single entry's frontmatter + body in a human-readable
// form. Output is plain text — Phase 5 may add ANSI color or markdown
// rendering via lipgloss / glamour, but for now this is enough.
func renderEntry(cmd *cobra.Command, e *catalog.Entry) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "ID:       %s/%s\n", e.Harness, e.ID)
	fmt.Fprintf(out, "Name:     %s\n", e.Name)
	fmt.Fprintf(out, "Kind:     %s\n", e.Kind)
	fmt.Fprintf(out, "Default:  %v\n", e.Default)
	if e.SizeMB > 0 {
		fmt.Fprintf(out, "Size:     ~%dMB\n", e.SizeMB)
	}
	if len(e.Deps.Features) > 0 {
		fmt.Fprintf(out, "Features: %s\n", strings.Join(e.Deps.Features, ", "))
	}
	if len(e.Deps.Catalog) > 0 {
		fmt.Fprintf(out, "Catalog:  %s\n", strings.Join(e.Deps.Catalog, ", "))
	}
	if e.Prereq != "" {
		fmt.Fprintf(out, "Prereq:   %s\n", e.Prereq)
	}
	if e.Auth != nil && e.Auth.Env != "" {
		fmt.Fprintf(out, "Auth env: %s\n", e.Auth.Env)
	}
	if e.Source != "" {
		fmt.Fprintf(out, "Source:   %s\n", e.Source)
	}
	if e.Install != "" {
		fmt.Fprintln(out, "Install:")
		for _, line := range strings.Split(strings.TrimRight(e.Install, "\n"), "\n") {
			fmt.Fprintf(out, "    %s\n", line)
		}
	}
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "---")
	fmt.Fprintln(out, "")
	fmt.Fprint(out, e.Body)
	return nil
}

// sortedKeys returns the keys of m sorted lexicographically. Stamped here so
// internal/cli doesn't have to depend on a separate utility package.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Simple insertion sort — maps in this command are tiny.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
