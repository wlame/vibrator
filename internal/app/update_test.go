package app

import (
	"encoding/json"
	"strings"
	"testing"
)

// dockerfileForUpdate is the pure-data generator the orchestrator uses
// to produce the one-layer Dockerfile. These tests pin its output
// shape so docker build's expectations don't drift away from what we
// emit.

func TestDockerfileForUpdate_HasFromAndRun(t *testing.T) {
	got := string(dockerfileForUpdate("vibrate-foo:abc", []string{"claude", "update"}, "1.2.3"))
	if !strings.Contains(got, "FROM vibrate-foo:abc\n") {
		t.Errorf("missing FROM directive:\n%s", got)
	}
	if !strings.Contains(got, "RUN ") {
		t.Errorf("missing RUN directive:\n%s", got)
	}
}

func TestDockerfileForUpdate_RunArgvIsJSONExecForm(t *testing.T) {
	// JSON exec form avoids the shell-quoting tarpit. Argv elements
	// containing spaces, quotes, or shell metacharacters round-trip
	// safely because Docker parses the array literally.
	got := string(dockerfileForUpdate("img:1", []string{"npm", "install", "-g", "@openai/codex@latest"}, ""))

	// Find the RUN line and parse its argv.
	var runLine string
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "RUN ") {
			runLine = line
			break
		}
	}
	if runLine == "" {
		t.Fatalf("no RUN line found:\n%s", got)
	}
	payload := strings.TrimPrefix(runLine, "RUN ")
	var argv []string
	if err := json.Unmarshal([]byte(payload), &argv); err != nil {
		t.Fatalf("RUN payload is not JSON exec form: %v\n%q", err, payload)
	}
	want := []string{"npm", "install", "-g", "@openai/codex@latest"}
	if len(argv) != len(want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, argv[i], want[i])
		}
	}
}

func TestDockerfileForUpdate_StampsVersionAndPreviousTag(t *testing.T) {
	got := string(dockerfileForUpdate("img:1", []string{"claude", "update"}, "v1.2.3"))
	if !strings.Contains(got, "v1.2.3") {
		t.Errorf("version not stamped:\n%s", got)
	}
	if !strings.Contains(got, "Previous image tag: img:1") {
		t.Errorf("previous tag not recorded (helps debug `docker history`):\n%s", got)
	}
}

func TestDockerfileForUpdate_DefaultsVersionToDev(t *testing.T) {
	got := string(dockerfileForUpdate("img:1", []string{"x"}, ""))
	if !strings.Contains(got, "dev") {
		t.Errorf("empty version did not fall back to 'dev':\n%s", got)
	}
}

func TestDockerfileForUpdate_SyntaxDirectiveFirst(t *testing.T) {
	// BuildKit's `# syntax=` MUST be on line 1 to take effect — same
	// rule the main generator follows.
	got := string(dockerfileForUpdate("img:1", []string{"x"}, ""))
	first := strings.SplitN(got, "\n", 2)[0]
	if !strings.HasPrefix(first, "# syntax=docker/dockerfile:") {
		t.Errorf("first line = %q, want # syntax= directive", first)
	}
}

func TestDockerfileForUpdate_PreservesShellMetacharacters(t *testing.T) {
	// Pathological case: argv containing characters that would break
	// shell form quoting. Exec form must round-trip them unchanged.
	weird := []string{"sh", "-c", `echo "hello world" && exit 0`}
	got := string(dockerfileForUpdate("img:1", weird, ""))

	var runLine string
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "RUN ") {
			runLine = line
		}
	}
	payload := strings.TrimPrefix(runLine, "RUN ")
	var argv []string
	if err := json.Unmarshal([]byte(payload), &argv); err != nil {
		t.Fatalf("not parseable JSON: %v\n%q", err, payload)
	}
	if argv[2] != `echo "hello world" && exit 0` {
		t.Errorf("argv[2] = %q, want round-trip of weird input", argv[2])
	}
}

func TestJoinForDisplay(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"claude", "update"}, "claude update"},
		{[]string{"npm", "install", "-g", "x@latest"}, "npm install -g x@latest"},
		{[]string{"single"}, "single"},
		{nil, ""},
		{[]string{}, ""},
	}
	for _, tc := range cases {
		got := joinForDisplay(tc.in)
		if got != tc.want {
			t.Errorf("joinForDisplay(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestUpdateOptions_StreamDefaults(t *testing.T) {
	// Update() should default nil streams to os.Stdout/Stderr/Stdin
	// rather than panic — pin that contract here without actually
	// calling Update (which needs a workspace + docker daemon).
	opts := UpdateOptions{}
	if opts.Stdout != nil {
		t.Errorf("zero-value Stdout = %v, want nil (filled in by Update)", opts.Stdout)
	}
	if opts.Stderr != nil {
		t.Errorf("zero-value Stderr = %v, want nil", opts.Stderr)
	}
}
