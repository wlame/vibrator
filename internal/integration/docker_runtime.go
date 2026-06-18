package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/wlame/vibrator/internal/docker"
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

	// Env is the container env-var map. Values are passed via a 0600
	// --env-file (see buildRunFlags), never as -e KEY=VALUE argv, so
	// secrets in user-authored integration configs don't leak to ps.
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
		return runDockerCmd(ctx, nil, "start", d.ContainerName)
	}

	flags, env := d.buildRunFlags()

	// Same env-delivery mechanism as internal/docker's CLIClient.Run: a
	// private (0600) temp file docker reads directly via --env-file, so
	// values reach the container even when something between us and the
	// real docker client (e.g. a sudo wrapper with env_reset) strips
	// cmd.Env before it runs. See docker.WriteEnvFile's doc comment.
	envFile, cleanup, err := docker.WriteEnvFile(env)
	if err != nil {
		return fmt.Errorf("docker run: %w", err)
	}
	defer cleanup()
	if envFile != "" {
		flags = append(flags, "--env-file", envFile)
	}

	args := append(flags, d.Image)
	args = append(args, d.Command...)
	args = append(args, d.Args...)

	return runDockerCmd(ctx, env, args...)
}

// buildRunFlags returns every `docker run` flag for this container EXCEPT
// the trailing image + command (mirrors internal/docker's
// buildRunFlags/buildRunArgs split), plus the EnvVar list carrying the
// actual values. Env values are NEVER embedded in the returned flags —
// only "-e NAME" (bare name) appears in argv; NAME's value travels via the
// returned EnvVar slice, which the caller feeds into docker.WriteEnvFile
// and the subprocess environment instead. Split out from Start so tests
// can assert on the argv shape without shelling out to docker.
func (d *DockerRuntime) buildRunFlags() (flags []string, env []docker.EnvVar) {
	flags = []string{"run", "-d", "--name", d.ContainerName}
	if d.Restart != "" {
		flags = append(flags, "--restart", d.Restart)
	}
	for _, p := range d.Ports {
		flags = append(flags, "-p", p)
	}
	for _, v := range d.Volumes {
		flags = append(flags, "-v", expandHome(v))
	}
	// Sort env keys so the docker run argument list is deterministic
	// regardless of map iteration order.
	envKeys := make([]string, 0, len(d.Env))
	for k := range d.Env {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	env = make([]docker.EnvVar, 0, len(envKeys))
	for _, k := range envKeys {
		flags = append(flags, "-e", k)
		env = append(env, docker.EnvVar{Name: k, Value: d.Env[k]})
	}
	for _, h := range d.AddHosts {
		flags = append(flags, "--add-host", h)
	}
	if d.Network != "" {
		flags = append(flags, "--network", d.Network)
	}
	return flags, env
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
		if err := runDockerCmd(ctx, nil, "stop", d.ContainerName); err != nil {
			return fmt.Errorf("docker stop: %w", err)
		}
	}
	if err := runDockerCmd(ctx, nil, "rm", d.ContainerName); err != nil {
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
// command output anywhere. env is the belt-and-suspenders companion to
// any name-only "-e NAME" flags already present in args (see
// buildRunFlags): docker resolves each such name against ITS OWN process
// environment, which we supply here via cmd.Env, mirroring
// internal/docker's CLIClient.Run/Exec. Pass nil when args carries no -e
// flags (e.g. "start"/"stop"/"rm", where env was already baked into the
// container on its original run).
func runDockerCmd(ctx context.Context, env []docker.EnvVar, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	if len(env) > 0 {
		pairs := make([]string, 0, len(env))
		for _, e := range env {
			pairs = append(pairs, e.Name+"="+e.Value)
		}
		cmd.Env = append(os.Environ(), pairs...)
	}
	out, err := cmd.CombinedOutput()
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
