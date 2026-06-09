package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/wlame/vibrator/internal/integration"
	claudememInteg "github.com/wlame/vibrator/internal/integration/claudemem"
	"github.com/wlame/vibrator/internal/prereq"
)

// runIntegrationsCM owns the host-only admin-config setup (server URL,
// team, DSN, stack dir) and then delegates everything else — runtime
// management, probe, workspace bootstrap — to the generic runner.
//
// The admin config form is bespoke because DSN entry has two modes
// (full string vs. individual fields) and because we want to validate
// the resolved stack directory before handing off. Everything past
// the form is identical to what every other integration gets.
func runIntegrationsCM(cmd *cobra.Command, _ []string) error {
	c := newColors(!isTerminal(cmd.OutOrStdout()))
	out := cmd.OutOrStdout()

	cfgPath := prereq.ClaudeMemAdminConfigPath()
	fmt.Fprintf(out, "\n%sclaude-mem (server-beta runtime)%s\n", c.bold, c.reset)
	fmt.Fprintln(out, strings.Repeat("─", 60))
	fmt.Fprintf(out, "  Admin config: %s\n\n", cfgPath)

	if err := cmAdminConfigForm(cmd, c); err != nil {
		return err
	}

	// Delegate to the generic runner, which handles compose start/stop,
	// probe, and workspace bootstrap.
	integ, ok := integration.Get("claude-mem")
	if !ok {
		return fmt.Errorf("claude-mem integration not registered (build error?)")
	}
	return runIntegration(cmd, integ)
}

// cmAdminConfigForm walks the user through editing
// ~/.config/vibrator/claude-mem.toml. Returns nil on save (including
// the case where the user cancelled and nothing was changed).
func cmAdminConfigForm(cmd *cobra.Command, c colors) error {
	out := cmd.OutOrStdout()

	existing, err := prereq.LoadClaudeMemAdminConfig()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load admin config: %w", err)
	}

	// Step 1 — server URL + team + stack dir.
	type baseBindings struct {
		ServerURL string
		TeamName  string
		StackDir  string
	}
	b := baseBindings{
		ServerURL: "http://host.docker.internal:37877",
		TeamName:  "vibrators",
		StackDir:  defaultStackDirFromHome(),
	}
	if existing != nil {
		if existing.ServerURL != "" {
			b.ServerURL = existing.ServerURL
		}
		if existing.TeamName != "" {
			b.TeamName = existing.TeamName
		}
		if existing.StackDir != "" {
			b.StackDir = existing.StackDir
		}
		fmt.Fprintf(out, "  %sExisting config found — fields pre-filled.%s\n\n", c.dim, c.reset)
	}

	baseForm := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Server URL").
			Description("Reachable from both host and container.\n"+
				"Use http://host.docker.internal:<port>.").
			Value(&b.ServerURL).
			Validate(nonEmpty("server URL")),
		huh.NewInput().
			Title("Team name  (optional)").
			Description(`Team scope for minted keys. Default: "vibrators".`).
			Value(&b.TeamName),
		huh.NewInput().
			Title("Stack directory").
			Description("Path to the cloned claude-mem-stack repo (contains docker-compose.yml).\n"+
				"vibrator runs `docker compose up -d` here.").
			Value(&b.StackDir),
	)).WithTheme(huh.ThemeCharm())

	if err := baseForm.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Fprintln(out, "Cancelled.")
			return nil
		}
		return err
	}

	// Step 2 — database connection (full DSN / individual fields / skip).
	existingDSN := ""
	if existing != nil {
		existingDSN = existing.DatabaseURL
	}
	dsn, cancelled, err := cmDBInputForm(existingDSN)
	if err != nil {
		return err
	}
	if cancelled {
		fmt.Fprintln(out, "Cancelled.")
		return nil
	}

	// Commit. Empty TeamName matching the default is dropped so the
	// file reflects "use default" rather than a literal "vibrators".
	teamName := strings.TrimSpace(b.TeamName)
	if teamName == "vibrators" {
		teamName = ""
	}
	cfg := &prereq.ClaudeMemAdminConfig{
		Runtime:     "server-beta",
		ServerURL:   strings.TrimSpace(b.ServerURL),
		DatabaseURL: dsn,
		TeamName:    teamName,
		StackDir:    strings.TrimSpace(b.StackDir),
	}
	if err := prereq.SaveClaudeMemAdminConfig(cfg); err != nil {
		return fmt.Errorf("save admin config: %w", err)
	}
	fmt.Fprintf(out, "\n  %s✓%s Admin config saved.\n", c.green, c.reset)

	// Pre-flight: if the stack dir doesn't have a compose file yet,
	// nudge the user to clone the stack — the generic runner's
	// Start would otherwise fail with a less-helpful error.
	expanded := expandTilde(cfg.StackDir)
	if expanded != "" && !claudememInteg.ComposeFileExists(expanded) {
		fmt.Fprintf(out, "\n  %s⚠%s Stack not found at %s.\n", c.yellow, c.reset, expanded)
		fmt.Fprintf(out, "    Clone it before continuing:\n")
		fmt.Fprintf(out, "      git clone https://github.com/thedotmack/claude-mem %s\n\n", expanded)
	}
	return nil
}

// nonEmpty returns a huh validator that requires non-empty trimmed
// input. label is the field name used in the error message.
func nonEmpty(label string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("%s is required", label)
		}
		return nil
	}
}

// defaultStackDirFromHome returns ~/dev/claude-mem-stack expanded
// against the current $HOME for use as the form's default value.
func defaultStackDirFromHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/dev/claude-mem-stack"
	}
	return filepath.Join(home, "dev", "claude-mem-stack")
}

// expandTilde rewrites a leading "~/" against $HOME. Mirrors the
// integration package's helper (kept local here to avoid cross-import
// noise — file-system path expansion is a CLI concern).
func expandTilde(p string) string {
	if !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[2:])
}

// ── DSN input form ─────────────────────────────────────────────────────

// cmDBInputForm presents the database-connection mode picker, then the
// matching input form. Returns the resolved DSN (empty = "skip").
// Uses two separate huh.NewForm calls — the huh v1 pattern that lets
// conditional logic live between form runs (Group has no commit hook).
func cmDBInputForm(existing string) (dsn string, cancelled bool, err error) {
	var mode string
	modeForm := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Database connection").
			Options(
				huh.NewOption("Full DSN  (postgres://user:pass@host:port/db)", "dsn"),
				huh.NewOption("Individual fields  (host / port / user / password / db)", "fields"),
				huh.NewOption("Skip  (no database — bootstrap disabled)", "skip"),
			).
			Value(&mode),
	)).WithTheme(huh.ThemeCharm())

	if err := modeForm.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", true, nil
		}
		return "", false, err
	}

	switch mode {
	case "skip":
		return "", false, nil

	case "dsn":
		val := existing
		dsnForm := huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title("Database URL").
				Description("Postgres DSN. NEVER forwarded to the container — host-only credential.").
				EchoMode(huh.EchoModePassword).
				Value(&val).
				Validate(nonEmpty("DSN")),
		)).WithTheme(huh.ThemeCharm())
		if err := dsnForm.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return "", true, nil
			}
			return "", false, err
		}
		return strings.TrimSpace(val), false, nil

	case "fields":
		type fieldBindings struct {
			Host, Port, User, Password, DB string
		}
		fb := fieldBindings{Port: "5432"}
		if existing != "" {
			if h, p, u, pw, db, ok := parseDSN(existing); ok {
				fb.Host, fb.Port, fb.User, fb.Password, fb.DB = h, p, u, pw, db
			}
		}

		fieldsForm := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Host").Value(&fb.Host).Validate(nonEmpty("host")),
			huh.NewInput().Title("Port").Value(&fb.Port),
			huh.NewInput().Title("User").Value(&fb.User).Validate(nonEmpty("user")),
			huh.NewInput().Title("Password").EchoMode(huh.EchoModePassword).Value(&fb.Password),
			huh.NewInput().Title("Database name").Value(&fb.DB).Validate(nonEmpty("database")),
		)).WithTheme(huh.ThemeCharm())

		if err := fieldsForm.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return "", true, nil
			}
			return "", false, err
		}
		return cmBuildDSN(
			strings.TrimSpace(fb.Host),
			strings.TrimSpace(fb.Port),
			strings.TrimSpace(fb.User),
			fb.Password,
			strings.TrimSpace(fb.DB),
		), false, nil
	}
	return "", false, nil
}

// cmBuildDSN constructs a postgres:// DSN from individual fields.
// Password gets a minimal URL-encode for the @ / : / / characters
// that would otherwise break the URI parser. Use of a full URL
// encoder would be safer but pulls net/url in for one helper —
// the existing CLI predates that and we keep the behavior identical.
func cmBuildDSN(host, port, user, password, db string) string {
	if port == "" {
		port = "5432"
	}
	pw := strings.ReplaceAll(password, "@", "%40")
	pw = strings.ReplaceAll(pw, ":", "%3A")
	pw = strings.ReplaceAll(pw, "/", "%2F")
	if pw != "" {
		return fmt.Sprintf("postgres://%s:%s@%s:%s/%s", user, pw, host, port, db)
	}
	return fmt.Sprintf("postgres://%s@%s:%s/%s", user, host, port, db)
}

// parseDSN naively extracts fields from a postgres:// DSN for
// pre-filling the individual-fields form. Returns ok=false when the
// shape doesn't look like a postgres URL — caller starts with blank
// fields in that case.
func parseDSN(dsn string) (host, port, user, password, db string, ok bool) {
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		return
	}
	rest := strings.SplitN(dsn, "://", 2)[1]
	atIdx := strings.LastIndex(rest, "@")
	if atIdx < 0 {
		return
	}
	userinfo := rest[:atIdx]
	hostdb := rest[atIdx+1:]
	if colonIdx := strings.Index(userinfo, ":"); colonIdx >= 0 {
		user = userinfo[:colonIdx]
		password = userinfo[colonIdx+1:]
	} else {
		user = userinfo
	}
	slashIdx := strings.Index(hostdb, "/")
	if slashIdx >= 0 {
		db = hostdb[slashIdx+1:]
		hostport := hostdb[:slashIdx]
		if colonIdx := strings.LastIndex(hostport, ":"); colonIdx >= 0 {
			host = hostport[:colonIdx]
			port = hostport[colonIdx+1:]
		} else {
			host = hostport
			port = "5432"
		}
	} else {
		host = hostdb
		port = "5432"
	}
	ok = host != "" && user != "" && db != ""
	return
}
