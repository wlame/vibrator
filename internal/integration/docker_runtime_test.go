package integration

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" {
		t.Skip("HOME unset — skipping expansion tests")
	}
	cases := []struct {
		in, want string
	}{
		{"~", home},
		{"~/.config/foo", home + "/.config/foo"},
		{"/absolute/path", "/absolute/path"},
		{"./relative", "./relative"},
		{"", ""},
		// "~user" is intentionally NOT expanded — we only support
		// the bare-$HOME shape. Keep it untouched.
		{"~user/path", "~user/path"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := expandHome(tc.in)
			if got != tc.want {
				t.Errorf("expandHome(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDockerRuntime_KindAndLabel(t *testing.T) {
	d := &DockerRuntime{Restart: "unless-stopped"}
	if d.Kind() != "docker" {
		t.Errorf("Kind = %q", d.Kind())
	}
	if !strings.Contains(d.Label(), "unless-stopped") {
		t.Errorf("Label = %q, expected to mention restart policy", d.Label())
	}

	d2 := &DockerRuntime{}
	if strings.Contains(d2.Label(), "--restart") {
		t.Errorf("Label = %q, should not mention --restart when empty", d2.Label())
	}
}

func TestDockerRuntime_StatusMissingContainer(t *testing.T) {
	// `docker inspect` for a non-existent container returns
	// a non-zero exit code — DockerRuntime.Status should treat
	// that as "not running" without error.
	d := &DockerRuntime{ContainerName: "vibrate-nonexistent-test-marker-xyz"}
	got, err := d.Status(context.Background())
	if err != nil {
		t.Errorf("Status on missing container: %v (want nil)", err)
	}
	if got.Running {
		t.Error("Status reported Running=true on a non-existent container")
	}
}

func TestDockerRuntime_StopMissingIsNoop(t *testing.T) {
	d := &DockerRuntime{ContainerName: "vibrate-nonexistent-test-marker-stop"}
	if err := d.Stop(context.Background()); err != nil {
		t.Errorf("Stop on missing container: %v (want nil)", err)
	}
}

func TestDockerRuntime_LogsMissingIsEmpty(t *testing.T) {
	d := &DockerRuntime{ContainerName: "vibrate-nonexistent-test-marker-logs"}
	got, err := d.Logs(context.Background(), 1024)
	if err != nil {
		t.Errorf("Logs on missing container: %v", err)
	}
	if got != "" {
		t.Errorf("Logs = %q, want empty", got)
	}
}

func TestDockerRuntime_BuildRunFlagsNoSecretValuesInArgv(t *testing.T) {
	// Secret values authored in a TOML integration's [env] table (self-hosted
	// MCP containers) must never land in the docker CLI's argv (visible via
	// ps and /proc/*/cmdline on the host) — same guarantee internal/docker's
	// CLIClient.Run makes. Only the env var NAME may appear in the flags;
	// the VALUE travels out-of-band via the returned EnvVar list, which the
	// caller feeds into docker.WriteEnvFile (--env-file) and the subprocess
	// environment — never as "-e NAME=VALUE".
	d := &DockerRuntime{
		ContainerName: "test-container",
		Image:         "img:latest",
		Env: map[string]string{
			"API_KEY":    "hunter2",
			"OTHER_FLAG": "public-value-ok-to-see",
		},
	}
	flags, env := d.buildRunFlags()

	sawNames := map[string]bool{}
	for i, a := range flags {
		if strings.Contains(a, "hunter2") {
			t.Errorf("secret value leaked into argv: %q", a)
		}
		if strings.Contains(a, "public-value-ok-to-see") {
			t.Errorf("env value leaked into argv (even non-secret values must stay name-only): %q", a)
		}
		if a == "-e" {
			if i+1 >= len(flags) {
				t.Fatalf("-e flag has no following value")
			}
			name := flags[i+1]
			if strings.Contains(name, "=") {
				t.Errorf("-e arg %q looks like NAME=VALUE, want a bare name", name)
			}
			sawNames[name] = true
		}
	}
	for _, want := range []string{"API_KEY", "OTHER_FLAG"} {
		if !sawNames[want] {
			t.Errorf("-e flags = %v, missing name-only entry for %q", flags, want)
		}
	}

	if len(env) != 2 {
		t.Fatalf("buildRunFlags env = %v, want 2 entries carrying the values out-of-band", env)
	}
	values := map[string]string{}
	for _, e := range env {
		values[e.Name] = e.Value
	}
	if values["API_KEY"] != "hunter2" || values["OTHER_FLAG"] != "public-value-ok-to-see" {
		t.Errorf("env values = %v, want the original map values preserved", values)
	}
}
