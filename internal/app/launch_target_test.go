package app

import (
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/config"
	_ "github.com/wlame/vibrator/internal/harness/all"
)

// Tests for resolveLaunchCmd — the function that decides what argv the
// orchestrator exec's inside the container. We can't drive `docker
// run` from a unit test, but we CAN pin the argv decisions so a
// regression here (e.g., dropping the claude-exec wrapper, picking
// the wrong harness binary) surfaces in CI.

func TestResolveLaunchCmd_DefaultLaunchesHarness(t *testing.T) {
	pin := config.Pin{Harness: "claude-code", Shell: "zsh"}
	// NoYolo isolates this test to the binary-selection concern; the
	// permission-bypass append is covered by its own tests below.
	got, err := resolveLaunchCmd(pin, Options{NoYolo: true}, nil) // zero-value LaunchTarget
	if err != nil {
		t.Fatalf("resolveLaunchCmd: %v", err)
	}
	// Expect [claude-exec, claude] — wrapper first, harness CLI next.
	if len(got) != 2 || got[0] != "/usr/local/bin/claude-exec" || got[1] != "claude" {
		t.Errorf("zero-value target argv = %v, want [/usr/local/bin/claude-exec claude]", got)
	}
}

func TestResolveLaunchCmd_ExplicitHarness(t *testing.T) {
	pin := config.Pin{Harness: "codex", Shell: "bash"}
	// NoYolo isolates this test to the binary-selection concern; the
	// permission-bypass append is covered by its own tests below.
	got, err := resolveLaunchCmd(pin, Options{LaunchTarget: LaunchHarness, NoYolo: true}, nil)
	if err != nil {
		t.Fatalf("resolveLaunchCmd: %v", err)
	}
	if len(got) != 2 || got[1] != "codex" {
		t.Errorf("LaunchHarness argv = %v, want second element = codex", got)
	}
}

func TestResolveLaunchCmd_Shell(t *testing.T) {
	pin := config.Pin{Harness: "claude-code", Shell: "fish"}
	got, err := resolveLaunchCmd(pin, Options{LaunchTarget: LaunchShell}, nil)
	if err != nil {
		t.Fatalf("resolveLaunchCmd: %v", err)
	}
	// Expect [claude-exec, /bin/fish].
	if len(got) != 2 || got[0] != "/usr/local/bin/claude-exec" || got[1] != "/bin/fish" {
		t.Errorf("LaunchShell argv = %v, want [/usr/local/bin/claude-exec /bin/fish]", got)
	}
}

func TestResolveLaunchCmd_Shell_DefaultsToZsh(t *testing.T) {
	// pin.Shell unset should fall back to zsh — the documented default.
	pin := config.Pin{Harness: "claude-code"}
	got, err := resolveLaunchCmd(pin, Options{LaunchTarget: LaunchShell}, nil)
	if err != nil {
		t.Fatalf("resolveLaunchCmd: %v", err)
	}
	if len(got) != 2 || got[1] != "/bin/zsh" {
		t.Errorf("shell with empty pin.Shell = %v, want second element /bin/zsh", got)
	}
}

func TestResolveLaunchCmd_EachHarness(t *testing.T) {
	cases := map[string]string{
		"claude-code": "claude",
		"codex":       "codex",
		"opencode":    "opencode",
		"pi":          "pi",
	}
	for id, wantBin := range cases {
		t.Run(id, func(t *testing.T) {
			got, err := resolveLaunchCmd(config.Pin{Harness: id}, Options{LaunchTarget: LaunchHarness}, nil)
			if err != nil {
				t.Fatalf("resolveLaunchCmd: %v", err)
			}
			if len(got) < 2 || got[1] != wantBin {
				t.Errorf("argv = %v, want second element %q", got, wantBin)
			}
			if got[0] != "/usr/local/bin/claude-exec" {
				t.Errorf("missing claude-exec wrapper: argv = %v", got)
			}
		})
	}
}

func TestResolveLaunchCmd_UnknownHarnessFails(t *testing.T) {
	pin := config.Pin{Harness: "totally-not-a-harness"}
	_, err := resolveLaunchCmd(pin, Options{LaunchTarget: LaunchHarness}, nil)
	if err == nil {
		t.Fatal("expected error for unknown harness, got nil")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("error %q should mention registration", err.Error())
	}
}

func TestLaunchTarget_EffectiveNormalization(t *testing.T) {
	cases := []struct {
		in   LaunchTarget
		want LaunchTarget
	}{
		{"", LaunchHarness}, // zero value → harness
		{LaunchHarness, LaunchHarness},
		{LaunchShell, LaunchShell},
		// Unknown values pass through; resolveLaunchCmd errors on them
		// downstream rather than silently picking a default.
		{"weird", "weird"},
	}
	for _, tc := range cases {
		t.Run(string(tc.in), func(t *testing.T) {
			if got := tc.in.effective(); got != tc.want {
				t.Errorf("effective(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveLaunchCmd_UnknownTargetErrors(t *testing.T) {
	pin := config.Pin{Harness: "claude-code"}
	_, err := resolveLaunchCmd(pin, Options{LaunchTarget: "weird"}, nil)
	if err == nil {
		t.Error("expected error for unknown launch target, got nil")
	}
}

func TestResolveLaunchCmdAppendsAddDir(t *testing.T) {
	pin := config.Pin{Harness: "claude-code"}
	got, err := resolveLaunchCmd(pin, Options{LaunchTarget: LaunchHarness},
		[]string{"/data/refs", "/work/lib"})
	if err != nil {
		t.Fatalf("resolveLaunchCmd: %v", err)
	}
	// Bypass args land after the harness binary and before add-dir args.
	want := []string{"/usr/local/bin/claude-exec", "claude",
		"--dangerously-skip-permissions",
		"--add-dir", "/data/refs", "--add-dir", "/work/lib"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestResolveLaunchCmdShellIgnoresAddDir(t *testing.T) {
	pin := config.Pin{Harness: "claude-code", Shell: "zsh"}
	got, err := resolveLaunchCmd(pin, Options{LaunchTarget: LaunchShell},
		[]string{"/data/refs"})
	if err != nil {
		t.Fatalf("resolveLaunchCmd: %v", err)
	}
	want := []string{"/usr/local/bin/claude-exec", "/bin/zsh"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// containsSeq reports whether b appears immediately after a somewhere in s.
func containsSeq(s []string, a, b string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == a && s[i+1] == b {
			return true
		}
	}
	return false
}

func TestResolveLaunchCmd_AppendsBypassByDefault(t *testing.T) {
	pin := config.Pin{Harness: "claude-code"}
	argv, err := resolveLaunchCmd(pin, Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// argv is ["/usr/local/bin/claude-exec", "claude", "--dangerously-skip-permissions"]
	if !containsSeq(argv, "claude", "--dangerously-skip-permissions") {
		t.Errorf("argv = %v, want claude followed by the bypass flag", argv)
	}
}

func TestResolveLaunchCmd_NoYoloOmitsBypass(t *testing.T) {
	pin := config.Pin{Harness: "claude-code"}
	argv, err := resolveLaunchCmd(pin, Options{NoYolo: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range argv {
		if a == "--dangerously-skip-permissions" {
			t.Errorf("--no-yolo should omit the bypass flag; argv = %v", argv)
		}
	}
}

// TestYoloEnvVar pins the runtime override yoloEnvVar computes for
// VIBRATOR_YOLO_ARGS — the env var the in-container shell alias (see
// templates/shells/{zshrc,bashrc,config.fish}) keys off of. This is a
// SEPARATE decision from resolveLaunchCmd's argv append above: the argv
// append affects the direct-launch `vibrate` invocation, while this env var
// overrides the build-baked ENV default so an already-built image's alias
// reflects --no-yolo without a rebuild.
func TestYoloEnvVar(t *testing.T) {
	cases := []struct {
		name    string
		harness string
		noYolo  bool
		want    string
	}{
		{"claude-code default is bypass args", "claude-code", false, "--dangerously-skip-permissions"},
		{"claude-code no-yolo blanks it", "claude-code", true, ""},
		{"codex default is its own bypass args", "codex", false, "--dangerously-bypass-approvals-and-sandbox"},
		{"codex no-yolo blanks it", "codex", true, ""},
		{"opencode has no bypass args regardless", "opencode", false, ""},
		{"unknown harness degrades to empty", "totally-not-a-harness", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pin := config.Pin{Harness: tc.harness}
			got := yoloEnvVar(pin, Options{NoYolo: tc.noYolo})
			if got.Name != "VIBRATOR_YOLO_ARGS" {
				t.Errorf("Name = %q, want VIBRATOR_YOLO_ARGS", got.Name)
			}
			if got.Value != tc.want {
				t.Errorf("Value = %q, want %q", got.Value, tc.want)
			}
		})
	}
}
