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

// launchProbe records which of the five terminal operations fired.
type launchProbe struct {
	built            bool
	ran              bool
	execed           bool
	entrypointWaited bool
	loggedIn         bool
}

// installLaunchStubs swaps the seams for recording stubs and returns a
// restore func (call via defer) so other tests aren't affected.
func installLaunchStubs(t *testing.T) *launchProbe {
	t.Helper()
	p := &launchProbe{}

	origBuild := buildImageFn
	origRun := runContainerFn
	origExec := execIntoContainerFn
	origWait := waitForEntrypointFn
	origLogin := runLoginStepFn
	t.Cleanup(func() {
		buildImageFn = origBuild
		runContainerFn = origRun
		execIntoContainerFn = origExec
		waitForEntrypointFn = origWait
		runLoginStepFn = origLogin
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
	waitForEntrypointFn = func(_ context.Context, _ docker.Client, _ string) error {
		p.entrypointWaited = true
		return nil
	}
	runLoginStepFn = func(_ context.Context, _ docker.Client, _, _ string, _ Options) error {
		p.loggedIn = true
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

// Regression: --rebuild with --login must rebuild and then go through the
// full login sequence (wait → login → exec), not return after runContainer.
func TestResolveAndLaunch_RebuildWithLoginRunsLoginThenExecs(t *testing.T) {
	probe := installLaunchStubs(t)
	mock := docker.NewMock()
	mock.Containers[testContainer] = "running"

	var stderr bytes.Buffer
	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(Options{
		Rebuild:   true,
		LoginMode: true,
		Stderr:    &stderr,
	})

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}

	if !probe.built {
		t.Error("expected image rebuild")
	}
	if !probe.ran {
		t.Error("expected runContainer (detached for login)")
	}
	if !probe.entrypointWaited {
		t.Error("expected waitForEntrypoint before login")
	}
	if !probe.loggedIn {
		t.Error("expected runLoginStep to be called")
	}
	if !probe.execed {
		t.Error("expected execIntoContainer after login")
	}
}

// --login against a running container: login step fires before exec, no rebuild.
func TestResolveAndLaunch_RunningContainerWithLoginRunsLoginThenExecs(t *testing.T) {
	probe := installLaunchStubs(t)
	mock := docker.NewMock()
	mock.Containers[testContainer] = "running"

	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(Options{
		LoginMode: true,
		Stderr:    &bytes.Buffer{},
	})

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}

	if probe.built || probe.ran {
		t.Errorf("expected no build/run for already-running container, got built=%v ran=%v", probe.built, probe.ran)
	}
	if !probe.loggedIn {
		t.Error("expected runLoginStep to be called")
	}
	if !probe.execed {
		t.Error("expected execIntoContainer after login")
	}
}

// --dind state change: an existing (exited) container created WITHOUT the
// socket must be recreated when --dind is requested — removed and re-run
// from the EXISTING image, never rebuilt. This is the regression target for
// "vibrate --dind on a prior container rebuilds from scratch".
func TestResolveAndLaunch_DinDChangeRecreatesContainerWithoutRebuild(t *testing.T) {
	probe := installLaunchStubs(t)
	mock := docker.NewMock()
	mock.Containers[testContainer] = "exited"
	// Container was created without --dind (label "false"); image is present.
	mock.ContainerLabels = map[string]map[string]string{
		testContainer: {dindLabelKey: "false"},
	}
	mock.Images[testImage] = true

	var stderr bytes.Buffer
	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(Options{
		DinD:   true,
		Stderr: &stderr,
	})

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}

	if probe.built {
		t.Error("expected NO image rebuild (image already present) when only --dind changed")
	}
	if !probe.ran {
		t.Error("expected a fresh runContainer with the socket mounted")
	}
	if n := len(rmContainerRemovals(mock)); n != 1 {
		t.Errorf("expected exactly one container removal for the --dind change, got %d", n)
	}
}

// --dind state matches the existing container → reuse it, no remove, no run.
func TestResolveAndLaunch_DinDMatchReusesContainer(t *testing.T) {
	probe := installLaunchStubs(t)
	mock := docker.NewMock()
	mock.Containers[testContainer] = "running"
	mock.ContainerLabels = map[string]map[string]string{
		testContainer: {dindLabelKey: "true"},
	}

	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(Options{
		DinD:   true,
		Stderr: &bytes.Buffer{},
	})

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}

	if !probe.execed {
		t.Error("expected exec into the matching --dind container")
	}
	if probe.built || probe.ran {
		t.Errorf("expected no build/run when --dind matches, got built=%v ran=%v", probe.built, probe.ran)
	}
	if n := len(rmContainerRemovals(mock)); n != 0 {
		t.Errorf("expected no removal when --dind state matches, got %d", n)
	}
}

// Setting (or changing) the [identity] alias on an existing container must
// recreate it from the existing image — identity is injected at run time
// (env + entrypoint), so an old container would otherwise keep leaking the
// real email. No rebuild; image is reused.
func TestResolveAndLaunch_IdentityChangeRecreatesContainerWithoutRebuild(t *testing.T) {
	probe := installLaunchStubs(t)
	mock := docker.NewMock()
	mock.Containers[testContainer] = "exited"
	// Existing container carries no identity (label empty); image present.
	mock.ContainerLabels = map[string]map[string]string{
		testContainer: {dindLabelKey: "false", identityLabelKey: ""},
	}
	mock.Images[testImage] = true

	var stderr bytes.Buffer
	dfSpec, wsSpec, _, wsDir, imageTag, containerName, opts := newResolveArgs(Options{Stderr: &stderr})
	pin := config.Pin{Identity: &config.Identity{Email: "alias@example.com"}}

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}

	if probe.built {
		t.Error("expected NO image rebuild when only the identity alias changed")
	}
	if !probe.ran {
		t.Error("expected a fresh runContainer carrying the new identity")
	}
	if n := len(rmContainerRemovals(mock)); n != 1 {
		t.Errorf("expected exactly one container removal for the identity change, got %d", n)
	}
}

// Changing the extra --mount set on an existing container must recreate it
// from the existing image — bind mounts can't be added to a live container.
// No rebuild; image is reused.
func TestResolveAndLaunch_MountChangeRecreatesContainerWithoutRebuild(t *testing.T) {
	probe := installLaunchStubs(t)
	mock := docker.NewMock()
	mock.Containers[testContainer] = "exited"
	mock.Images[testImage] = true
	// dind and identity MATCH (so they don't trigger); mounts MISMATCH.
	mock.ContainerLabels = map[string]map[string]string{
		testContainer: {dindLabelKey: "false", identityLabelKey: "", mountsLabelKey: "stale-fingerprint"},
	}

	var stderr bytes.Buffer
	dfSpec, wsSpec, _, wsDir, imageTag, containerName, opts := newResolveArgs(Options{Stderr: &stderr})
	// A real existing dir so ResolveAll succeeds and yields a non-empty fingerprint
	// that differs from "stale-fingerprint".
	pin := config.Pin{Mounts: []string{t.TempDir()}}

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}

	if probe.built {
		t.Error("expected NO image rebuild when only the mount set changed")
	}
	if !probe.ran {
		t.Error("expected a fresh runContainer with the new mounts")
	}
	if n := len(rmContainerRemovals(mock)); n != 1 {
		t.Errorf("expected exactly one container removal for the mount change, got %d", n)
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
