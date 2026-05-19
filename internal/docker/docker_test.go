package docker

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestMock_RecordsCalls(t *testing.T) {
	m := NewMock()
	ctx := context.Background()

	_ = m.Info(ctx)
	_ = m.Start(ctx, "vb-foo-abc")
	_, _ = m.ImageExists(ctx, "vb-claude-code-backend-abc12345:latest")

	if got := len(m.Calls); got != 3 {
		t.Fatalf("want 3 recorded calls, got %d: %+v", got, m.Calls)
	}
	if m.Calls[0][0] != "info" {
		t.Errorf("call[0] = %v, want [info ...]", m.Calls[0])
	}
	if m.Calls[1][0] != "start" || m.Calls[1][1] != "vb-foo-abc" {
		t.Errorf("call[1] = %v, want [start vb-foo-abc]", m.Calls[1])
	}
}

func TestMock_ImageExists_RespectsStub(t *testing.T) {
	m := NewMock()
	m.Images["present:latest"] = true

	ok, err := m.ImageExists(context.Background(), "present:latest")
	if err != nil || !ok {
		t.Errorf("want true,nil got %v,%v", ok, err)
	}

	ok, err = m.ImageExists(context.Background(), "missing:latest")
	if err != nil || ok {
		t.Errorf("want false,nil for missing image got %v,%v", ok, err)
	}
}

func TestMock_ContainerStatus_MissingIsNotError(t *testing.T) {
	m := NewMock()
	m.Containers["vb-running-abc"] = "running"

	status, err := m.ContainerStatus(context.Background(), "vb-running-abc")
	if err != nil || status != "running" {
		t.Errorf("want running,nil got %q,%v", status, err)
	}

	// Missing container: empty string + nil error.
	status, err = m.ContainerStatus(context.Background(), "vb-missing-abc")
	if err != nil || status != "" {
		t.Errorf("want empty,nil for missing container got %q,%v", status, err)
	}
}

func TestMock_PropagatesStubbedErrors(t *testing.T) {
	m := NewMock()

	wantErr := errSentinel("boom")
	m.RunErr = wantErr

	if err := m.Run(context.Background(), RunSpec{Image: "x:latest"}); err != wantErr {
		t.Errorf("want stubbed err, got %v", err)
	}
}

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

func TestMock_Reset(t *testing.T) {
	m := NewMock()
	_ = m.Info(context.Background())
	m.InfoErr = errSentinel("oops")
	m.Images["x"] = true

	m.Reset()

	if len(m.Calls) != 0 {
		t.Errorf("want 0 calls after Reset, got %d", len(m.Calls))
	}
	if m.InfoErr != nil {
		t.Errorf("InfoErr should be nil after Reset")
	}
	if len(m.Images) != 0 {
		t.Errorf("Images should be empty after Reset")
	}
}

func TestBuildRunArgs_FullSpec(t *testing.T) {
	spec := RunSpec{
		Image:         "vb-claude-code-backend-abc12345:latest",
		ContainerName: "vb-myproj-wshash-abc12345",
		Interactive:   true,
		Remove:        false,
		Init:          true,
		ShmSize:       "2g",
		Network:       "host",
		SecurityOpts:  []string{"no-new-privileges"},
		Volumes: []Volume{
			{Host: "/Users/x/proj", Container: "/Users/x/proj"},
			{Host: "/Users/x/.claude", Container: "/home/user/.claude", ReadOnly: true},
		},
		Env: []EnvVar{
			{Name: "ANTHROPIC_API_KEY", Value: "sk-..."},
		},
		Labels: map[string]string{
			"vibrator.managed":   "true",
			"vibrator.workspace": "/Users/x/proj",
		},
		Cmd: []string{"/bin/zsh"},
	}
	got := buildRunArgs(spec)

	// Spot-check key positions and presence rather than exact ordering.
	if got[0] != "run" {
		t.Errorf("first arg should be 'run', got %q", got[0])
	}
	if !containsSubseq(got, []string{"--init"}) {
		t.Error("missing --init")
	}
	if !containsSubseq(got, []string{"--shm-size", "2g"}) {
		t.Error("missing --shm-size 2g")
	}
	if !containsSubseq(got, []string{"--network", "host"}) {
		t.Error("missing --network host")
	}
	if !containsSubseq(got, []string{"--name", "vb-myproj-wshash-abc12345"}) {
		t.Error("missing --name")
	}
	if !containsSubseq(got, []string{"-v", "/Users/x/proj:/Users/x/proj"}) {
		t.Error("missing rw workspace mount")
	}
	if !containsSubseq(got, []string{"-v", "/Users/x/.claude:/home/user/.claude:ro"}) {
		t.Error("missing ro claude mount")
	}
	// Image and Cmd must be at the tail in that order.
	last := got[len(got)-2:]
	if !reflect.DeepEqual(last, []string{"vb-claude-code-backend-abc12345:latest", "/bin/zsh"}) {
		t.Errorf("want [image, cmd] tail, got %v", last)
	}
}

func TestBuildRunArgs_WorkingDirEmitsWhenSet(t *testing.T) {
	spec := RunSpec{
		Image:      "x:latest",
		WorkingDir: "/Users/wlame/dev/foo",
	}
	got := buildRunArgs(spec)
	if !containsSubseq(got, []string{"--workdir", "/Users/wlame/dev/foo"}) {
		t.Errorf("missing --workdir, got %v", got)
	}
}

func TestBuildRunArgs_WorkingDirOmittedWhenEmpty(t *testing.T) {
	spec := RunSpec{Image: "x:latest"}
	got := buildRunArgs(spec)
	for _, a := range got {
		if a == "--workdir" {
			t.Errorf("--workdir should not appear when WorkingDir is empty; got %v", got)
		}
	}
}

func TestBuildRunArgs_LabelsAreSorted(t *testing.T) {
	// Stable label emission is essential: image fingerprints depend on it.
	spec := RunSpec{
		Image: "x:latest",
		Labels: map[string]string{
			"z.label": "1",
			"a.label": "2",
			"m.label": "3",
		},
	}
	got := buildRunArgs(spec)

	// Find the indices of each label and check they appear in lex order.
	idxA := strings.Index(strings.Join(got, " "), "a.label=2")
	idxM := strings.Index(strings.Join(got, " "), "m.label=3")
	idxZ := strings.Index(strings.Join(got, " "), "z.label=1")
	if !(idxA >= 0 && idxA < idxM && idxM < idxZ) {
		t.Errorf("labels not in lex order: a@%d m@%d z@%d", idxA, idxM, idxZ)
	}
}

func TestVolumeString(t *testing.T) {
	cases := []struct {
		in   Volume
		want string
	}{
		{Volume{Host: "/h", Container: "/c"}, "/h:/c"},
		{Volume{Host: "/h", Container: "/c", ReadOnly: true}, "/h:/c:ro"},
	}
	for _, tc := range cases {
		if got := tc.in.String(); got != tc.want {
			t.Errorf("Volume{%+v}.String() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMockBuild_PassesDockerfileBytesAsStdin(t *testing.T) {
	m := NewMock()
	spec := BuildSpec{
		ContextDir:      ".",
		Tag:             "x:latest",
		DockerfileBytes: []byte("FROM scratch\n"),
	}
	if err := m.Build(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if len(m.Calls) != 1 {
		t.Fatalf("want 1 build call, got %d", len(m.Calls))
	}
	// "-f -" signals "Dockerfile on stdin" — that's what the mock should record.
	if !containsSubseq(m.Calls[0], []string{"-f", "-"}) {
		t.Errorf("expected -f - for stdin-fed dockerfile, got %v", m.Calls[0])
	}
}

// containsSubseq reports whether sub appears contiguously inside seq.
func containsSubseq(seq, sub []string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(seq); i++ {
		match := true
		for j := range sub {
			if seq[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
