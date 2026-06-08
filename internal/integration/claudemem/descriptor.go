// Package claudemem registers the claude-mem Integration with the
// global integration registry.
//
// claude-mem is the canonical "compose-stack with workspace
// credentials" case: the host runs a docker-compose stack
// (Postgres + worker + server), the host has a privileged DSN, and
// every workspace mints its own bearer token. The descriptor wires
// those concerns together:
//
//   - Runtime: ComposeRuntime (the stack) + ExternalRuntime (escape
//     hatch for users who orchestrate elsewhere).
//   - Probe: HTTP probe of the configured server URL.
//   - Workspace: adapts prereq.ClaudeMemBootstrap.
//
// The CLI flow in internal/cli/integrations_claudemem.go runs the
// admin-config form (URL / DSN / stack dir) FIRST, then delegates to
// the generic runIntegration runner using this descriptor.
package claudemem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/integration"
	"github.com/wlame/vibrator/internal/prereq"
)

// defaultStackDir is the path users see suggested in the setup form
// and the implicit fallback for the ComposeRuntime when no admin
// config exists. Matches the convention used in the existing CLI.
const defaultStackDir = "~/dev/claude-mem-stack"

// ComposeServices is the list of services that MUST be running for
// claude-mem to be considered "up". Read by the ComposeRuntime's
// Status check. Exported so the CLI layer can reference the same
// names in error messages.
var ComposeServices = []string{"claude-mem-server", "claude-mem-worker"}

func init() {
	integration.Register(descriptor())
}

func descriptor() *integration.Integration {
	return &integration.Integration{
		ID:       "claude-mem",
		Name:     "claude-mem",
		Summary:  "Persistent memory for AI agents (server-beta runtime)",
		DocsURL:  "https://github.com/thedotmack/claude-mem",
		Category: "memory",
		Runtimes: []integration.HostRuntime{
			&dynamicComposeRuntime{},
			&integration.ExternalRuntime{
				Instructions: "claude-mem's stack is up under your control. " +
					"vibrator will probe the configured server URL on every shell " +
					"entry and write a workspace key via the postgres bootstrap.",
			},
		},
		ProbeFn:      probeFn,
		AdminConfig:  &integration.AdminConfigSchema{Path: prereq.ClaudeMemAdminConfigPath()},
		Workspace:    &workspaceDriver{},
		LaunchChecks: claudeMemLaunchChecks(),
	}
}

// claudeMemLaunchChecks returns the three pre-launch readiness checks for
// the claude-mem integration. All checks are skipped (OK=true) when the
// claude-mem extension is not selected for the workspace.
//
//  1. admin-config  — ~/.config/vibrator/claude-mem.toml present and has a
//     server_url. Without it, CLAUDE_MEM_* env vars are never emitted and
//     the plugin is permanently dormant.
//
//  2. server-probe  — the configured server URL answers /healthz. Without
//     this, settings.json will be written but every observation call will
//     fail silently.
//
//  3. workspace-key — [prereqs.claude-mem-server-beta] in .vb has an
//     api_key. Without it, the container has no bearer token and all
//     authenticated requests return 401. Offers an inline bootstrap.
func claudeMemLaunchChecks() []integration.LaunchCheck {
	return []integration.LaunchCheck{
		{
			ID: "admin-config",
			Check: func(_ context.Context, lc integration.LaunchCheckContext) integration.LaunchCheckResult {
				if !hasExt(lc.Extensions, "claude-mem") {
					return integration.LaunchCheckResult{OK: true}
				}
				cfg, err := prereq.LoadClaudeMemAdminConfig()
				if errors.Is(err, os.ErrNotExist) {
					return integration.LaunchCheckResult{
						Message: "claude-mem admin config not found — CLAUDE_MEM_* vars will not be forwarded",
						Hint:    "the admin config holds the server URL, runtime, and database DSN",
						FixCmd:  "vibrate integrations claude-mem",
					}
				}
				if err != nil {
					return integration.LaunchCheckResult{
						Message: fmt.Sprintf("claude-mem admin config unreadable: %v", err),
						FixCmd:  "vibrate integrations claude-mem",
					}
				}
				if cfg == nil || strings.TrimSpace(cfg.ServerURL) == "" {
					return integration.LaunchCheckResult{
						Message: "claude-mem admin config has no server_url — CLAUDE_MEM_SERVER_BETA_URL will be empty",
						FixCmd:  "vibrate integrations claude-mem",
					}
				}
				return integration.LaunchCheckResult{OK: true}
			},
		},
		{
			ID: "server-probe",
			Check: func(ctx context.Context, lc integration.LaunchCheckContext) integration.LaunchCheckResult {
				if !hasExt(lc.Extensions, "claude-mem") {
					return integration.LaunchCheckResult{OK: true}
				}
				cfg, err := prereq.LoadClaudeMemAdminConfig()
				if err != nil || cfg == nil || cfg.ServerURL == "" {
					return integration.LaunchCheckResult{OK: true} // admin-config check handles this
				}
				probeURL := rewriteForHostProbe(cfg.ServerURL) + "/healthz"
				probe := integration.HTTPProbe{URL: probeURL}
				if probe.Check(ctx) == nil {
					return integration.LaunchCheckResult{OK: true}
				}
				return integration.LaunchCheckResult{
					Message: fmt.Sprintf("claude-mem server not reachable at %s", probeURL),
					Hint:    "memory observations will silently fail until the server is up",
					FixCmd:  "vibrate integrations claude-mem",
				}
			},
		},
		{
			ID: "workspace-key",
			Check: func(_ context.Context, lc integration.LaunchCheckContext) integration.LaunchCheckResult {
				if !hasExt(lc.Extensions, "claude-mem") {
					return integration.LaunchCheckResult{OK: true}
				}
				cached := lc.Prereqs[prereq.ClaudeMemPrereqID]
				if len(cached) > 0 && cached["api_key"] != "" {
					return integration.LaunchCheckResult{OK: true}
				}
				return integration.LaunchCheckResult{
					Message: "no workspace key for claude-mem — all auth'd requests will return 401",
					Hint:    "a project-scoped bearer token must be minted against the host postgres",
					FixCmd:  "vibrate prereqs bootstrap " + prereq.ClaudeMemPrereqID,
					FixNow: func(ctx context.Context, lc integration.LaunchCheckContext) (string, map[string]string, error) {
						cfg, err := prereq.LoadClaudeMemAdminConfig()
						if err != nil {
							return "", nil, fmt.Errorf("load admin config: %w", err)
						}
						if cfg.DatabaseURL == "" {
							return "", nil, fmt.Errorf(
								"admin config at %s has no database_url — set it with: vibrate integrations claude-mem",
								prereq.ClaudeMemAdminConfigPath())
						}
						dc, err := docker.NewCLIClient()
						if err != nil {
							return "", nil, err
						}
						p := prereq.ClaudeMemPrereq(cfg, dc)
						result, err := p.Bootstrapper.Bootstrap(ctx, prereq.Workspace{
							Path:        lc.WsDir,
							ProjectName: lc.ProjectName,
							Hostname:    lc.Hostname,
						})
						if err != nil {
							return "", nil, err
						}
						return prereq.ClaudeMemPrereqID, result, nil
					},
				}
			},
		},
	}
}

// hasExt reports whether id is present in the extensions slice.
func hasExt(exts []string, id string) bool {
	for _, e := range exts {
		if e == id {
			return true
		}
	}
	return false
}

// probeFn loads the admin config and returns an HTTP probe at the
// configured server URL. Returns (nil, nil) when the admin config is
// missing or has no server URL — the generic runner treats that as
// "skip the reachability check".
func probeFn(_ context.Context) (integration.Probe, error) {
	cfg, err := prereq.LoadClaudeMemAdminConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if cfg == nil || strings.TrimSpace(cfg.ServerURL) == "" {
		return nil, nil
	}
	// The stored URL is container-shaped (host.docker.internal:port).
	// Probes run from the host, where that DNS name doesn't resolve —
	// rewrite to 127.0.0.1 for probing. Append /healthz which the
	// claude-mem server beta exposes.
	probeURL := rewriteForHostProbe(cfg.ServerURL) + "/healthz"
	return integration.HTTPProbe{URL: probeURL}, nil
}

// rewriteForHostProbe converts a host.docker.internal URL into a
// 127.0.0.1 URL for host-side probing. The replace is naive
// (substring) but safe because the marker doesn't appear in real
// hostnames. Kept in this package to avoid a circular dep with
// internal/prereq.
func rewriteForHostProbe(url string) string {
	return strings.Replace(url, "host.docker.internal", "127.0.0.1", 1)
}

// dynamicComposeRuntime is a thin facade around ComposeRuntime that
// re-loads the admin config on every method call. Without this
// indirection we'd snapshot the stack dir at init() time — a stale
// value would persist across `vibrate integrations claude-mem`
// invocations that change StackDir.
//
// Each method constructs a fresh ComposeRuntime from the current
// admin config and forwards. The underlying ComposeRuntime methods
// already tolerate missing dirs/files, so we don't have to special-
// case "config not loaded".
type dynamicComposeRuntime struct{}

func (d *dynamicComposeRuntime) Kind() string  { return "compose" }
func (d *dynamicComposeRuntime) Label() string { return "Docker Compose stack (multi-container, persistent)" }

func (d *dynamicComposeRuntime) Status(ctx context.Context) (integration.RuntimeStatus, error) {
	return composeForCurrentConfig().Status(ctx)
}
func (d *dynamicComposeRuntime) Start(ctx context.Context) error {
	return composeForCurrentConfig().Start(ctx)
}
func (d *dynamicComposeRuntime) Stop(ctx context.Context) error {
	return composeForCurrentConfig().Stop(ctx)
}
func (d *dynamicComposeRuntime) Logs(ctx context.Context, maxBytes int64) (string, error) {
	return composeForCurrentConfig().Logs(ctx, maxBytes)
}

// composeForCurrentConfig builds a ComposeRuntime sized to the
// current admin config. Missing config → ComposeRuntime with empty
// Dir, whose Status/Start/Stop/Logs all degrade gracefully.
func composeForCurrentConfig() *integration.ComposeRuntime {
	cfg, _ := prereq.LoadClaudeMemAdminConfig()
	stackDir := defaultStackDir
	dsn := ""
	if cfg != nil {
		if cfg.StackDir != "" {
			stackDir = cfg.StackDir
		}
		dsn = cfg.DatabaseURL
	}
	return &integration.ComposeRuntime{
		Dir:              stackDir,
		OverrideFilename: "docker-compose.override.yml",
		OverrideContent: func() (string, error) {
			if dsn == "" {
				return "", nil // no DSN → no override needed
			}
			return GenerateOverride(dsn), nil
		},
		Services:     ComposeServices,
		StatusDetail: "stack at " + stackDir,
	}
}

// GenerateOverride returns the docker-compose.override.yml content
// that injects DATABASE_URL into the claude-mem server and worker
// services. Used both by the ComposeRuntime (Start path) and by the
// CLI flow (when displaying setup state). Public so tests can pin
// the format.
func GenerateOverride(dsn string) string {
	// Redact password from the human-readable comment block.
	displayDSN := dsn
	if u := parsePostgresURL(dsn); u != nil {
		displayDSN = fmt.Sprintf("postgres://%s:***@%s:%s/%s", u.user, u.host, u.port, u.db)
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

// pgURL is a tiny shape for password-redacted display. We only use
// the user/host/port/db fields, so password isn't even captured.
type pgURL struct {
	user, host, port, db string
}

// parsePostgresURL is the bare-minimum DSN parser used for the
// password-redacted comment. Returns nil on anything that doesn't
// look like postgres://user[:pw]@host[:port]/db.
func parsePostgresURL(dsn string) *pgURL {
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		return nil
	}
	rest := strings.SplitN(dsn, "://", 2)[1]
	atIdx := strings.LastIndex(rest, "@")
	if atIdx < 0 {
		return nil
	}
	userInfo, hostDB := rest[:atIdx], rest[atIdx+1:]
	user := userInfo
	if colon := strings.Index(userInfo, ":"); colon >= 0 {
		user = userInfo[:colon]
	}
	slash := strings.Index(hostDB, "/")
	if slash < 0 {
		return nil
	}
	db := hostDB[slash+1:]
	hostPort := hostDB[:slash]
	host, port := hostPort, "5432"
	if colon := strings.LastIndex(hostPort, ":"); colon >= 0 {
		host = hostPort[:colon]
		port = hostPort[colon+1:]
	}
	if host == "" || user == "" || db == "" {
		return nil
	}
	return &pgURL{user: user, host: host, port: port, db: db}
}

// ── WorkspaceDriver ─────────────────────────────────────────────────────

// workspaceDriver adapts prereq.ClaudeMemBootstrap as an
// integration.WorkspaceDriver. The existing implementation in
// internal/prereq is full-fat — this adapter just translates the
// shapes (integration.Workspace ↔ prereq.Workspace).
type workspaceDriver struct{}

func (d *workspaceDriver) PrereqID() string { return prereq.ClaudeMemPrereqID }

func (d *workspaceDriver) Bootstrap(ctx context.Context, ws integration.Workspace) (map[string]string, error) {
	cfg, err := prereq.LoadClaudeMemAdminConfig()
	if err != nil {
		return nil, fmt.Errorf("load admin config: %w", err)
	}
	if cfg == nil {
		return nil, errors.New("admin config missing — run `vibrate integrations claude-mem`")
	}
	dc, err := docker.NewCLIClient()
	if err != nil {
		return nil, err
	}
	p := prereq.ClaudeMemPrereq(cfg, dc)
	if p.Bootstrapper == nil {
		return nil, errors.New("bootstrapper unavailable — check database URL in admin config")
	}
	return p.Bootstrapper.Bootstrap(ctx, prereq.Workspace{
		Path:        ws.Path,
		ProjectName: ws.ProjectName,
		Hostname:    ws.Hostname,
	})
}

// Rotate delegates to Bootstrap. The underlying ClaudeMemBootstrap is
// already rotate-safe — it revokes any live (team, project, actor)
// key and inserts a fresh one in a single transaction.
func (d *workspaceDriver) Rotate(ctx context.Context, ws integration.Workspace, _ map[string]string) (map[string]string, error) {
	return d.Bootstrap(ctx, ws)
}

// composeFileExists is duplicated from internal/integration here only
// so the CLI's pre-flight check can use it without exporting the
// helper. Kept tiny.
func composeFileExists(dir string) bool {
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// ComposeFileExists is the exported sibling — callers in the CLI
// layer use it to short-circuit setup when the stack hasn't been
// cloned yet.
func ComposeFileExists(dir string) bool { return composeFileExists(dir) }
