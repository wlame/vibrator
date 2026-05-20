package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeFileExists(t *testing.T) {
	dir := t.TempDir()
	if composeFileExists(dir) {
		t.Error("composeFileExists on empty dir = true (want false)")
	}

	// Drop in docker-compose.yml — should detect.
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !composeFileExists(dir) {
		t.Error("composeFileExists with docker-compose.yml = false (want true)")
	}
}

func TestComposeFileExists_AlternateNames(t *testing.T) {
	cases := []string{"docker-compose.yaml", "compose.yml", "compose.yaml"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, name), []byte("services: {}\n"), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			if !composeFileExists(dir) {
				t.Errorf("composeFileExists with %s = false", name)
			}
		})
	}
}

func TestComposeRuntime_StatusMissingDirIsNotRunning(t *testing.T) {
	c := &ComposeRuntime{Dir: filepath.Join(t.TempDir(), "absent")}
	got, err := c.Status(context.Background())
	if err != nil {
		t.Errorf("Status on missing dir: %v (want nil)", err)
	}
	if got.Running {
		t.Error("Status reported Running=true on missing dir")
	}
}

func TestComposeRuntime_StopMissingIsNoop(t *testing.T) {
	c := &ComposeRuntime{Dir: filepath.Join(t.TempDir(), "absent")}
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop on missing dir: %v (want nil)", err)
	}
}

func TestComposeRuntime_LogsMissingIsEmpty(t *testing.T) {
	c := &ComposeRuntime{Dir: filepath.Join(t.TempDir(), "absent")}
	got, err := c.Logs(context.Background(), 1024)
	if err != nil {
		t.Errorf("Logs on missing dir: %v", err)
	}
	if got != "" {
		t.Errorf("Logs = %q, want empty", got)
	}
}

func TestComposeRuntime_WriteOverrideMaterializesFile(t *testing.T) {
	dir := t.TempDir()
	c := &ComposeRuntime{
		Dir:              dir,
		OverrideFilename: "docker-compose.override.yml",
		OverrideContent: func() (string, error) {
			return "services:\n  foo:\n    image: bar\n", nil
		},
	}
	if err := c.writeOverride(dir); err != nil {
		t.Fatalf("writeOverride: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "docker-compose.override.yml"))
	if err != nil {
		t.Fatalf("read override: %v", err)
	}
	if !strings.Contains(string(got), "image: bar") {
		t.Errorf("override content = %q, expected to contain 'image: bar'", got)
	}
}

func TestComposeRuntime_EmptyOverrideContentSkipsWrite(t *testing.T) {
	dir := t.TempDir()
	c := &ComposeRuntime{
		Dir:              dir,
		OverrideFilename: "docker-compose.override.yml",
		OverrideContent: func() (string, error) {
			return "", nil // signals "skip"
		},
	}
	if err := c.writeOverride(dir); err != nil {
		t.Fatalf("writeOverride: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "docker-compose.override.yml")); !os.IsNotExist(err) {
		t.Errorf("override file written despite empty content (stat err: %v)", err)
	}
}

func TestComposeRuntime_NoOverrideFunctionSkipsWrite(t *testing.T) {
	dir := t.TempDir()
	c := &ComposeRuntime{Dir: dir, OverrideFilename: "ignored.yml"}
	if err := c.writeOverride(dir); err != nil {
		t.Fatalf("writeOverride: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ignored.yml")); !os.IsNotExist(err) {
		t.Errorf("override file written with nil OverrideContent (stat err: %v)", err)
	}
}

func TestComposeRuntime_OverrideHas0600Mode(t *testing.T) {
	// Override files typically embed DSNs / API tokens. Verify the
	// permissions are 0600 to avoid other-readable secret files.
	dir := t.TempDir()
	c := &ComposeRuntime{
		Dir:              dir,
		OverrideFilename: "override.yml",
		OverrideContent: func() (string, error) {
			return "services: {}\n", nil
		},
	}
	if err := c.writeOverride(dir); err != nil {
		t.Fatalf("writeOverride: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "override.yml"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("perm = %o, want 0600", mode)
	}
}

func TestContainsString(t *testing.T) {
	cases := []struct {
		hay  []string
		need string
		want bool
	}{
		{[]string{"a", "b"}, "a", true},
		{[]string{"a", "b"}, "c", false},
		{nil, "anything", false},
	}
	for _, tc := range cases {
		got := containsString(tc.hay, tc.need)
		if got != tc.want {
			t.Errorf("containsString(%v, %q) = %v, want %v", tc.hay, tc.need, got, tc.want)
		}
	}
}
