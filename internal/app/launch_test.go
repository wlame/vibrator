package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/harness"
	"github.com/wlame/vibrator/internal/mount"
	"github.com/wlame/vibrator/internal/workspace"
)

func TestRunContainerIncludesExtraMounts(t *testing.T) {
	roDir := t.TempDir()
	rwDir := t.TempDir()
	ws := t.TempDir()

	dc := docker.NewMock()
	var captured docker.RunSpec
	dc.RunHandler = func(_ context.Context, spec docker.RunSpec) error {
		captured = spec
		return nil
	}

	pin := config.Pin{Harness: "claude-code", Shell: "zsh",
		Mounts: []string{roDir, rwDir + ":rw"}}
	opts := Options{LaunchTarget: LaunchHarness, Stdout: io.Discard, Stderr: io.Discard}

	err := runContainer(context.Background(), dc, "img:tag", "ctr", ws,
		workspace.Spec{Harness: "claude-code"}, pin, nil, opts)
	if err != nil {
		t.Fatalf("runContainer: %v", err)
	}

	var sawRO, sawRW bool
	for _, v := range captured.Volumes {
		if v.Host == roDir && v.Container == roDir && v.ReadOnly {
			sawRO = true
		}
		if v.Host == rwDir && v.Container == rwDir && !v.ReadOnly {
			sawRW = true
		}
	}
	if !sawRO {
		t.Errorf("read-only mount %s missing or not ro in RunSpec.Volumes: %+v", roDir, captured.Volumes)
	}
	if !sawRW {
		t.Errorf("read-write mount %s missing or not rw in RunSpec.Volumes: %+v", rwDir, captured.Volumes)
	}
}

func TestRunContainerMissingMountAborts(t *testing.T) {
	ws := t.TempDir()
	dc := docker.NewMock()

	pin := config.Pin{Harness: "claude-code", Shell: "zsh",
		Mounts: []string{"/definitely/does/not/exist/vb-missing"}}
	opts := Options{LaunchTarget: LaunchHarness, Stdout: io.Discard, Stderr: io.Discard}

	err := runContainer(context.Background(), dc, "img:tag", "ctr", ws,
		workspace.Spec{Harness: "claude-code"}, pin, nil, opts)
	if err == nil {
		t.Fatal("expected runContainer to abort on a missing --mount path, got nil")
	}
	if !strings.Contains(err.Error(), "no such directory") {
		t.Fatalf("error should mention the missing directory, got: %v", err)
	}
	// Fail-fast: no container was created.
	if runs := dc.CallsFor("run"); len(runs) != 0 {
		t.Errorf("expected NO docker run after a bad --mount, got %d run call(s)", len(runs))
	}
}

func TestExecIntoContainerAnnouncesMounts(t *testing.T) {
	roDir := t.TempDir()
	ws := t.TempDir()
	dc := docker.NewMock()
	var stderr bytes.Buffer

	pin := config.Pin{Harness: "claude-code", Shell: "zsh", Mounts: []string{roDir}}
	opts := Options{LaunchTarget: LaunchHarness, Stdout: io.Discard, Stderr: &stderr}

	if err := execIntoContainer(context.Background(), dc, "ctr", ws, pin, opts); err != nil {
		t.Fatalf("execIntoContainer: %v", err)
	}
	if !strings.Contains(stderr.String(), "Mounted 1 extra folder") {
		t.Fatalf("expected a mount notice on the reuse path, got: %q", stderr.String())
	}
}

// TestRunContainerCarriesYoloEnvOverride pins that runContainer forwards
// the runtime VIBRATOR_YOLO_ARGS override in RunSpec.Env — the mechanism
// that lets --no-yolo blank the in-container alias without a rebuild. Uses
// RunHandler (not the recorded Calls argv) because the recorded argv only
// captures env NAMEs (see docker.envArgs) — the VALUE only appears on the
// full RunSpec the handler receives.
func TestRunContainerCarriesYoloEnvOverride(t *testing.T) {
	ws := t.TempDir()

	for _, tc := range []struct {
		name    string
		noYolo  bool
		wantVal string
	}{
		{"bypass on by default", false, "--dangerously-skip-permissions"},
		{"no-yolo blanks it", true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dc := docker.NewMock()
			var captured docker.RunSpec
			dc.RunHandler = func(_ context.Context, spec docker.RunSpec) error {
				captured = spec
				return nil
			}

			pin := config.Pin{Harness: "claude-code", Shell: "zsh"}
			opts := Options{LaunchTarget: LaunchHarness, NoYolo: tc.noYolo, Stdout: io.Discard, Stderr: io.Discard}

			err := runContainer(context.Background(), dc, "img:tag", "ctr", ws,
				workspace.Spec{Harness: "claude-code"}, pin, nil, opts)
			if err != nil {
				t.Fatalf("runContainer: %v", err)
			}

			if !envContains(captured.Env, "VIBRATOR_YOLO_ARGS", tc.wantVal) {
				t.Errorf("RunSpec.Env = %+v, want VIBRATOR_YOLO_ARGS=%q", captured.Env, tc.wantVal)
			}
		})
	}
}

// TestExecIntoContainerCarriesYoloEnvOverride mirrors the run-path test
// above for the re-entry path: exec'ing into an already-running container
// must ALSO carry the override, so toggling --no-yolo on a second `vibrate`
// invocation against a live container still takes effect (the alias lives
// in the shell process env, not baked into a long-lived container state).
func TestExecIntoContainerCarriesYoloEnvOverride(t *testing.T) {
	ws := t.TempDir()

	for _, tc := range []struct {
		name    string
		noYolo  bool
		wantVal string
	}{
		{"bypass on by default", false, "--dangerously-skip-permissions"},
		{"no-yolo blanks it", true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dc := docker.NewMock()
			var captured docker.ExecSpec
			dc.ExecHandler = func(_ context.Context, spec docker.ExecSpec) error {
				captured = spec
				return nil
			}

			pin := config.Pin{Harness: "claude-code", Shell: "zsh"}
			opts := Options{LaunchTarget: LaunchHarness, NoYolo: tc.noYolo, Stdout: io.Discard, Stderr: io.Discard}

			err := execIntoContainer(context.Background(), dc, "ctr", ws, pin, opts)
			if err != nil {
				t.Fatalf("execIntoContainer: %v", err)
			}

			if !envContains(captured.Env, "VIBRATOR_YOLO_ARGS", tc.wantVal) {
				t.Errorf("ExecSpec.Env = %+v, want VIBRATOR_YOLO_ARGS=%q", captured.Env, tc.wantVal)
			}
		})
	}
}

// envContains reports whether env has an entry matching name/value exactly.
func envContains(env []docker.EnvVar, name, value string) bool {
	for _, e := range env {
		if e.Name == name && e.Value == value {
			return true
		}
	}
	return false
}

func TestMountVolumesAndDirs(t *testing.T) {
	rs := []mount.Resolved{
		{Path: "/data/refs", ReadOnly: true},
		{Path: "/work/lib", ReadOnly: false},
	}
	vols := mountVolumes(rs)
	if len(vols) != 2 {
		t.Fatalf("got %d volumes, want 2", len(vols))
	}
	if vols[0].Host != "/data/refs" || vols[0].Container != "/data/refs" || !vols[0].ReadOnly {
		t.Fatalf("vol0 = %+v", vols[0])
	}
	if vols[1].ReadOnly {
		t.Fatalf("vol1 should be writable: %+v", vols[1])
	}
	dirs := mountDirs(rs)
	if len(dirs) != 2 || dirs[0] != "/data/refs" || dirs[1] != "/work/lib" {
		t.Fatalf("dirs = %v", dirs)
	}
}

// ─── authURLWriter / writebackAuthToHost (LoginFlow-driven) ────────────────

// The URL scraper must honor an arbitrary marker (proves it's not hardcoded
// to claude's string).
func TestAuthURLWriter_ParameterizedMarker(t *testing.T) {
	var out bytes.Buffer
	w := newAuthURLWriter(&out, "GO HERE: ")
	// openBrowser is best-effort/non-blocking; we only assert passthrough +
	// that scraping doesn't corrupt the stream.
	in := "please auth\nGO HERE: https://example.com/device xyz\ndone\n"
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatal(err)
	}
	if out.String() != in {
		t.Errorf("passthrough altered: %q", out.String())
	}
}

// execHandlerReturning builds an ExecHandler that writes body to Stdout for
// any `cat` invocation — the shape writebackAuthToHost uses to read the
// container's auth file.
func execHandlerReturning(body string) func(context.Context, docker.ExecSpec) error {
	return func(_ context.Context, spec docker.ExecSpec) error {
		if len(spec.Cmd) > 0 && spec.Cmd[0] == "cat" {
			_, _ = spec.Stdout.Write([]byte(body))
		}
		return nil
	}
}

// Fields=["a"] merges only the named field, leaving other container keys
// out of the host file.
func TestWritebackAuthToHost_NamedFieldsMergesOnlyThose(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dc := docker.NewMock()
	dc.ExecHandler = execHandlerReturning(`{"a":"1","b":"2"}`)

	wb := &harness.AuthWriteback{
		ContainerRel: ".claude.json",
		HostRel:      ".claude.json",
		Fields:       []string{"a"},
	}
	if err := writebackAuthToHost(context.Background(), dc, "ctr", "alice", wb); err != nil {
		t.Fatalf("writebackAuthToHost: %v", err)
	}

	got := readJSONFile(t, filepath.Join(tmp, ".claude.json"))
	if _, ok := got["a"]; !ok {
		t.Errorf("expected field %q to be merged, got %v", "a", got)
	}
	if _, ok := got["b"]; ok {
		t.Errorf("field %q should NOT have been merged when Fields=[%q], got %v", "b", "a", got)
	}
}

// Fields=nil merges every top-level key from the container file.
func TestWritebackAuthToHost_EmptyFieldsMergesAllKeys(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dc := docker.NewMock()
	dc.ExecHandler = execHandlerReturning(`{"a":"1","b":"2"}`)

	wb := &harness.AuthWriteback{
		ContainerRel: ".claude.json",
		HostRel:      ".claude.json",
		// Fields intentionally nil.
	}
	if err := writebackAuthToHost(context.Background(), dc, "ctr", "alice", wb); err != nil {
		t.Fatalf("writebackAuthToHost: %v", err)
	}

	got := readJSONFile(t, filepath.Join(tmp, ".claude.json"))
	for _, key := range []string{"a", "b"} {
		if _, ok := got[key]; !ok {
			t.Errorf("expected field %q to be merged when Fields is empty, got %v", key, got)
		}
	}
}

// A pre-existing host file that fails to parse aborts the writeback entirely
// (no clobber) — silently continuing would erase every other host setting.
func TestWritebackAuthToHost_InvalidHostFileAborts(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	hostPath := filepath.Join(tmp, ".claude.json")
	const invalid = "{not valid json"
	mustWriteFile(t, hostPath, invalid)

	dc := docker.NewMock()
	dc.ExecHandler = execHandlerReturning(`{"a":"1"}`)

	wb := &harness.AuthWriteback{
		ContainerRel: ".claude.json",
		HostRel:      ".claude.json",
		Fields:       []string{"a"},
	}
	if err := writebackAuthToHost(context.Background(), dc, "ctr", "alice", wb); err == nil {
		t.Fatal("expected an error when the host file is invalid JSON, got nil")
	}

	data, err := os.ReadFile(hostPath)
	if err != nil {
		t.Fatalf("re-read host file: %v", err)
	}
	if string(data) != invalid {
		t.Errorf("host file was modified despite invalid JSON: got %q, want unchanged %q", data, invalid)
	}
}

// readJSONFile parses path as a JSON object, failing the test on any error.
func readJSONFile(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return got
}

// TestRunLoginStep_ClaudeCodeFlow_MatchesLegacyBehavior drives runLoginStep
// end to end through claude-code's real, registered harness.LoginFlow (not a
// hand-built fixture) to pin the byte-identical invariant: the exec argv, the
// URL marker, and the writeback field set must be exactly what the old
// hardcoded claudeAuthURLMarker/authFields machinery produced.
func TestRunLoginStep_ClaudeCodeFlow_MatchesLegacyBehavior(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	flow := loginFlowFor(config.Pin{Harness: "claude-code"})
	if flow == nil {
		t.Fatal("loginFlowFor(claude-code) = nil, want a populated LoginFlow")
	}

	var loginCmd []string
	dc := docker.NewMock()
	dc.ExecHandler = func(_ context.Context, spec docker.ExecSpec) error {
		if len(spec.Cmd) > 0 && spec.Cmd[0] == "cat" {
			// The container's ~/.claude.json — includes an extra field that
			// must NOT be merged, proving the field set is still scoped.
			_, _ = spec.Stdout.Write([]byte(`{
				"oauthAccount": "acct-1",
				"userID": "u-1",
				"hasCompletedOnboarding": true,
				"lastOnboardingVersion": "1.2.3",
				"subscriptionNoticeCount": 2,
				"hasAvailableSubscription": false,
				"s1mAccessCache": {"ok": true},
				"someUnrelatedSetting": "must-not-leak"
			}`))
			return nil
		}
		// The login exec itself — record the argv and emit the legacy URL line
		// through spec.Stdout to prove the marker scrape still fires.
		loginCmd = append([]string(nil), spec.Cmd...)
		_, _ = spec.Stdout.Write([]byte("If the browser didn't open, visit: https://example.com/oauth\n"))
		return nil
	}

	var stdout, stderr bytes.Buffer
	opts := Options{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}

	if err := runLoginStep(context.Background(), dc, "ctr", "alice", flow, opts); err != nil {
		t.Fatalf("runLoginStep: %v", err)
	}

	// Same exec argv as the old hardcoded []string{"claude", "auth", "login"}.
	wantCmd := []string{"claude", "auth", "login"}
	if len(loginCmd) != len(wantCmd) {
		t.Fatalf("login exec argv = %v, want %v", loginCmd, wantCmd)
	}
	for i, c := range wantCmd {
		if loginCmd[i] != c {
			t.Fatalf("login exec argv = %v, want %v", loginCmd, wantCmd)
		}
	}

	// The marker line still reaches the user's stdout (scraping is passthrough).
	if !strings.Contains(stdout.String(), "If the browser didn't open, visit: https://example.com/oauth") {
		t.Errorf("expected the auth URL line to pass through to stdout, got: %q", stdout.String())
	}

	// Same 7 writeback fields as the old hardcoded authFields — no more, no less.
	got := readJSONFile(t, filepath.Join(tmp, ".claude.json"))
	wantFields := []string{
		"oauthAccount", "userID", "hasCompletedOnboarding",
		"lastOnboardingVersion", "subscriptionNoticeCount",
		"hasAvailableSubscription", "s1mAccessCache",
	}
	for _, f := range wantFields {
		if _, ok := got[f]; !ok {
			t.Errorf("expected writeback field %q to be merged, got %v", f, got)
		}
	}
	if _, ok := got["someUnrelatedSetting"]; ok {
		t.Errorf("unrelated field leaked into the host writeback: %v", got)
	}
	if len(got) != len(wantFields) {
		t.Errorf("host file has %d fields, want exactly %d (%v): got %v", len(got), len(wantFields), wantFields, got)
	}
}
