package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/prereq"
)

// prereqsCmd is a parent command for prerequisite-related operations:
//   - vibrate prereqs status        — probe host stacks, dump resolved wiring
//   - vibrate prereqs bootstrap ID  — run host-side setup (e.g., mint claude-mem key)
var prereqsCmd = &cobra.Command{
	Use:   "prereqs",
	Short: "Inspect and bootstrap host-side prerequisites for selected tools",
}

// prereqsStatusFlags holds flag state shared by `prereqs status`.
type prereqsStatusFlags struct {
	noColor bool
}

var prereqsStatusFlagsState prereqsStatusFlags

var prereqsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Probe host stacks declared by selected extension entries",
	Long: `Probes each known prerequisite and prints a one-screen status report.
Currently the only built-in prerequisite is claude-mem (server-beta runtime).`,
	RunE: runPrereqsStatus,
}

// prereqsBootstrapFlags holds flag state for `prereqs bootstrap`.
type prereqsBootstrapFlags struct {
	force   bool
	noColor bool
}

var prereqsBootstrapFlagsState prereqsBootstrapFlags

var prereqsBootstrapCmd = &cobra.Command{
	Use:   "bootstrap PREREQ_ID",
	Short: "Run host-side bootstrap for a prerequisite (no container launch)",
	Long: `Runs the host-side auto-setup for a prerequisite. Currently supports:

  claude-mem-server-beta   Mint a project-scoped API key against the host's
                           claude-mem postgres, then cache it in the
                           workspace pin file (.vb).

Idempotent by default: if .vb already contains a cached value for this
prerequisite, the command exits without re-minting. Use --force to rotate.`,
	Args: cobra.ExactArgs(1),
	RunE: runPrereqsBootstrap,
}

func init() {
	prereqsStatusCmd.Flags().BoolVar(&prereqsStatusFlagsState.noColor, "no-color", false,
		"Disable ANSI colors in the status report.")
	prereqsBootstrapCmd.Flags().BoolVar(&prereqsBootstrapFlagsState.force, "force", false,
		"Re-mint even when .vb already has a cached value for this prereq.")
	prereqsBootstrapCmd.Flags().BoolVar(&prereqsBootstrapFlagsState.noColor, "no-color", false,
		"Disable ANSI colors in the output.")

	prereqsCmd.AddCommand(prereqsStatusCmd)
	prereqsCmd.AddCommand(prereqsBootstrapCmd)
	rootCmd.AddCommand(prereqsCmd)
}

// runPrereqsStatus prints a one-screen status report covering the known
// prereqs. Right now claude-mem-server-beta is the only built-in.
//
// The report has three sections per prereq:
//  1. Admin config — where it lives, whether it loaded, the key fields
//  2. Server probe — HTTP reachability with the resolved URL
//  3. Workspace cache — whether .vb has the values minted by a prior bootstrap
//
// Exit code 0 = everything we could check passed. Exit code 1 = at least
// one required step failed (so callers like `make check` can gate on this).
func runPrereqsStatus(cmd *cobra.Command, _ []string) error {
	c := newColors(prereqsStatusFlagsState.noColor || !isTerminal(cmd.OutOrStdout()))
	out := cmd.OutOrStdout()
	allOK := true

	// --- claude-mem ---
	fmt.Fprintf(out, "\n%s%s%s\n", c.bold, "claude-mem (server-beta runtime)", c.reset)
	fmt.Fprintln(out, strings.Repeat("=", 33))

	cfgPath := prereq.ClaudeMemAdminConfigPath()
	fmt.Fprintf(out, "  Admin config:   %s\n", cfgPath)

	cfg, err := prereq.LoadClaudeMemAdminConfig()
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(out, "                    %sMISSING%s — create it with runtime=\"server-beta\" and server_url\n",
			c.red, c.reset)
		fmt.Fprintln(out)
		return errors.New("claude-mem admin config not configured")
	}
	if err != nil {
		fmt.Fprintf(out, "                    %sPARSE ERROR%s: %v\n", c.red, c.reset, err)
		return err
	}
	fmt.Fprintf(out, "    Runtime:      %s\n", or(cfg.Runtime, "(unset)"))
	fmt.Fprintf(out, "    Server URL:   %s\n", or(cfg.ServerURL, "(unset)"))
	if cfg.DatabaseURL != "" {
		fmt.Fprintf(out, "    Database URL: (set — bootstrap available)\n")
	} else {
		fmt.Fprintf(out, "    Database URL: %s(unset — bootstrap disabled, manual key required)%s\n", c.dim, c.reset)
	}

	// --- Server probe ---
	p := prereq.ClaudeMemPrereq(cfg, nil)
	r := p.Verifier.Verify(context.Background())
	fmt.Fprintln(out)
	if r.OK {
		fmt.Fprintf(out, "  Server probe:   %s✓%s %s\n", c.green, c.reset, r.Message)
	} else {
		allOK = false
		fmt.Fprintf(out, "  Server probe:   %s✗%s %s\n", c.red, c.reset, r.Message)
		if r.Hint != "" {
			fmt.Fprintf(out, "                    %shint:%s %s\n", c.dim, c.reset, r.Hint)
		}
	}

	// --- Workspace cache ---
	fmt.Fprintln(out)
	ws, projectName, pinPath, err := resolveWorkspace()
	if err != nil {
		fmt.Fprintf(out, "  Workspace:      %s(unresolved: %v)%s\n", c.dim, err, c.reset)
		return err
	}
	fmt.Fprintf(out, "  Workspace:      %s\n", ws)
	fmt.Fprintf(out, "  Project name:   %s\n", projectName)

	cached, ok := loadCachedPrereq(pinPath, prereq.ClaudeMemPrereqID)
	if !ok {
		fmt.Fprintf(out, "  Cached key:     %sNONE%s\n", c.yellow, c.reset)
		if cfg.DatabaseURL != "" {
			fmt.Fprintf(out, "                    next: %svibrate prereqs bootstrap claude-mem-server-beta%s\n",
				c.bold, c.reset)
		} else {
			fmt.Fprintf(out, "                    %sno database_url in admin config — bootstrap disabled%s\n", c.dim, c.reset)
		}
		fmt.Fprintln(out)
		if !allOK {
			return errors.New("prereq probe failed")
		}
		return nil
	}

	apiKey := cached["api_key"]
	teamID := cached["team_id"]
	projectID := cached["project_id"]
	fmt.Fprintf(out, "  Cached key:     %s✓%s present in %s\n", c.green, c.reset, pinPath)
	if apiKey != "" {
		fmt.Fprintf(out, "    API key:        %s…  (length: %d)\n", apiKey[:min(12, len(apiKey))], len(apiKey))
	}
	if teamID != "" {
		fmt.Fprintf(out, "    Team id:        %s\n", teamID)
	}
	if projectID != "" {
		fmt.Fprintf(out, "    Project id:     %s\n", projectID)
	}

	fmt.Fprintln(out)
	if !allOK {
		return errors.New("prereq probe failed")
	}
	return nil
}

// runPrereqsBootstrap dispatches the requested prereq's Bootstrap, persists
// the result into the workspace pin, and ensures .vb is gitignored.
func runPrereqsBootstrap(cmd *cobra.Command, args []string) error {
	id := args[0]
	c := newColors(prereqsBootstrapFlagsState.noColor || !isTerminal(cmd.OutOrStdout()))
	out := cmd.OutOrStdout()

	if id != prereq.ClaudeMemPrereqID {
		return fmt.Errorf("unknown prereq %q (supported: %s)", id, prereq.ClaudeMemPrereqID)
	}

	// Resolve workspace context.
	wsPath, projectName, pinPath, err := resolveWorkspace()
	if err != nil {
		return err
	}

	// Idempotency check: if we already have a value in .vb, bail unless --force.
	if existing, ok := loadCachedPrereq(pinPath, id); ok && !prereqsBootstrapFlagsState.force {
		fmt.Fprintf(out, "%s✓%s %s already cached in %s (use --force to rotate)\n",
			c.green, c.reset, id, pinPath)
		if k := existing["api_key"]; k != "" {
			fmt.Fprintf(out, "  current api_key prefix: %s…\n", k[:min(12, len(k))])
		}
		return nil
	}

	// Load admin config.
	cfg, err := prereq.LoadClaudeMemAdminConfig()
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("admin config not found at %s — create it first",
			prereq.ClaudeMemAdminConfigPath())
	}
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf(
			"admin config at %s has no database_url — bootstrap requires it",
			prereq.ClaudeMemAdminConfigPath())
	}

	// Resolve docker client (real, not mock).
	dc, err := docker.NewCLIClient()
	if err != nil {
		return err
	}

	// Build the prereq with both verifier + bootstrapper wired.
	p := prereq.ClaudeMemPrereq(cfg, dc)
	if p.Bootstrapper == nil {
		// Defensive — should be unreachable since we validated DatabaseURL above.
		return errors.New("ClaudeMemPrereq returned nil Bootstrapper despite valid config")
	}

	hostname, _ := os.Hostname()
	ws := prereq.Workspace{
		Path:        wsPath,
		ProjectName: projectName,
		Hostname:    hostname,
	}

	fmt.Fprintf(out, "Bootstrapping %s for project=%q on team=%q ...\n", id, projectName, or(cfg.TeamName, "vibrators"))

	result, err := p.Bootstrapper.Bootstrap(context.Background(), ws)
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	if err := persistPrereqResult(pinPath, id, result); err != nil {
		return fmt.Errorf("persist to %s: %w", pinPath, err)
	}

	// .gitignore: append .vb if not already there (idempotent).
	wsDir := filepath.Dir(pinPath)
	changed, err := config.AppendToGitignore(wsDir)
	if err != nil {
		fmt.Fprintf(out, "  %swarning: could not update .gitignore: %v%s\n", c.yellow, err, c.reset)
	}

	// Summary.
	fmt.Fprintf(out, "  %s✓%s team_id    = %s\n", c.green, c.reset, result["team_id"])
	fmt.Fprintf(out, "  %s✓%s project_id = %s\n", c.green, c.reset, result["project_id"])
	if k := result["api_key"]; k != "" {
		fmt.Fprintf(out, "  %s✓%s api_key    = %s… (length %d)\n", c.green, c.reset, k[:min(12, len(k))], len(k))
	}
	fmt.Fprintf(out, "  %s✓%s persisted in %s [prereqs.%s]\n", c.green, c.reset, pinPath, id)
	if changed {
		fmt.Fprintf(out, "  %s✓%s added .vb to .gitignore\n", c.green, c.reset)
	}
	return nil
}

// --- helpers --------------------------------------------------------------

// resolveWorkspace finds the workspace root and the pin file path. If a
// .vb already exists upward of $PWD, we treat its directory as the
// workspace; otherwise we use $PWD itself and the pin path is $PWD/.vb.
func resolveWorkspace() (workspaceDir, projectName, pinPath string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", "", err
	}

	if existing, err := config.FindPin(cwd); err == nil {
		workspaceDir = filepath.Dir(existing)
		pinPath = existing
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", "", err
	} else {
		workspaceDir = cwd
		pinPath = filepath.Join(cwd, config.PinFileName)
	}

	projectName = filepath.Base(workspaceDir)
	return workspaceDir, projectName, pinPath, nil
}

// loadCachedPrereq reads pinPath and returns pin.Prereqs[id] if present.
// Returns (nil, false) when the pin is missing or has no entry for this id.
func loadCachedPrereq(pinPath, id string) (map[string]string, bool) {
	pin, err := config.Load(pinPath)
	if err != nil {
		return nil, false
	}
	v, ok := pin.Prereqs[id]
	return v, ok && len(v) > 0
}

// persistPrereqResult merges `result` into the pin's Prereqs[id] section
// (creating the file if missing) and writes it back to disk with mode 0600.
// Prefers to preserve any existing scalar/list fields the user may have
// already configured.
func persistPrereqResult(pinPath, id string, result map[string]string) error {
	pin, err := config.Load(pinPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		pin = &config.Pin{}
	}
	if pin.Prereqs == nil {
		pin.Prereqs = make(map[string]map[string]string)
	}
	pin.Prereqs[id] = result
	return config.Save(pinPath, pin)
}

// or returns s if non-empty, else fallback. Pure helper for status output.
func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// min returns the smaller of a and b (no go1.21 builtin used because we may
// support older toolchains in CI).
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- terminal color helpers ----------------------------------------------

// colors is a tiny ANSI helper. Construct via newColors(disable bool); when
// disabled, every field is "" so the same format strings produce plain text.
type colors struct {
	bold, dim, green, yellow, red, reset string
}

func newColors(disable bool) colors {
	if disable {
		return colors{}
	}
	return colors{
		bold:   "\033[1m",
		dim:    "\033[2m",
		green:  "\033[32m",
		yellow: "\033[33m",
		red:    "\033[31m",
		reset:  "\033[0m",
	}
}

// isTerminal returns true if w is a *os.File pointing at a TTY. The check
// is best-effort: when w isn't an *os.File (e.g., bytes.Buffer in tests),
// we return false so tests see plain output.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
