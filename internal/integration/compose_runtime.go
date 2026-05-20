package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ComposeRuntime is a HostRuntime backed by a docker-compose stack —
// multiple containers + networks + volumes orchestrated together.
// Used by claude-mem (Postgres + worker + server) and any future
// integration whose host runtime is "git clone a stack, docker compose
// up". User-defined TOML integrations CAN'T declare a ComposeRuntime
// today (the schema doesn't expose it); add it there if/when needed.
type ComposeRuntime struct {
	// Dir is the directory containing docker-compose.yml. Required.
	// May start with "~" — expanded against $HOME at use time.
	Dir string

	// OverrideFilename is the basename for the generated override file
	// (typically "docker-compose.override.yml"). Empty disables
	// override file generation entirely.
	OverrideFilename string

	// OverrideContent generates the override file content at Start
	// time. Called fresh each Start so DSN/env updates are picked up
	// without a rebuild. Returning ("", nil) skips override emission
	// (useful when the override is conditional on user state). When
	// nil, no override file is written.
	OverrideContent func() (string, error)

	// Project is the --project-name. Empty defaults to docker compose's
	// own basename-of-dir resolution. Set explicitly to keep stack
	// identity stable across renames.
	Project string

	// Services optionally constrains Status's "is it up?" check.
	// Empty means "any service running counts as up". Set to the
	// services that NEED to be alive for the integration to be
	// considered ready (e.g., {"claude-mem-server", "claude-mem-worker"}).
	Services []string

	// StatusDetail is shown in RuntimeStatus.Detail. Free-form.
	StatusDetail string
}

// Kind implements HostRuntime. Always "compose".
func (c *ComposeRuntime) Kind() string { return "compose" }

// Label implements HostRuntime.
func (c *ComposeRuntime) Label() string {
	return "Docker Compose stack (multi-container, persistent)"
}

// Status implements HostRuntime. Returns Running=true when:
//   - Services is empty: at least one service in the stack is running
//   - Services is non-empty: all listed services are running
//
// Any tool/file/permission error reports not-running rather than
// surfacing — Status is best-effort and the generic runner will fall
// back to the probe if it disagrees.
func (c *ComposeRuntime) Status(ctx context.Context) (RuntimeStatus, error) {
	dir := expandHome(c.Dir)
	if !composeAvailable() || !composeFileExists(dir) {
		return RuntimeStatus{}, nil
	}
	running, err := composeRunningServices(ctx, dir, c.Project)
	if err != nil {
		return RuntimeStatus{}, nil
	}
	if len(c.Services) == 0 {
		return RuntimeStatus{
			Running: len(running) > 0,
			Detail:  c.StatusDetail,
		}, nil
	}
	for _, want := range c.Services {
		if !containsString(running, want) {
			return RuntimeStatus{Detail: c.StatusDetail}, nil
		}
	}
	return RuntimeStatus{
		Running: true,
		Detail:  c.StatusDetail,
	}, nil
}

// Start implements HostRuntime. Writes the override file (when
// configured), then runs `docker compose up -d`.
func (c *ComposeRuntime) Start(ctx context.Context) error {
	dir := expandHome(c.Dir)
	if !composeAvailable() {
		return fmt.Errorf("docker compose not on PATH")
	}
	if !composeFileExists(dir) {
		return fmt.Errorf("docker-compose.yml not found in %s", dir)
	}
	if err := c.writeOverride(dir); err != nil {
		return err
	}
	return c.composeRun(ctx, dir, "up", "-d")
}

// Stop implements HostRuntime. Runs `docker compose down`. Leaves the
// override file in place — the user may want to inspect it.
func (c *ComposeRuntime) Stop(ctx context.Context) error {
	dir := expandHome(c.Dir)
	if !composeAvailable() || !composeFileExists(dir) {
		return nil
	}
	return c.composeRun(ctx, dir, "down")
}

// Logs implements HostRuntime. Returns the last maxBytes of
// `docker compose logs --tail=80`.
func (c *ComposeRuntime) Logs(ctx context.Context, maxBytes int64) (string, error) {
	dir := expandHome(c.Dir)
	if !composeAvailable() || !composeFileExists(dir) {
		return "", nil
	}
	args := []string{"compose"}
	if c.Project != "" {
		args = append(args, "--project-name", c.Project)
	}
	args = append(args, "logs", "--tail=80")
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	if int64(len(out)) > maxBytes {
		out = out[int64(len(out))-maxBytes:]
	}
	return string(out), nil
}

// writeOverride materializes the override file from OverrideContent.
// No-ops when OverrideContent is nil or returns empty content.
func (c *ComposeRuntime) writeOverride(dir string) error {
	if c.OverrideContent == nil || c.OverrideFilename == "" {
		return nil
	}
	content, err := c.OverrideContent()
	if err != nil {
		return fmt.Errorf("generate override: %w", err)
	}
	if strings.TrimSpace(content) == "" {
		return nil
	}
	path := filepath.Join(dir, c.OverrideFilename)
	// 0600 because compose overrides typically embed secrets
	// (database DSNs, API keys).
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// composeRun runs `docker compose <args>` in dir, capturing output
// for the error path. Used for up/down — invocations whose output we
// don't want to stream to the user's terminal directly (let the
// caller decide via Stdout/Stderr if they want to).
func (c *ComposeRuntime) composeRun(ctx context.Context, dir string, args ...string) error {
	full := []string{"compose"}
	if c.Project != "" {
		full = append(full, "--project-name", c.Project)
	}
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %w\n%s", strings.Join(full, " "), err,
			strings.TrimSpace(string(out)))
	}
	return nil
}

// composeAvailable reports whether `docker compose` is callable. We
// don't run `docker compose version` (which needs the daemon); LookPath
// for docker is enough because compose is a subcommand of the modern
// CLI and ships with Docker Desktop.
func composeAvailable() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

// composeFileExists reports whether a docker-compose.yml (or .yaml)
// lives in dir. Used as a precondition before any compose invocation.
func composeFileExists(dir string) bool {
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// composeRunningServices returns the list of service names currently
// running in the stack at dir. Empty + nil error means "stack is
// down". Errors are pushed up so Status can decide whether to swallow.
func composeRunningServices(ctx context.Context, dir, project string) ([]string, error) {
	args := []string{"compose"}
	if project != "" {
		args = append(args, "--project-name", project)
	}
	args = append(args, "ps", "--services", "--filter", "status=running")
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var services []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			services = append(services, l)
		}
	}
	return services, nil
}

func containsString(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
