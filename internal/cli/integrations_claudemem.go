package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/prereq"
)

// runIntegrationsCM is the entry point for `vibrate integrations claude-mem`.
func runIntegrationsCM(cmd *cobra.Command, _ []string) error {
	c := newColors(!isTerminal(cmd.OutOrStdout()))
	out := cmd.OutOrStdout()

	cfgPath := prereq.ClaudeMemAdminConfigPath()
	fmt.Fprintf(out, "\n%s%s%s\n", c.bold, "claude-mem (server-beta runtime)", c.reset)
	fmt.Fprintln(out, strings.Repeat("─", 42))
	fmt.Fprintf(out, "  Config: %s\n\n", cfgPath)

	// Pre-fill from any existing admin config.
	existing, err := prereq.LoadClaudeMemAdminConfig()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load admin config: %w", err)
	}

	// ── Step 1: Server URL + team name ──────────────────────────────────────
	type serverBindings struct {
		ServerURL string
		TeamName  string
	}
	sb := serverBindings{
		ServerURL: "http://host.docker.internal:37877",
		TeamName:  "vibrators",
	}
	if existing != nil {
		if existing.ServerURL != "" {
			sb.ServerURL = existing.ServerURL
		}
		if existing.TeamName != "" {
			sb.TeamName = existing.TeamName
		}
		fmt.Fprintf(out, "  %sExisting config found — fields pre-filled.%s\n\n", c.dim, c.reset)
	}

	serverForm := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Server URL").
			Description("Endpoint reachable from both host and container.\n"+
				"Use http://host.docker.internal:<port>.").
			Value(&sb.ServerURL).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return errors.New("server URL is required")
				}
				return nil
			}),
		huh.NewInput().
			Title("Team name  (optional)").
			Description(`Team scope for minted keys. Default: "vibrators".`).
			Value(&sb.TeamName),
	)).WithTheme(huh.ThemeCharm())

	if err := serverForm.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Fprintln(out, "\nCancelled.")
			return nil
		}
		return err
	}

	// ── Step 2: Database connection ─────────────────────────────────────────
	existingDSN := ""
	if existing != nil {
		existingDSN = existing.DatabaseURL
	}
	dsn, cancelled, err := cmDBInputForm(existingDSN)
	if err != nil {
		return err
	}
	if cancelled {
		fmt.Fprintln(out, "\nCancelled.")
		return nil
	}

	// ── Commit: build + save admin config ───────────────────────────────────
	teamName := strings.TrimSpace(sb.TeamName)
	if teamName == "vibrators" {
		teamName = ""
	}
	cfg := &prereq.ClaudeMemAdminConfig{
		Runtime:     "server-beta",
		ServerURL:   strings.TrimSpace(sb.ServerURL),
		DatabaseURL: dsn,
		TeamName:    teamName,
	}
	if err := prereq.SaveClaudeMemAdminConfig(cfg); err != nil {
		return fmt.Errorf("save admin config: %w", err)
	}
	fmt.Fprintf(out, "\n  %s✓%s Admin config saved: %s\n", c.green, c.reset, cfgPath)

	// ── Step 3: Server setup ─────────────────────────────────────────────────
	if dsn != "" {
		if err := cmServerSetupFlow(cmd, dsn, c); err != nil {
			return err
		}
	}

	// ── Step 4: Probe + workspace bootstrap ─────────────────────────────────
	return cmProbeAndBootstrap(cmd, cfg, c)
}

// cmDBInputForm presents the database connection mode picker, then the
// appropriate input form. Returns the resolved DSN (empty = skip).
// Uses two separate huh.NewForm calls per the huh v1 pattern — Group has
// no commit hook so conditional logic lives between form runs.
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
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("DSN is required (use Skip if you don't have one yet)")
					}
					return nil
				}),
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
			Host     string
			Port     string
			User     string
			Password string
			DB       string
		}
		fb := fieldBindings{Port: "5432"}
		// Pre-parse existing DSN into fields if present.
		if existing != "" {
			if h, p, u, pw, db, ok := parseDSN(existing); ok {
				fb.Host, fb.Port, fb.User, fb.Password, fb.DB = h, p, u, pw, db
			}
		}

		fieldsForm := huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title("Host").
				Value(&fb.Host).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("host is required")
					}
					return nil
				}),
			huh.NewInput().
				Title("Port").
				Value(&fb.Port),
			huh.NewInput().
				Title("User").
				Value(&fb.User).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("user is required")
					}
					return nil
				}),
			huh.NewInput().
				Title("Password").
				EchoMode(huh.EchoModePassword).
				Value(&fb.Password),
			huh.NewInput().
				Title("Database name").
				Value(&fb.DB).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("database name is required")
					}
					return nil
				}),
		)).WithTheme(huh.ThemeCharm())

		if err := fieldsForm.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return "", true, nil
			}
			return "", false, err
		}
		built := cmBuildDSN(
			strings.TrimSpace(fb.Host),
			strings.TrimSpace(fb.Port),
			strings.TrimSpace(fb.User),
			fb.Password,
			strings.TrimSpace(fb.DB),
		)
		return built, false, nil
	}
	return "", false, nil
}

// cmBuildDSN constructs a postgres:// DSN from individual fields.
func cmBuildDSN(host, port, user, password, db string) string {
	if port == "" {
		port = "5432"
	}
	// URL-encode password to handle special characters.
	pw := strings.ReplaceAll(password, "@", "%40")
	pw = strings.ReplaceAll(pw, ":", "%3A")
	pw = strings.ReplaceAll(pw, "/", "%2F")
	if pw != "" {
		return fmt.Sprintf("postgres://%s:%s@%s:%s/%s", user, pw, host, port, db)
	}
	return fmt.Sprintf("postgres://%s@%s:%s/%s", user, host, port, db)
}

// parseDSN naively extracts fields from a postgres:// DSN for pre-filling.
// Returns false if the DSN doesn't look like a postgres URL.
func parseDSN(dsn string) (host, port, user, password, db string, ok bool) {
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		return
	}
	// Strip scheme.
	rest := strings.SplitN(dsn, "://", 2)[1]
	// Split userinfo@hostpart/db.
	atIdx := strings.LastIndex(rest, "@")
	if atIdx < 0 {
		return
	}
	userinfo := rest[:atIdx]
	hostdb := rest[atIdx+1:]
	// Userinfo: user[:password].
	if colonIdx := strings.Index(userinfo, ":"); colonIdx >= 0 {
		user = userinfo[:colonIdx]
		password = userinfo[colonIdx+1:]
	} else {
		user = userinfo
	}
	// hostdb: host:port/db or host/db.
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

// cmServerSetupFlow asks how to start the claude-mem server and executes
// the chosen path. Only shown when a DSN was provided (bootstrap needs a server).
func cmServerSetupFlow(cmd *cobra.Command, dsn string, c colors) error {
	out := cmd.OutOrStdout()

	var mode string
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("claude-mem server setup").
			Description("The server must be running for the workspace key to be useful.").
			Options(
				huh.NewOption("I already have a running server — skip this step", "skip"),
				huh.NewOption("Set up docker compose stack  (generates override for external DB)", "compose"),
			).
			Value(&mode),
	)).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Fprintln(out, "\nCancelled.")
			return nil
		}
		return err
	}

	if mode == "compose" {
		return cmComposeSetupFlow(cmd, dsn, c)
	}
	return nil
}

// cmComposeSetupFlow walks the user through docker compose stack startup.
func cmComposeSetupFlow(cmd *cobra.Command, dsn string, c colors) error {
	out := cmd.OutOrStdout()

	home, _ := os.UserHomeDir()
	defaultStack := filepath.Join(home, "dev", "claude-mem-stack")

	var stackDir string
	// Offer the default and allow override.
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("claude-mem stack directory").
			Description("Directory containing docker-compose.yml for the claude-mem server-beta stack.").
			Value(&stackDir).
			Placeholder(defaultStack),
	)).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Fprintln(out, "\nCancelled.")
			return nil
		}
		return err
	}

	if strings.TrimSpace(stackDir) == "" {
		stackDir = defaultStack
	}
	stackDir = strings.TrimSpace(stackDir)
	// Expand ~ manually (shell doesn't expand inside Go string).
	if strings.HasPrefix(stackDir, "~/") {
		stackDir = filepath.Join(home, stackDir[2:])
	}

	// Locate docker-compose.yml walking up from the given directory.
	composeDir, err := cmFindComposeFile(stackDir)
	if err != nil {
		fmt.Fprintf(out, "\n  %s✗%s docker-compose.yml not found in %s (checked up to 3 parents).\n",
			c.red, c.reset, stackDir)
		fmt.Fprintf(out, "  Clone the stack first:\n")
		fmt.Fprintf(out, "    git clone https://github.com/thedotmack/claude-mem ~/dev/claude-mem-stack\n\n")
		return nil
	}
	fmt.Fprintf(out, "\n  %s✓%s Found: %s/docker-compose.yml\n", c.green, c.reset, composeDir)

	// Write the override file.
	overridePath := filepath.Join(composeDir, "docker-compose.override.yml")
	overrideContent := cmGenerateOverride(dsn)
	if err := os.WriteFile(overridePath, []byte(overrideContent), 0600); err != nil {
		return fmt.Errorf("write override: %w", err)
	}
	fmt.Fprintf(out, "  %s✓%s Written: %s\n", c.green, c.reset, overridePath)

	// Run docker compose up -d.
	fmt.Fprintf(out, "  Running: docker compose up -d …\n\n")
	upCmd := exec.Command("docker", "compose", "up", "-d")
	upCmd.Dir = composeDir
	upCmd.Stdout = out
	upCmd.Stderr = out
	if err := upCmd.Run(); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}
	fmt.Fprintf(out, "\n  %s✓%s Stack started.\n\n", c.green, c.reset)
	return nil
}

// cmFindComposeFile looks for docker-compose.yml (or .yaml) starting in dir
// and walking up to 3 parent levels. Returns the directory that contains it.
func cmFindComposeFile(dir string) (string, error) {
	for i := 0; i < 4; i++ {
		for _, name := range []string{"docker-compose.yml", "docker-compose.yaml"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached filesystem root
		}
		dir = parent
	}
	return "", fmt.Errorf("not found")
}

// cmGenerateOverride returns the content of a docker-compose.override.yml
// that injects DATABASE_URL into the claude-mem server and worker services.
func cmGenerateOverride(dsn string) string {
	// Redact password from the in-file comment to avoid leaking plaintext.
	displayDSN := dsn
	if h, p, u, _, db, ok := parseDSN(dsn); ok {
		displayDSN = fmt.Sprintf("postgres://%s:***@%s:%s/%s", u, h, p, db)
	}

	return fmt.Sprintf(`# Generated by: vibrate integrations claude-mem
# Configures an external PostgreSQL database instead of the bundled one.
# DATABASE_URL: %s
#
# To disable the bundled postgres service, find its service name in
# docker-compose.yml and add an entry like this:
#
#   your-postgres-service-name:
#     entrypoint: ["true"]
#     command: []
#     healthcheck:
#       disable: true

services:
  claude-mem-server:
    environment:
      DATABASE_URL: %q
  claude-mem-worker:
    environment:
      DATABASE_URL: %q
`, displayDSN, dsn, dsn)
}

// cmProbeAndBootstrap probes the server then handles workspace key minting.
// Shared tail for both "already running" and "compose setup" paths.
func cmProbeAndBootstrap(cmd *cobra.Command, cfg *prereq.ClaudeMemAdminConfig, c colors) error {
	out := cmd.OutOrStdout()

	// Probe.
	p := prereq.ClaudeMemPrereq(cfg, nil)
	r := p.Verifier.Verify(context.Background())
	fmt.Fprintln(out)
	if r.OK {
		fmt.Fprintf(out, "  %s✓%s Server probe: %s\n", c.green, c.reset, r.Message)
	} else {
		fmt.Fprintf(out, "  %s✗%s Server probe: %s\n", c.red, c.reset, r.Message)
		if r.Hint != "" {
			fmt.Fprintf(out, "      %shint:%s %s\n", c.dim, c.reset, r.Hint)
		}
	}

	if cfg.DatabaseURL == "" {
		fmt.Fprintf(out, "\n  %sNo database URL — skipping bootstrap.%s\n"+
			"  Add one and re-run to auto-mint workspace keys.\n\n", c.dim, c.reset)
		return nil
	}

	wsPath, projectName, pinPath, err := resolveWorkspace()
	if err != nil {
		fmt.Fprintf(out, "\n  %sNo workspace found.%s Run from a project directory to bootstrap a key.\n\n",
			c.dim, c.reset)
		return nil
	}

	fmt.Fprintf(out, "\n  Workspace:  %s\n", wsPath)
	fmt.Fprintf(out, "  Project:    %s\n", projectName)

	cached, hasCached := loadCachedPrereq(pinPath, prereq.ClaudeMemPrereqID)
	if hasCached {
		fmt.Fprintf(out, "  Cached key: %s✓%s present", c.green, c.reset)
		if k := cached["api_key"]; k != "" {
			fmt.Fprintf(out, " (%s…)", k[:min(12, len(k))])
		}
		fmt.Fprintln(out)

		var rotate bool
		if err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title("A key is already cached. Rotate it?").
				Description("Revokes the old key and mints a fresh one.").
				Value(&rotate),
		)).WithTheme(huh.ThemeCharm()).Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return err
		}
		if !rotate {
			return nil
		}
	} else {
		var doBootstrap bool
		if err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title("Bootstrap an API key for this workspace?").
				Description("Runs a one-shot postgres container to mint a project-scoped key.\n" +
					"The key is saved to .vb (gitignored). The DSN never enters the container.").
				Value(&doBootstrap),
		)).WithTheme(huh.ThemeCharm()).Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return err
		}
		if !doBootstrap {
			fmt.Fprintf(out, "\n  Skipped. Run %svibrate integrations claude-mem%s from the workspace later.\n\n",
				c.bold, c.reset)
			return nil
		}
	}

	dc, err := docker.NewCLIClient()
	if err != nil {
		return err
	}
	p2 := prereq.ClaudeMemPrereq(cfg, dc)
	if p2.Bootstrapper == nil {
		return errors.New("bootstrapper unavailable — check database URL")
	}

	hostname, _ := os.Hostname()
	ws := prereq.Workspace{Path: wsPath, ProjectName: projectName, Hostname: hostname}
	effectiveTeam := or(cfg.TeamName, "vibrators")
	fmt.Fprintf(out, "\n  Bootstrapping key for project=%q on team=%q …\n", projectName, effectiveTeam)

	result, err := p2.Bootstrapper.Bootstrap(context.Background(), ws)
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	if err := persistPrereqResult(pinPath, prereq.ClaudeMemPrereqID, result); err != nil {
		return fmt.Errorf("persist: %w", err)
	}

	changed, err := config.AppendToGitignore(filepath.Dir(pinPath))
	if err != nil {
		fmt.Fprintf(out, "  %swarning: could not update .gitignore: %v%s\n", c.yellow, err, c.reset)
	}
	fmt.Fprintf(out, "  %s✓%s team_id    = %s\n", c.green, c.reset, result["team_id"])
	fmt.Fprintf(out, "  %s✓%s project_id = %s\n", c.green, c.reset, result["project_id"])
	if k := result["api_key"]; k != "" {
		fmt.Fprintf(out, "  %s✓%s api_key    = %s… (length %d)\n", c.green, c.reset, k[:min(12, len(k))], len(k))
	}
	fmt.Fprintf(out, "  %s✓%s key cached in %s [prereqs.%s]\n", c.green, c.reset, pinPath, prereq.ClaudeMemPrereqID)
	if changed {
		fmt.Fprintf(out, "  %s✓%s .vb added to .gitignore\n", c.green, c.reset)
	}
	fmt.Fprintf(out, "\n  Next: %svibrate --rebuild%s to pick up the new key in the container.\n\n",
		c.bold, c.reset)
	return nil
}
