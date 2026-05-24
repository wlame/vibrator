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
	got, err := resolveLaunchCmd(pin, Options{}) // zero-value LaunchTarget
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
	got, err := resolveLaunchCmd(pin, Options{LaunchTarget: LaunchHarness})
	if err != nil {
		t.Fatalf("resolveLaunchCmd: %v", err)
	}
	if len(got) != 2 || got[1] != "codex" {
		t.Errorf("LaunchHarness argv = %v, want second element = codex", got)
	}
}

func TestResolveLaunchCmd_Shell(t *testing.T) {
	pin := config.Pin{Harness: "claude-code", Shell: "fish"}
	got, err := resolveLaunchCmd(pin, Options{LaunchTarget: LaunchShell})
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
	got, err := resolveLaunchCmd(pin, Options{LaunchTarget: LaunchShell})
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
			got, err := resolveLaunchCmd(config.Pin{Harness: id}, Options{LaunchTarget: LaunchHarness})
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
	_, err := resolveLaunchCmd(pin, Options{LaunchTarget: LaunchHarness})
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
		{"", LaunchHarness},     // zero value → harness
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
	_, err := resolveLaunchCmd(pin, Options{LaunchTarget: "weird"})
	if err == nil {
		t.Error("expected error for unknown launch target, got nil")
	}
}
