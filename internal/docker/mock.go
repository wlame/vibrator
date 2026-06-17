package docker

import (
	"context"
	"sync"
)

// Mock is an in-memory Client implementation for unit tests.
//
// It records every call's name and arguments so tests can assert that
// callers invoked the right operation with the right shape. Return values
// (Info error, ImageExists bool, ContainerStatus string) are stubbed by
// setting the corresponding ...Result fields before exercising the SUT.
//
// Mock is safe for concurrent use.
type Mock struct {
	mu sync.Mutex

	// Calls records every method invocation in order. Each entry is the
	// docker arg vector that would have been passed to the real CLI, so
	// assertions can compare against expected command lines without
	// depending on Go struct field ordering.
	Calls [][]string

	// --- Stubbed return values ---

	// InfoErr is returned by Info().
	InfoErr error

	// BuildErr is returned by Build().
	BuildErr error

	// RunErr is returned by Run().
	RunErr error

	// ExecErr is returned by Exec().
	ExecErr error

	// StartErr is returned by Start().
	StartErr error

	// Images is the set of image tags that ImageExists should report as
	// present. nil = empty.
	Images map[string]bool

	// ImageExistsErr, when set, is returned by ImageExists instead of the
	// Images lookup — lets tests simulate a docker daemon failure (e.g. a
	// timeout inspecting the image) during the generator-staleness check.
	ImageExistsErr error

	// Containers maps container name to its reported docker status string
	// (e.g., "running", "exited"). Missing entries → ("", nil) — meaning
	// "container does not exist", matching the real CLI semantics.
	Containers map[string]string

	// ContainerLabels maps container name → label key → value, backing
	// ContainerLabel(). Missing name or key → ("", nil).
	ContainerLabels map[string]map[string]string

	// ImageLabels maps image tag → label key → value, backing
	// ImageLabel(). Missing image or key → ("", nil).
	ImageLabels map[string]map[string]string

	// RunHandler, if non-nil, is called by Run() before returning RunErr.
	// It has full access to the RunSpec — including Stdin/Stdout — so tests
	// can simulate processes that consume input and produce output (e.g.,
	// the claude-mem psql one-shot). Errors returned by the handler
	// supersede RunErr.
	RunHandler func(ctx context.Context, spec RunSpec) error

	// Listed stubs for List* responses, keyed by labelFilter.
	ListedImages     map[string][]ImageInfo
	ListedContainers map[string][]ContainerInfo

	// RemoveErr is returned by Remove().
	RemoveErr error
}

// NewMock returns a Mock with empty stubs. Use this in tests when you want
// a clean baseline; mutate fields directly to customize.
func NewMock() *Mock {
	return &Mock{
		Images:     make(map[string]bool),
		Containers: make(map[string]string),
	}
}

// recordCall captures a call. Argv mimics what the real CLIClient would
// have passed to `docker`, which makes test assertions match user expectations.
func (m *Mock) recordCall(argv ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Defensive copy so callers can't mutate retroactively.
	cp := make([]string, len(argv))
	copy(cp, argv)
	m.Calls = append(m.Calls, cp)
}

// CallsFor returns just the calls whose first arg matches verb (e.g., "build",
// "run", "exec"). Convenience for assertions.
func (m *Mock) CallsFor(verb string) [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out [][]string
	for _, c := range m.Calls {
		if len(c) > 0 && c[0] == verb {
			out = append(out, c)
		}
	}
	return out
}

// Reset clears recorded calls and stubbed state. Useful between subtests.
func (m *Mock) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = nil
	m.InfoErr = nil
	m.BuildErr = nil
	m.RunErr = nil
	m.ExecErr = nil
	m.StartErr = nil
	m.Images = make(map[string]bool)
	m.ImageExistsErr = nil
	m.Containers = make(map[string]string)
}

// --- Client implementation ---

func (m *Mock) Info(ctx context.Context) error {
	m.recordCall("info")
	return m.InfoErr
}

func (m *Mock) Build(ctx context.Context, spec BuildSpec) error {
	// Mirror the real arg construction so tests can compare easily.
	argv := []string{"build"}
	for _, k := range sortedMapKeys(spec.BuildArgs) {
		argv = append(argv, "--build-arg", k+"="+spec.BuildArgs[k])
	}
	if spec.NoCache {
		argv = append(argv, "--no-cache")
	}
	for _, k := range sortedMapKeys(spec.Labels) {
		argv = append(argv, "--label", k+"="+spec.Labels[k])
	}
	argv = append(argv, "-t", spec.Tag)
	if spec.DockerfilePath != "" {
		argv = append(argv, "-f", spec.DockerfilePath, spec.ContextDir)
	} else if len(spec.DockerfileBytes) > 0 {
		argv = append(argv, "-f", "-", spec.ContextDir)
	} else {
		argv = append(argv, spec.ContextDir)
	}
	m.recordCall(argv...)
	return m.BuildErr
}

func (m *Mock) Run(ctx context.Context, spec RunSpec) error {
	argv := buildRunArgs(spec)
	m.recordCall(argv...)
	if m.RunHandler != nil {
		return m.RunHandler(ctx, spec)
	}
	return m.RunErr
}

func (m *Mock) Exec(ctx context.Context, spec ExecSpec) error {
	argv := []string{"exec"}
	if spec.Interactive {
		argv = append(argv, "-it")
	}
	for _, e := range spec.Env {
		argv = append(argv, "-e", e.Name+"="+e.Value)
	}
	argv = append(argv, spec.Container)
	argv = append(argv, spec.Cmd...)
	m.recordCall(argv...)
	return m.ExecErr
}

func (m *Mock) Start(ctx context.Context, nameOrID string) error {
	m.recordCall("start", nameOrID)
	return m.StartErr
}

func (m *Mock) ImageExists(ctx context.Context, image string) (bool, error) {
	m.recordCall("image", "inspect", "--format", "{{.Id}}", image)
	if m.ImageExistsErr != nil {
		return false, m.ImageExistsErr
	}
	return m.Images[image], nil
}

func (m *Mock) ContainerStatus(ctx context.Context, name string) (string, error) {
	m.recordCall("container", "inspect", "--format", "{{.State.Status}}", name)
	return m.Containers[name], nil
}

func (m *Mock) ContainerLabel(ctx context.Context, name, key string) (string, error) {
	m.recordCall("container", "inspect", "--format", "{{index .Config.Labels "+key+"}}", name)
	if labels, ok := m.ContainerLabels[name]; ok {
		return labels[key], nil
	}
	return "", nil
}

func (m *Mock) ImageLabel(ctx context.Context, image, key string) (string, error) {
	m.recordCall("image", "inspect", "--format", "{{index .Config.Labels "+key+"}}", image)
	if labels, ok := m.ImageLabels[image]; ok {
		return labels[key], nil
	}
	return "", nil
}

func (m *Mock) ListImages(ctx context.Context, labelFilter string) ([]ImageInfo, error) {
	m.recordCall("images", "--filter", "label="+labelFilter)
	return m.ListedImages[labelFilter], nil
}

func (m *Mock) ListContainers(ctx context.Context, labelFilter string) ([]ContainerInfo, error) {
	m.recordCall("ps", "-a", "--filter", "label="+labelFilter)
	return m.ListedContainers[labelFilter], nil
}

func (m *Mock) Remove(ctx context.Context, kind RemoveKind, nameOrID string, force bool) error {
	argv := []string{string(kind), "rm"}
	if force {
		argv = append(argv, "-f")
	}
	argv = append(argv, nameOrID)
	m.recordCall(argv...)
	return m.RemoveErr
}

// Static compile-time check that Mock satisfies the Client interface.
// If we change the Client signature, the build breaks here before tests do.
var _ Client = (*Mock)(nil)

// Same for CLIClient (kept here rather than in docker.go so adding new
// methods to Client surfaces both broken implementations on the same line).
var _ Client = (*CLIClient)(nil)
