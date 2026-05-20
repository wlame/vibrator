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
			&integration.ExternalRuntime{
				Instructions: "Start Serena yourself (e.g., via systemd, launchd, " +
					"or `screen`) — see https://github.com/oraios/serena for options. " +
					"The container will pick up the http transport automatically " +
					"on next shell entry once the server is reachable.",
			},
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
				Name: "serena",
				HTTP: &integration.MCPHTTP{
					URL: fmt.Sprintf("http://host.docker.internal:%d/mcp", port),
				},
				// Stdio fallback: when the host server is unreachable
				// the container spawns Serena locally via uvx. Same
				// command the old claude-exec.sh hardcoded.
				Stdio: &integration.MCPStdio{
					Command: []string{
						"uvx", "--from", "git+https://github.com/oraios/serena",
						"serena", "start-mcp-server", "--project-from-cwd",
					},
				},
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

func (p *processRuntime) Kind() string { return "process" }
func (p *processRuntime) Label() string {
	return "Background process (uvx, survives terminal close)"
}

func (p *processRuntime) Status(_ context.Context) (integration.RuntimeStatus, error) {
	state, err := Read(p.port)
	if err != nil {
		return integration.RuntimeStatus{}, err
	}
	return integration.RuntimeStatus{
		Running: state.Status == StatusRunning,
		PID:     state.PID,
		Detail:  "log: " + state.LogFile,
	}, nil
}

func (p *processRuntime) Start(_ context.Context) error {
	// Idempotent — if already running, return nil.
	state, err := Read(p.port)
	if err != nil {
		return err
	}
	if state.Status == StatusRunning {
		return nil
	}
	// Pre-check uvx availability — gives a clearer error than the
	// exec failure inside Start().
	if _, err := exec.LookPath("uvx"); err != nil {
		return fmt.Errorf("uvx not found on PATH (install uv: " +
			"curl -LsSf https://astral.sh/uv/install.sh | sh)")
	}
	_, err = Start(p.port)
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

func (d *dockerRuntime) Start(_ context.Context) error {
	if ContainerRunning() {
		return nil
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not found on PATH")
	}
	if ContainerExists() {
		// Restart the existing container instead of failing.
		return exec.Command("docker", "start", ContainerName).Run()
	}
	return StartDocker(d.port)
}

func (d *dockerRuntime) Stop(_ context.Context) error {
	if !ContainerRunning() && !ContainerExists() {
		return nil
	}
	return StopDocker()
}

func (d *dockerRuntime) Logs(ctx context.Context, maxBytes int64) (string, error) {
	if !ContainerExists() {
		return "", nil
	}
	out, err := exec.CommandContext(ctx, "docker", "logs", "--tail=40", ContainerName).CombinedOutput()
	if err != nil {
		return "", err
	}
	if int64(len(out)) > maxBytes {
		out = out[int64(len(out))-maxBytes:]
	}
	return string(out), nil
}
