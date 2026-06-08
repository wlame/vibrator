package app

import (
	"bytes"
	"context"
	"testing"

	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/dockerfile"
	"github.com/wlame/vibrator/internal/extensions"
	"github.com/wlame/vibrator/internal/workspace"
)

// resolveAndLaunch decides whether to rebuild, reuse, start, or freshly
// build+run a workspace. These tests pin that decision logic. The heavy
// operations (buildImage, runContainer, execIntoContainer) are replaced with
// recording stubs via the package-level seams so the tests assert *which*
// path runs without performing a real docker build/run/exec.

// launchProbe records which of the three terminal operations fired.
type launchProbe struct {
	built  bool
	ran    bool
	execed bool
}

// installLaunchStubs swaps the seams for recording stubs and returns a
// restore func (call via defer) so other tests aren't affected.
func installLaunchStubs(t *testing.T) *launchProbe {
	t.Helper()
	p := &launchProbe{}

	origBuild, origRun, origExec := buildImageFn, runContainerFn, execIntoContainerFn
	t.Cleanup(func() {
		buildImageFn, runContainerFn, execIntoContainerFn = origBuild, origRun, origExec
	})

	buildImageFn = func(_ context.Context, _ docker.Client, _ dockerfile.Spec, _ string, _ Options) error {
		p.built = true
		return nil
	}
	runContainerFn = func(_ context.Context, _ docker.Client, _, _, _ string,
		_ workspace.Spec, _ config.Pin, _ []*extensions.Entry, _ Options) error {
		p.ran = true
		return nil
	}
	execIntoContainerFn = func(_ context.Context, _ docker.Client, _, _ string, _ config.Pin, _ Options) error {
		p.execed = true
		return nil
	}
	return p
}

const (
	testContainer = "vibe-ws-abc123"
	testImage     = "vibrator/ws:abc123"
)

func newResolveArgs(opts Options) (dockerfile.Spec, workspace.Spec, config.Pin, string, string, string, Options) {
	return dockerfile.Spec{}, workspace.Spec{}, config.Pin{}, "/ws", testImage, testContainer, opts
}

// The regression target: with a running container, --rebuild must tear the
// container down and rebuild from scratch instead of exec'ing straight in.
func TestResolveAndLaunch_RebuildRemovesRunningContainerAndRebuilds(t *testing.T) {
	probe := installLaunchStubs(t)
	mock := docker.NewMock()
	mock.Containers[testContainer] = "running"

	var stderr bytes.Buffer
	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(Options{
		Rebuild: true,
		Stderr:  &stderr,
	})

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}

	if !probe.built {
		t.Error("expected image rebuild, but buildImage was not called")
	}
	if !probe.ran {
		t.Error("expected fresh container run, but runContainer was not called")
	}
	if probe.execed {
		t.Error("expected NO exec into the old container, but execIntoContainer was called")
	}
	// The running container must be force-removed before the rebuild.
	if rm := mock.CallsFor("container"); len(rmContainerRemovals(mock)) != 1 {
		t.Errorf("expected exactly one container rm, got calls: %v", rm)
	}
}

// Without --rebuild, a running container is reused: exec only, no build/remove.
func TestResolveAndLaunch_RunningContainerReusedWithoutRebuild(t *testing.T) {
	probe := installLaunchStubs(t)
	mock := docker.NewMock()
	mock.Containers[testContainer] = "running"

	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(Options{
		Stderr: &bytes.Buffer{},
	})

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}

	if !probe.execed {
		t.Error("expected exec into running container")
	}
	if probe.built || probe.ran {
		t.Errorf("expected no build/run when reusing, got built=%v ran=%v", probe.built, probe.ran)
	}
	if n := len(rmContainerRemovals(mock)); n != 0 {
		t.Errorf("expected no container removal when reusing, got %d", n)
	}
}

// With no existing container and a missing image, build then run.
func TestResolveAndLaunch_NoContainerBuildsAndRuns(t *testing.T) {
	probe := installLaunchStubs(t)
	mock := docker.NewMock() // no container, no image present

	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(Options{
		Stderr: &bytes.Buffer{},
	})

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}

	if !probe.built || !probe.ran {
		t.Errorf("expected build+run for fresh workspace, got built=%v ran=%v", probe.built, probe.ran)
	}
	if probe.execed {
		t.Error("did not expect exec when no container exists")
	}
}

// rmContainerRemovals returns the recorded `container rm` calls. The mock
// records Remove as [<kind>, rm, ...], so a container removal looks like
// ["container", "rm", "-f", name].
func rmContainerRemovals(m *docker.Mock) [][]string {
	var out [][]string
	for _, c := range m.CallsFor("container") {
		if len(c) >= 2 && c[1] == "rm" {
			out = append(out, c)
		}
	}
	return out
}
