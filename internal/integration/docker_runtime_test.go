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
