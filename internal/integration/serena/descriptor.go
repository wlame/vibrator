package serena

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/wlame/vibrator/internal/integration"
)

// init registers the Serena Integration with the global registry. This
// runs once at program start (whenever any package imports this one,
// even with a blank `_` import — see cli/integrations_list.go).
func init() {
	integration.Register(descriptor())
}

// descriptor builds the Integration value for Serena. Wrapped in a
// function (rather than a package-level var) so the SERENA_PORT env var
// is consulted at init() time, when the binary's environment is
// already resolved.
func descriptor() *integration.Integration {
	port := portFromEnv()
	return &integration.Integration{
		ID:       "serena",
		Name:     "Serena MCP",
		Summary:  "Code-aware MCP server for project navigation & symbol search",
		DocsURL:  "https://github.com/oraios/serena",
		Category: "mcp-tools",
		Runtimes: []integration.HostRuntime{
			&processRuntime{port: port},
			&dockerRuntime{port: port},
		},
		ProbeFn: func(_ context.Context) (integration.Probe, error) {
			return integration.HTTPProbe{
				URL:     fmt.Sprintf("http://127.0.0.1:%d/mcp", port),
				Timeout: 2 * time.Second,
			}, nil
		},
		Wiring: []integration.Wiring{{
			Harness: "claudecode",
			MCP: &integration.MCPWiring{
				Name:      "serena",
				Transport: "http",
				URL:       fmt.Sprintf("http://host.docker.internal:%d/mcp", port),
			},
		}},
	}
}

// portFromEnv returns SERENA_PORT if set to a positive integer, else
// DefaultPort. Mirrors the resolveSerenaPort helper in the CLI package
// (kept duplicated here to avoid an import cycle).
func portFromEnv() int {
	if s := os.Getenv("SERENA_PORT"); s != "" {
		if p, err := strconv.Atoi(s); err == nil && p > 0 {
			return p
		}
	}
	return DefaultPort
}

// ── HostRuntime: process ────────────────────────────────────────────────

// processRuntime wraps the detached-uvx-process daemon (Start/Stop/Read
// in this package's daemon.go) as a HostRuntime.
type processRuntime struct{ port int }

func (p *processRuntime) Kind() string  { return "process" }
func (p *processRuntime) Label() string { return "Background process (uvx, survives terminal close)" }

func (p *processRuntime) Status(_ context.Context) (integration.RuntimeStatus, error) {
	state, err := Read(p.port)
	if err != nil {
		return integration.RuntimeStatus{}, err
	}
	return integration.RuntimeStatus{
		Running: state.Status == StatusRunning,
		PID:     state.PID,
	}, nil
}

func (p *processRuntime) Start(_ context.Context) error {
	_, err := Start(p.port)
	return err
}

func (p *processRuntime) Stop(_ context.Context) error {
	state, err := Read(p.port)
	if err != nil {
		return err
	}
	if state.Status != StatusRunning {
		return nil
	}
	return Stop(state)
}

func (p *processRuntime) Logs(_ context.Context, maxBytes int64) (string, error) {
	return TailLog(maxBytes)
}

// ── HostRuntime: docker ─────────────────────────────────────────────────

// dockerRuntime wraps the Docker container variant (ContainerRunning,
// StartDocker, StopDocker in this package's daemon.go) as a HostRuntime.
type dockerRuntime struct{ port int }

func (d *dockerRuntime) Kind() string  { return "docker" }
func (d *dockerRuntime) Label() string { return "Docker container (auto-restarts on boot)" }

func (d *dockerRuntime) Status(_ context.Context) (integration.RuntimeStatus, error) {
	return integration.RuntimeStatus{
		Running:   ContainerRunning(),
		Container: ContainerName,
		Detail:    "auto-restart unless-stopped",
	}, nil
}

func (d *dockerRuntime) Start(_ context.Context) error { return StartDocker(d.port) }
func (d *dockerRuntime) Stop(_ context.Context) error  { return StopDocker() }

func (d *dockerRuntime) Logs(ctx context.Context, maxBytes int64) (string, error) {
	// docker logs --tail=N is plenty for the inspector flow; we trim to
	// maxBytes after the fact to match the TailLog contract.
	out, err := exec.CommandContext(ctx, "docker", "logs", "--tail=40", ContainerName).CombinedOutput()
	if err != nil {
		return "", err
	}
	if int64(len(out)) > maxBytes {
		out = out[int64(len(out))-maxBytes:]
	}
	return string(out), nil
}
