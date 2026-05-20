// Package claudemem registers the claude-mem Integration with the
// global integration registry.
//
// Step 1 wraps the existing prereq.ClaudeMem* logic as Integration +
// WorkspaceDriver. The existing CLI flow in
// internal/cli/integrations_claudemem.go still owns the interactive
// setup and bootstrapping today; this descriptor only makes the
// integration discoverable through the registry. Migrating the CLI
// flow to the generic runner lives in a later step.
package claudemem

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/integration"
	"github.com/wlame/vibrator/internal/prereq"
)

// init registers the claude-mem Integration. Triggered once per
// program start, via the blank import in cli/integrations_list.go.
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
			// Step 1 treats claude-mem as externally managed: the
			// existing CLI flow walks the user through a
			// docker-compose stack. A dedicated ComposeRuntime that
			// understands the override-file dance will replace this
			// in a later step.
			&integration.ExternalRuntime{
				Instructions: "claude-mem requires the server-beta stack " +
					"(Postgres + worker + server). " +
					"Run `vibrate integrations claude-mem` for guided setup.",
			},
		},
		ProbeFn: func(_ context.Context) (integration.Probe, error) {
			cfg, err := prereq.LoadClaudeMemAdminConfig()
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return nil, nil // not configured yet — skip probe
				}
				return nil, err
			}
			if cfg == nil || cfg.ServerURL == "" {
				return nil, nil
			}
			// Use the existing healthz endpoint that the
			// prereq.HTTPVerify already probes; reuse its
			// host.docker.internal → 127.0.0.1 rewrite by going
			// through prereq.ClaudeMemPrereq for the URL? Simpler:
			// rebuild the URL the same way the CLI does.
			return integration.HTTPProbe{URL: cfg.ServerURL + "/healthz"}, nil
		},
		// claude-mem doesn't add an MCP entry — it integrates via the
		// server-beta runtime hook + workspace-mounted API key. Wiring
		// stays empty for step 1.
		AdminConfig: &integration.AdminConfigSchema{
			Path: prereq.ClaudeMemAdminConfigPath(),
		},
		Workspace: &workspaceDriver{},
	}
}

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

// Rotate currently delegates to Bootstrap. The underlying
// ClaudeMemBootstrap.Bootstrap is already rotate-safe — it revokes the
// existing live key for (team, project, actor) and inserts a fresh one
// in a single transaction. A dedicated Rotate path can be split out
// later if the semantics ever diverge.
func (d *workspaceDriver) Rotate(ctx context.Context, ws integration.Workspace, _ map[string]string) (map[string]string, error) {
	return d.Bootstrap(ctx, ws)
}
