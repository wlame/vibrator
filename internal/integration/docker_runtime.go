package integration

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DockerRuntime is a generic HostRuntime backed by a single docker
// container. Used by user-defined TOML integrations and by any
// in-tree integration whose host runtime is "spawn this image with
// these ports/volumes/env" — i.e., most MCP-server-as-a-container
// cases.
//
// Integrations that need richer orchestration (multi-container
// compose stacks, host-side daemon supervision) supply their own
// HostRuntime instead — see ComposeRuntime, serena.processRuntime.
type DockerRuntime struct {
	// Image is the container image reference (with tag).
	Image string

	// ContainerName is the docker --name. Required — DockerRuntime
	// addresses the container by name across Start/Stop/Status, so
	// anonymous containers don't fit.
	ContainerName string

	// Ports are passed verbatim as -p arguments. Use host:container
	// form (e.g., "127.0.0.1:9100:9100").
	Ports []string

	// Volumes are passed verbatim as -v arguments. Paths beginning
	// with "~/" are expanded against $HOME at Start time.
	Volumes []string

	// Env is the container env-var map (-e KEY=VALUE for each entry).
	Env map[string]string

	// AddHosts are passed verbatim as --add-host arguments. Use this
	// to map host.docker.internal on Linux:
	// AddHosts: []string{"host.docker.internal:host-gateway"}.
	AddHosts []string

	// Restart is the --restart policy. "unless-stopped" is the
	// recommended default for long-running MCP-server containers.
	// Empty disables --restart.
	Restart string

	// Network is the --network argument (empty leaves the default).
	Network string

	// Command (and Args) override the image's CMD. Both optional.
	Command []string
	Args    []string

	// LabelHint, when non-empty, is shown in RuntimeStatus.Detail to
	// help users disambiguate runtime modes in the manage menu.
	LabelHint string
}

// Kind implements HostRuntime. Always "docker".
func (d *DockerRuntime) Kind() string { return "docker" }

// Label implements HostRuntime. Adds Restart policy detail when set.
func (d *DockerRuntime) Label() string {
	if d.Restart != "" {
		return fmt.Sprintf("Docker container (--restart=%s)", d.Restart)
	}
	return "Docker container"
}

// Status implements HostRuntime. Reports Running=true when
// `docker inspect` says the container's State.Running is true.
func (d *DockerRuntime) Status(ctx context.Context) (RuntimeStatus, error) {
	out, err := exec.CommandContext(ctx, "docker", "inspect",
		"--format={{.State.Running}}", d.ContainerName).Output()
	if err != nil {
		// Container doesn't exist or daemon is unreachable. Either
		// way, "not running" is the right answer — not an error.
		return RuntimeStatus{}, nil
	}
	running := strings.TrimSpace(string(out)) == "true"
	return RuntimeStatus{
		Running:   running,
		Container: d.ContainerName,
		Detail:    d.LabelHint,
	}, nil
}

// Start implements HostRuntime. If the container already exists (running
// or not), uses `docker start <name>` to (re-)launch it; otherwise runs
// a fresh `docker run -d` with the configured flags.
func (d *DockerRuntime) Start(ctx context.Context) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not on PATH")
	}

	exists, running := d.inspectExistence(ctx)
	if running {
		return nil
	}
	if exists {
		return runDockerCmd(ctx, "start", d.ContainerName)
	}

	args := []string{"run", "-d", "--name", d.ContainerName}
	if d.Restart != "" {
		args = append(args, "--restart", d.Restart)
	}
	for _, p := range d.Ports {
		args = append(args, "-p", p)
	}
	for _, v := range d.Volumes {
		args = append(args, "-v", expandHome(v))
	}
	for k, v := range d.Env {
		args = append(args, "-e", k+"="+v)
	}
	for _, h := range d.AddHosts {
		args = append(args, "--add-host", h)
	}
	if d.Network != "" {
		args = append(args, "--network", d.Network)
	}
	args = append(args, d.Image)
	args = append(args, d.Command...)
	args = append(args, d.Args...)

	return runDockerCmd(ctx, args...)
}

// Stop implements HostRuntime. Stops the container and removes it so
// a future Start gets a fresh state. Idempotent — no-ops when the
// container doesn't exist.
func (d *DockerRuntime) Stop(ctx context.Context) error {
	exists, running := d.inspectExistence(ctx)
	if !exists {
		return nil
	}
	if running {
		if err := runDockerCmd(ctx, "stop", d.ContainerName); err != nil {
			return fmt.Errorf("docker stop: %w", err)
		}
	}
	if err := runDockerCmd(ctx, "rm", d.ContainerName); err != nil {
		return fmt.Errorf("docker rm: %w", err)
	}
	return nil
}

// Logs implements HostRuntime. Returns the last maxBytes of the
// container's stdout+stderr. Empty + nil when the container doesn't
// exist.
func (d *DockerRuntime) Logs(ctx context.Context, maxBytes int64) (string, error) {
	exists, _ := d.inspectExistence(ctx)
	if !exists {
		return "", nil
	}
	out, err := exec.CommandContext(ctx, "docker", "logs", "--tail=80", d.ContainerName).CombinedOutput()
	if err != nil {
		return "", err
	}
	if int64(len(out)) > maxBytes {
		out = out[int64(len(out))-maxBytes:]
	}
	return string(out), nil
}

// inspectExistence runs `docker inspect` and returns (exists, running).
// Used to make Start / Stop idempotent without races.
func (d *DockerRuntime) inspectExistence(ctx context.Context) (exists, running bool) {
	out, err := exec.CommandContext(ctx, "docker", "inspect",
		"--format={{.State.Running}}", d.ContainerName).Output()
	if err != nil {
		return false, false
	}
	return true, strings.TrimSpace(string(out)) == "true"
}

// runDockerCmd is a tiny shell helper — combines stdout+stderr into
// the error message so failures are debuggable without us routing
// command output anywhere.
func runDockerCmd(ctx context.Context, args ...string) error {
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %w\n%s", strings.Join(args, " "), err,
			strings.TrimSpace(string(out)))
	}
	return nil
}

// expandHome rewrites a leading "~/" or "~" in p against $HOME. Paths
// without that prefix pass through untouched. Used for the Volumes
// list so authors can write "~/.config/foo" naturally.
func expandHome(p string) string {
	if p == "~" {
		if h := homeOrEmpty(); h != "" {
			return h
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if h := homeOrEmpty(); h != "" {
			return h + p[1:]
		}
	}
	return p
}
