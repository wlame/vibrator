// Package docker is a thin abstraction over the `docker` CLI. The whole tool
// shells out rather than using the official Go SDK — see docs/plans for the
// rationale (binary size, CLI stability, mockability).
//
// Client is an interface so unit tests can use a stub (mock.go) instead of
// touching the real daemon. Integration tests use the real CLIClient.
//
// Spec structs (BuildSpec, RunSpec, ExecSpec) carry the user-set knobs of
// each operation. They intentionally don't expose every `docker run` flag —
// only the ones vibrator actually uses.
package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// jsonStdUnmarshal is a typed alias to encoding/json's Unmarshal so the
// tiny wrapper in parseDockerImageJSON/parseDockerContainerJSON doesn't
// need to import encoding/json itself.
var jsonStdUnmarshal = json.Unmarshal

// EnvVar is a host-environment value forwarded into the container.
type EnvVar struct {
	Name, Value string
}

// Volume is a single bind mount, e.g. /host/path:/container/path[:ro].
type Volume struct {
	Host      string
	Container string
	ReadOnly  bool
}

// String renders a Volume in the form docker expects on the command line.
func (v Volume) String() string {
	s := v.Host + ":" + v.Container
	if v.ReadOnly {
		s += ":ro"
	}
	return s
}

// BuildSpec describes a `docker build` invocation.
type BuildSpec struct {
	// Either DockerfilePath OR DockerfileBytes must be set. If both are set,
	// DockerfilePath wins. DockerfileBytes is for inline builds where the
	// generator hasn't (or shouldn't) write to disk.
	DockerfilePath  string
	DockerfileBytes []byte

	// ContextDir is the build context. Required.
	ContextDir string

	// Tag is the resulting image tag. Required.
	Tag string

	// BuildArgs become --build-arg flags. Iteration order is sorted at emit
	// time so two equivalent BuildSpecs produce the same command line.
	BuildArgs map[string]string

	// NoCache forces --no-cache. Drastically slows the build; reserve for
	// --rebuild user requests.
	NoCache bool

	// Stdout/Stderr stream the build output. nil = discard.
	Stdout, Stderr io.Writer
}

// RunSpec describes a `docker run` invocation. Captures the cross-section
// of flags vibrator actually uses; we don't try to mirror docker's full
// flag surface.
type RunSpec struct {
	Image         string            // image:tag — required
	ContainerName string            // --name — optional but recommended
	Interactive  bool              // -it
	Detach       bool              // -d
	Remove       bool              // --rm
	Network      string            // --network (e.g. "host" or "bridge")
	Privileged   bool              // --privileged (escape hatch only)
	Init         bool              // --init (zombie reaper)
	ShmSize      string            // --shm-size (e.g. "2g")
	SecurityOpts []string          // --security-opt entries
	CapAdd       []string          // --cap-add (e.g. SYS_ADMIN for dind)
	AddHosts     []string          // --add-host entries (e.g. "host.docker.internal:host-gateway")
	Volumes      []Volume          // -v repeated
	Env          []EnvVar          // -e repeated
	Labels       map[string]string // --label repeated
	WorkingDir   string            // --workdir (cwd inside the container)
	Hostname     string            // --hostname (RFC 1123 — letters, digits, hyphens; max 63 chars)
	Cmd          []string          // command + args inside the container

	// I/O streams. nil stdin/stdout/stderr connect to the real process
	// streams when Interactive is true. Otherwise nil = discard.
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// ExecSpec describes a `docker exec` invocation.
type ExecSpec struct {
	Container      string
	Interactive    bool
	Env            []EnvVar
	WorkingDir     string // --workdir (cwd inside the container)
	Cmd            []string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// Client is the interface implemented by CLIClient (production) and Mock
// (tests). Methods take a context for cancellation and timeouts.
type Client interface {
	// Info verifies the docker daemon is reachable. Returns nil if reachable,
	// non-nil on failure. We deliberately don't return server info — most
	// callers just want a liveness check before proceeding.
	Info(ctx context.Context) error

	// Build runs `docker build`. Streams output to spec.Stdout/Stderr.
	Build(ctx context.Context, spec BuildSpec) error

	// Run starts a new container. If spec.Detach is false, blocks until the
	// container exits.
	Run(ctx context.Context, spec RunSpec) error

	// Exec runs a command in a running container.
	Exec(ctx context.Context, spec ExecSpec) error

	// Start starts a stopped container by name or ID.
	Start(ctx context.Context, nameOrID string) error

	// ImageExists reports whether an image with this tag exists locally.
	ImageExists(ctx context.Context, image string) (bool, error)

	// ContainerStatus returns the docker-reported status of a container by
	// name (e.g., "running", "exited"). Returns ("", nil) — NOT an error —
	// when the container doesn't exist. Callers should distinguish via the
	// empty-string return.
	ContainerStatus(ctx context.Context, name string) (string, error)

	// ListImages returns vibrator-managed images filtered by label. The
	// labelFilter is a "key=value" string passed to `--filter label=...`.
	// Used by `vibrate variants list` and by the launch orchestrator for
	// multi-variant detection.
	ListImages(ctx context.Context, labelFilter string) ([]ImageInfo, error)

	// ListContainers returns containers filtered by label. Like
	// ListImages, labelFilter is "key=value". Includes stopped containers.
	ListContainers(ctx context.Context, labelFilter string) ([]ContainerInfo, error)

	// Remove deletes an image or container by name/ID. kind selects
	// `docker image rm` vs `docker rm` (or `rm -f` when force=true).
	Remove(ctx context.Context, kind RemoveKind, nameOrID string, force bool) error
}

// ImageInfo is a minimal summary of a docker image, returned by
// ListImages.
type ImageInfo struct {
	// Tag is the human-readable tag, e.g. "vb-claude-code-full-abc12345:latest".
	Tag string

	// ID is the docker image ID (sha256 prefix). Used for force-removal
	// when tag matching is ambiguous.
	ID string

	// Labels are the image labels (parsed from `--format` JSON output).
	Labels map[string]string

	// CreatedAt is the image creation time string, as docker reports it
	// ("2 hours ago", "3 days ago"). Not parsed — used as-is in listings.
	CreatedAt string

	// SizeHuman is the docker-reported size (e.g. "1.2GB").
	SizeHuman string
}

// ContainerInfo is a minimal summary of a docker container.
type ContainerInfo struct {
	Name     string
	ID       string
	Image    string
	Status   string // "running", "exited", etc.
	Labels   map[string]string
	CreatedAt string
}

// RemoveKind selects between container and image removal.
type RemoveKind string

const (
	RemoveImage     RemoveKind = "image"
	RemoveContainer RemoveKind = "container"
)

// CLIClient is the production Client backed by `os/exec`. Construct with
// NewCLIClient(); the zero value is intentionally unusable so we can extend
// it with fields later without breaking callers.
type CLIClient struct {
	// DockerPath is the resolved path to the docker binary. Populated by
	// NewCLIClient; tests can override it to point at a stub script.
	DockerPath string
}

// NewCLIClient resolves the docker binary path and returns a CLIClient.
// Returns an error if docker isn't on $PATH.
func NewCLIClient() (*CLIClient, error) {
	path, err := exec.LookPath("docker")
	if err != nil {
		return nil, fmt.Errorf("docker CLI not found on $PATH: %w", err)
	}
	return &CLIClient{DockerPath: path}, nil
}

// --- Inspectors (return-data methods) ---

// Info shells out to `docker info` and returns nil if it succeeds. We don't
// parse the output — the success of the command is the liveness signal.
func (c *CLIClient) Info(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.DockerPath, "info")
	// Discard the (verbose) output; we only care about the exit code.
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker info: %w", err)
	}
	return nil
}

// ImageExists uses `docker image inspect` which exits 0 if the image is
// present and 1 if it isn't. We translate exit-1 into (false, nil) so
// callers can `if exists, _ := c.ImageExists(...); !exists { ... }`.
func (c *CLIClient) ImageExists(ctx context.Context, image string) (bool, error) {
	cmd := exec.CommandContext(ctx, c.DockerPath, "image", "inspect", "--format", "{{.Id}}", image)
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			// exit 1 specifically = not found
			return false, nil
		}
		return false, fmt.Errorf("docker image inspect %s: %w", image, err)
	}
	return true, nil
}

// ContainerStatus uses `docker container inspect --format {{.State.Status}}`.
// Container not found → ("", nil). Found → ("running"|"exited"|..., nil).
func (c *CLIClient) ContainerStatus(ctx context.Context, name string) (string, error) {
	cmd := exec.CommandContext(ctx, c.DockerPath,
		"container", "inspect", "--format", "{{.State.Status}}", name)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			// Inspect returns exit 1 with "No such container" on stderr
			// when the container doesn't exist. We treat that as a
			// non-error: container simply not present.
			if strings.Contains(string(ee.Stderr), "No such container") {
				return "", nil
			}
		}
		return "", fmt.Errorf("docker container inspect %s: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// --- Action methods ---

// Build runs `docker build`. Output streams to spec.Stdout/Stderr.
//
// When DockerfilePath is empty, we pipe DockerfileBytes via stdin and use
// `-f -`, which lets us build deterministically without touching disk.
func (c *CLIClient) Build(ctx context.Context, spec BuildSpec) error {
	args := []string{"build"}

	// Build args, sorted for stable command-line output.
	for _, k := range sortedMapKeys(spec.BuildArgs) {
		args = append(args, "--build-arg", k+"="+spec.BuildArgs[k])
	}
	if spec.NoCache {
		args = append(args, "--no-cache")
	}
	args = append(args, "-t", spec.Tag)

	// Dockerfile + context handling: either path or stdin.
	switch {
	case spec.DockerfilePath != "":
		args = append(args, "-f", spec.DockerfilePath, spec.ContextDir)
	case len(spec.DockerfileBytes) > 0:
		args = append(args, "-f", "-", spec.ContextDir)
	default:
		return errors.New("docker build: both DockerfilePath and DockerfileBytes are empty")
	}

	cmd := exec.CommandContext(ctx, c.DockerPath, args...)
	if len(spec.DockerfileBytes) > 0 {
		cmd.Stdin = strings.NewReader(string(spec.DockerfileBytes))
	}
	cmd.Stdout = spec.Stdout
	cmd.Stderr = spec.Stderr
	return cmd.Run()
}

// Run translates RunSpec into `docker run ...` arguments and execs.
func (c *CLIClient) Run(ctx context.Context, spec RunSpec) error {
	args := buildRunArgs(spec)
	cmd := exec.CommandContext(ctx, c.DockerPath, args...)
	cmd.Stdin = spec.Stdin
	cmd.Stdout = spec.Stdout
	cmd.Stderr = spec.Stderr
	return cmd.Run()
}

// Exec translates ExecSpec into `docker exec ...` arguments and execs.
func (c *CLIClient) Exec(ctx context.Context, spec ExecSpec) error {
	args := []string{"exec"}
	if spec.Interactive {
		args = append(args, "-it")
	}
	for _, e := range spec.Env {
		args = append(args, "-e", e.Name+"="+e.Value)
	}
	if spec.WorkingDir != "" {
		args = append(args, "--workdir", spec.WorkingDir)
	}
	args = append(args, spec.Container)
	args = append(args, spec.Cmd...)

	cmd := exec.CommandContext(ctx, c.DockerPath, args...)
	cmd.Stdin = spec.Stdin
	cmd.Stdout = spec.Stdout
	cmd.Stderr = spec.Stderr
	return cmd.Run()
}

// ListImages shells out to `docker images --filter label=<labelFilter>
// --format <json>` and parses the JSON-per-line output. Empty result
// when no images match — caller decides whether that's an error.
func (c *CLIClient) ListImages(ctx context.Context, labelFilter string) ([]ImageInfo, error) {
	args := []string{"images"}
	if labelFilter != "" {
		args = append(args, "--filter", "label="+labelFilter)
	}
	args = append(args, "--format", "{{json .}}")

	out, err := exec.CommandContext(ctx, c.DockerPath, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("docker images: %w", err)
	}
	return parseDockerImageJSON(out, c, ctx)
}

// ListContainers shells out to `docker ps -a --filter label=...
// --format json`. Includes stopped/exited containers.
func (c *CLIClient) ListContainers(ctx context.Context, labelFilter string) ([]ContainerInfo, error) {
	args := []string{"ps", "-a"}
	if labelFilter != "" {
		args = append(args, "--filter", "label="+labelFilter)
	}
	args = append(args, "--format", "{{json .}}")

	out, err := exec.CommandContext(ctx, c.DockerPath, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}
	return parseDockerContainerJSON(out, c, ctx)
}

// Remove deletes a container or image. When force=true, also kills
// running containers / forces image removal even when referenced.
func (c *CLIClient) Remove(ctx context.Context, kind RemoveKind, nameOrID string, force bool) error {
	var args []string
	switch kind {
	case RemoveImage:
		args = []string{"image", "rm"}
	case RemoveContainer:
		args = []string{"rm"}
	default:
		return fmt.Errorf("docker Remove: unknown kind %q", kind)
	}
	if force {
		args = append(args, "-f")
	}
	args = append(args, nameOrID)
	cmd := exec.CommandContext(ctx, c.DockerPath, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker %s rm %s: %w (output: %s)",
			kind, nameOrID, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// parseDockerImageJSON parses the `--format {{json .}}` newline-
// delimited output. We fetch the labels separately via `docker image
// inspect` (the listing format doesn't include the full label map).
func parseDockerImageJSON(raw []byte, c *CLIClient, ctx context.Context) ([]ImageInfo, error) {
	type listed struct {
		Repository, Tag, ID, CreatedSince, Size string
	}
	var out []ImageInfo
	for _, line := range splitNonEmptyLines(raw) {
		var l listed
		if err := jsonUnmarshal(line, &l); err != nil {
			return nil, fmt.Errorf("parse images JSON: %w", err)
		}
		info := ImageInfo{
			Tag:       l.Repository + ":" + l.Tag,
			ID:        l.ID,
			CreatedAt: l.CreatedSince,
			SizeHuman: l.Size,
		}
		// Pull labels via inspect — best-effort; failures don't abort
		// the listing.
		if labels, err := inspectLabels(ctx, c, "image", l.ID); err == nil {
			info.Labels = labels
		}
		out = append(out, info)
	}
	return out, nil
}

// parseDockerContainerJSON parses `docker ps -a --format {{json .}}`.
func parseDockerContainerJSON(raw []byte, c *CLIClient, ctx context.Context) ([]ContainerInfo, error) {
	type listed struct {
		Names, ID, Image, Status, CreatedAt string
	}
	var out []ContainerInfo
	for _, line := range splitNonEmptyLines(raw) {
		var l listed
		if err := jsonUnmarshal(line, &l); err != nil {
			return nil, fmt.Errorf("parse ps JSON: %w", err)
		}
		info := ContainerInfo{
			Name:      l.Names,
			ID:        l.ID,
			Image:     l.Image,
			Status:    l.Status,
			CreatedAt: l.CreatedAt,
		}
		if labels, err := inspectLabels(ctx, c, "container", l.ID); err == nil {
			info.Labels = labels
		}
		out = append(out, info)
	}
	return out, nil
}

// inspectLabels runs `docker <kind> inspect --format {{json .Config.Labels}}`
// and returns the parsed label map. Used to enrich the bare listing
// output with our `vibrator.*` labels.
func inspectLabels(ctx context.Context, c *CLIClient, kind, id string) (map[string]string, error) {
	cmd := exec.CommandContext(ctx, c.DockerPath, kind, "inspect",
		"--format", "{{json .Config.Labels}}", id)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var labels map[string]string
	if err := jsonUnmarshal(strings.TrimSpace(string(out)), &labels); err != nil {
		return nil, err
	}
	return labels, nil
}

// splitNonEmptyLines splits raw into non-empty trimmed lines.
func splitNonEmptyLines(raw []byte) []string {
	var out []string
	for _, l := range strings.Split(string(raw), "\n") {
		t := strings.TrimSpace(l)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// jsonUnmarshal is a tiny wrapper to avoid pulling the import into the
// public API surface; the body uses encoding/json directly.
func jsonUnmarshal(s string, dst any) error {
	return jsonStdUnmarshal([]byte(s), dst)
}

// Start starts a stopped container.
func (c *CLIClient) Start(ctx context.Context, nameOrID string) error {
	cmd := exec.CommandContext(ctx, c.DockerPath, "start", nameOrID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker start %s: %w", nameOrID, err)
	}
	return nil
}

// buildRunArgs is split out so the mock can mirror the same flag-ordering
// logic for assertion-friendly call traces.
func buildRunArgs(spec RunSpec) []string {
	args := []string{"run"}
	if spec.Remove {
		args = append(args, "--rm")
	}
	if spec.Interactive {
		args = append(args, "-it")
	}
	if spec.Detach {
		args = append(args, "-d")
	}
	if spec.Privileged {
		args = append(args, "--privileged")
	}
	if spec.Init {
		args = append(args, "--init")
	}
	if spec.ShmSize != "" {
		args = append(args, "--shm-size", spec.ShmSize)
	}
	if spec.Network != "" {
		args = append(args, "--network", spec.Network)
	}
	for _, s := range spec.SecurityOpts {
		args = append(args, "--security-opt", s)
	}
	for _, c := range spec.CapAdd {
		args = append(args, "--cap-add", c)
	}
	for _, h := range spec.AddHosts {
		args = append(args, "--add-host", h)
	}
	if spec.ContainerName != "" {
		args = append(args, "--name", spec.ContainerName)
	}
	if spec.Hostname != "" {
		args = append(args, "--hostname", spec.Hostname)
	}
	for _, k := range sortedMapKeys(spec.Labels) {
		args = append(args, "--label", k+"="+spec.Labels[k])
	}
	for _, e := range spec.Env {
		args = append(args, "-e", e.Name+"="+e.Value)
	}
	for _, v := range spec.Volumes {
		args = append(args, "-v", v.String())
	}
	if spec.WorkingDir != "" {
		args = append(args, "--workdir", spec.WorkingDir)
	}
	args = append(args, spec.Image)
	args = append(args, spec.Cmd...)
	return args
}

// sortedMapKeys returns the keys of m in lexicographic order. Used wherever
// a flag's emission order matters for reproducibility.
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Use a simple insertion sort for tiny maps; for larger ones swap to
	// sort.Strings. For the sizes vibrator deals with, the difference is noise.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}
