package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/dockerfile"
	"github.com/wlame/vibrator/internal/extensions"
	"github.com/wlame/vibrator/internal/harness"
	_ "github.com/wlame/vibrator/internal/harness/all" // register built-in harnesses
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
	runLoginStepFn = func(_ context.Context, _ docker.Client, _, _ string, _ *harness.LoginFlow, _ Options) error {
		p.loggedIn = true
		return nil
	}
	return p
}

const (
	testContainer = "vibe-ws-abc123"
	testImage     = "vibrator/ws:abc123"
)

// testDfSpec returns the smallest dockerfile.Spec valid enough for
// dockerfile.GeneratorHash to succeed (Harness + Shell set) — needed now
// that resolveAndLaunch's generator-staleness check runs GeneratorHash on
// the dfSpec even for tests that don't otherwise care about its contents.
func testDfSpec(t *testing.T) dockerfile.Spec {
	t.Helper()
	h, ok := harness.ByID("claude-code")
	if !ok {
		t.Fatalf("harness %q not registered", "claude-code")
	}
	return dockerfile.Spec{Harness: h, Shell: "zsh"}
}

func newResolveArgs(t *testing.T, opts Options) (dockerfile.Spec, workspace.Spec, config.Pin, string, string, string, Options) {
	t.Helper()
	return testDfSpec(t), workspace.Spec{}, config.Pin{}, "/ws", testImage, testContainer, opts
}

// The regression target: with a running container, --rebuild must tear the
// container down and rebuild from scratch instead of exec'ing straight in.
func TestResolveAndLaunch_RebuildRemovesRunningContainerAndRebuilds(t *testing.T) {
	probe := installLaunchStubs(t)
	mock := docker.NewMock()
	mock.Containers[testContainer] = "running"

	var stderr bytes.Buffer
	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
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

	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
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

	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
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
	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
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

	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
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
	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
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

	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
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
	dfSpec, wsSpec, _, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{Stderr: &stderr})
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
	dfSpec, wsSpec, _, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{Stderr: &stderr})
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

// ─── Login-container cleanup ───────────────────────────────────────────────
//
// LoginMode starts a container detached solely to run `claude auth login`
// then exec in. If the final exec never succeeds, the user is left with an
// invisible background container unless the failure path removes it. These
// tests pin that cleanup for the "no existing container" path — the same
// loginLaunch helper backs the --rebuild branch too.

// Login exec failure: the detached container must be removed exactly once
// and the error must propagate to the caller.
func TestResolveAndLaunch_LoginExecFailureRemovesDetachedContainer(t *testing.T) {
	probe := installLaunchStubs(t)
	wantErr := errors.New("exec: attach failed")
	execIntoContainerFn = func(_ context.Context, _ docker.Client, _, _ string, _ config.Pin, _ Options) error {
		probe.execed = true
		return wantErr
	}
	mock := docker.NewMock() // no existing container
	mock.Images[testImage] = true

	var stderr bytes.Buffer
	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
		LoginMode: true,
		Stderr:    &stderr,
	})

	err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the exec error to propagate, got: %v", err)
	}
	if n := len(rmContainerRemovals(mock)); n != 1 {
		t.Errorf("expected exactly one container removal after a failed login exec, got %d", n)
	}
}

// Login exec success: no removal — the container is the user's live session.
func TestResolveAndLaunch_LoginExecSuccessLeavesContainer(t *testing.T) {
	probe := installLaunchStubs(t)
	mock := docker.NewMock() // no existing container
	mock.Images[testImage] = true

	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
		LoginMode: true,
		Stderr:    &bytes.Buffer{},
	})

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}
	if !probe.execed {
		t.Error("expected execIntoContainer to be called")
	}
	if n := len(rmContainerRemovals(mock)); n != 0 {
		t.Errorf("expected no container removal after a successful login exec, got %d", n)
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

// ─── Generator-staleness detection ─────────────────────────────────────────
//
// These tests pin resolveAndLaunch's stale-image check: when an existing
// image's "vibrator.generator" label doesn't match the hash the current
// generator would produce, the user is warned and offered a rebuild via the
// promptStaleRebuildFn seam — never rebuilt silently.

// installPromptStub swaps promptStaleRebuildFn for a recording stub that
// always returns decision, and restores the original on test cleanup.
// Returns a pointer the test can check to confirm the seam was invoked.
func installPromptStub(t *testing.T, decision bool) *bool {
	t.Helper()
	called := false
	orig := promptStaleRebuildFn
	t.Cleanup(func() { promptStaleRebuildFn = orig })
	promptStaleRebuildFn = func(_ Options, _, _, _ string) bool {
		called = true
		return decision
	}
	return &called
}

// imageLabelCalls reports whether any recorded call references the
// generator label key — i.e., whether ImageLabel was invoked at all. Used
// to assert the stale check was skipped entirely (--rebuild already set,
// or the image doesn't exist).
func imageLabelCalls(m *docker.Mock) bool {
	for _, c := range m.Calls {
		if strings.Contains(strings.Join(c, " "), GeneratorLabelKey) {
			return true
		}
	}
	return false
}

// Case 1: the image's generator label matches the current hash — no
// staleness, no prompt, normal reuse (image present, skip build, run).
func TestResolveAndLaunch_GeneratorHashMatchesNoRebuild(t *testing.T) {
	probe := installLaunchStubs(t)
	mock := docker.NewMock()
	mock.Images[testImage] = true

	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
		Stderr: &bytes.Buffer{},
	})
	want, err := dockerfile.GeneratorHash(dfSpec)
	if err != nil {
		t.Fatalf("GeneratorHash: %v", err)
	}
	mock.ImageLabels = map[string]map[string]string{
		testImage: {GeneratorLabelKey: want},
	}

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}

	if probe.built {
		t.Error("expected NO rebuild when the generator hash matches")
	}
	if !probe.ran {
		t.Error("expected normal runContainer when reusing a fresh image")
	}
}

// Case 2: generator label mismatch, user declines the rebuild prompt — a
// warning is printed but the launch proceeds without a rebuild.
func TestResolveAndLaunch_GeneratorHashMismatchPromptDeclines(t *testing.T) {
	probe := installLaunchStubs(t)
	called := installPromptStub(t, false)
	mock := docker.NewMock()
	mock.Images[testImage] = true

	var stderr bytes.Buffer
	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
		Stderr: &stderr,
	})
	mock.ImageLabels = map[string]map[string]string{
		testImage: {GeneratorLabelKey: "stale-hash-0000"},
	}

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}

	if !*called {
		t.Error("expected promptStaleRebuildFn to be invoked on mismatch")
	}
	if probe.built {
		t.Error("expected NO rebuild when the user declines the prompt")
	}
	if !probe.ran {
		t.Error("expected the launch to proceed (reuse the stale image) after declining")
	}
	if !strings.Contains(stderr.String(), "different vibrate") {
		t.Errorf("expected a staleness warning on stderr, got: %s", stderr.String())
	}
}

// Case 3: generator label mismatch, user accepts the rebuild prompt — the
// existing container is removed and the image is rebuilt from scratch.
func TestResolveAndLaunch_GeneratorHashMismatchPromptAccepts(t *testing.T) {
	probe := installLaunchStubs(t)
	called := installPromptStub(t, true)
	mock := docker.NewMock()
	mock.Containers[testContainer] = "running"
	mock.Images[testImage] = true

	var stderr bytes.Buffer
	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
		Stderr: &stderr,
	})
	mock.ImageLabels = map[string]map[string]string{
		testImage: {GeneratorLabelKey: "stale-hash-0000"},
	}

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}

	if !*called {
		t.Error("expected promptStaleRebuildFn to be invoked on mismatch")
	}
	if !probe.built {
		t.Error("expected a rebuild when the user accepts the prompt")
	}
	if !probe.ran {
		t.Error("expected a fresh runContainer after rebuilding")
	}
	if n := len(rmContainerRemovals(mock)); n != 1 {
		t.Errorf("expected exactly one container removal for the rebuild, got %d", n)
	}
}

// Case 4: an image predating the label entirely (no "vibrator.generator"
// label at all, distinct from Case 2/3's stale-but-present value) is
// treated identically to a mismatch — "have" resolves to "" and the
// warning reports it as unknown/pre-label.
func TestResolveAndLaunch_GeneratorLabelAbsentTreatedAsMismatch(t *testing.T) {
	probe := installLaunchStubs(t)
	called := installPromptStub(t, false)
	mock := docker.NewMock()
	mock.Images[testImage] = true
	// No mock.ImageLabels entry at all for testImage — simulates an image
	// built before this label existed.

	var stderr bytes.Buffer
	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
		Stderr: &stderr,
	})

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}

	if !*called {
		t.Error("expected promptStaleRebuildFn to be invoked for a pre-label image")
	}
	if probe.built {
		t.Error("expected NO rebuild when the user declines the prompt")
	}
	if !strings.Contains(stderr.String(), "unknown/pre-label") {
		t.Errorf("expected the warning to report the missing label as unknown/pre-label, got: %s", stderr.String())
	}
}

// Case 5: --rebuild is already set — the stale check must be skipped
// entirely (no ImageLabel call), since a from-scratch rebuild is already
// guaranteed regardless of the existing image's label.
func TestResolveAndLaunch_RebuildAlreadySetSkipsStaleCheck(t *testing.T) {
	probe := installLaunchStubs(t)
	mock := docker.NewMock()
	mock.Images[testImage] = true
	mock.ImageLabels = map[string]map[string]string{
		testImage: {GeneratorLabelKey: "stale-hash-0000"},
	}

	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
		Rebuild: true,
		Stderr:  &bytes.Buffer{},
	})

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}

	if !probe.built {
		t.Error("expected --rebuild to force a rebuild regardless of the label")
	}
	if imageLabelCalls(mock) {
		t.Error("expected NO ImageLabel call when --rebuild is already set")
	}
}

// Case 6: the image doesn't exist at all — the stale check must be skipped
// (no ImageLabel call) and the normal build-then-run path runs.
func TestResolveAndLaunch_ImageMissingSkipsStaleCheck(t *testing.T) {
	probe := installLaunchStubs(t)
	mock := docker.NewMock() // no image present

	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
		Stderr: &bytes.Buffer{},
	})

	if err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}

	if !probe.built {
		t.Error("expected a normal build when the image doesn't exist")
	}
	if imageLabelCalls(mock) {
		t.Error("expected NO ImageLabel call when the image doesn't exist")
	}
}

// Case 7: dc.ImageExists itself fails (e.g. a docker daemon hiccup) during
// the staleness check. The error must surface, not be silently swallowed —
// a prior version of this check used `if exists, err := ...; err == nil &&
// exists` which discarded a real error and fell through as if the image
// were simply absent, masking daemon failures from the user.
//
// The container is set up as already "running" so the ONLY ImageExists call
// resolveAndLaunch makes is the staleness check itself — the "" (no
// container) branch further down happens to make its own correctly-checked
// ImageExists call, which would mask this exact bug if a container weren't
// already present.
func TestResolveAndLaunch_ImageExistsErrorDuringStaleCheckSurfaces(t *testing.T) {
	installLaunchStubs(t)
	mock := docker.NewMock()
	mock.Containers[testContainer] = "running"
	wantErr := errors.New("docker: daemon unreachable")
	mock.ImageExistsErr = wantErr

	dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts := newResolveArgs(t, Options{
		Stderr: &bytes.Buffer{},
	})

	err := resolveAndLaunch(context.Background(), mock, dfSpec, wsSpec, pin,
		wsDir, imageTag, containerName, opts)
	if err == nil {
		t.Fatal("expected resolveAndLaunch to return an error when ImageExists fails, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected the returned error to wrap %v, got: %v", wantErr, err)
	}
}
