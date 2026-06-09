package app

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/extensions"
	"github.com/wlame/vibrator/internal/harness"
	_ "github.com/wlame/vibrator/internal/harness/all" // register built-in harnesses
	"github.com/wlame/vibrator/internal/prereq"
)

// --- needsWizard ----------------------------------------------------------

func TestNeedsWizard_TrueWhenHarnessEmpty(t *testing.T) {
	if !needsWizard(config.Pin{}) {
		t.Errorf("expected needsWizard=true on empty pin")
	}
}

func TestNeedsWizard_FalseWhenHarnessSet(t *testing.T) {
	if needsWizard(config.Pin{Harness: "claude-code"}) {
		t.Errorf("expected needsWizard=false when harness is set")
	}
}

// --- applyFlagOverrides ---------------------------------------------------

func TestApplyFlagOverrides_NonEmptyFlagsWin(t *testing.T) {
	pin := config.Pin{
		Harness: "claude-code",
		Profile: "minimal",
		Shell:   "bash",
		With:    []string{"old"},
	}
	opts := Options{
		Harness: "codex",
		Profile: "full",
		Shell:   "zsh",
		With:    []string{"playwright"},
	}
	applyFlagOverrides(&pin, opts)
	if pin.Harness != "codex" || pin.Profile != "full" || pin.Shell != "zsh" {
		t.Errorf("flags should override pin, got %+v", pin)
	}
	if !reflect.DeepEqual(pin.With, []string{"playwright"}) {
		t.Errorf("With should be replaced, got %v", pin.With)
	}
}

func TestApplyFlagOverrides_EmptyFlagsLeavePinIntact(t *testing.T) {
	pin := config.Pin{
		Harness: "claude-code",
		Profile: "full",
	}
	applyFlagOverrides(&pin, Options{})
	if pin.Harness != "claude-code" || pin.Profile != "full" {
		t.Errorf("empty flags should not clobber pin: %+v", pin)
	}
}

// --- validatePin ----------------------------------------------------------

func TestValidatePin_FailsWithoutHarness(t *testing.T) {
	if err := validatePin(config.Pin{}); err == nil {
		t.Errorf("expected error for empty harness")
	}
}

func TestValidatePin_FailsOnUnknownHarness(t *testing.T) {
	if err := validatePin(config.Pin{Harness: "not-a-real-harness"}); err == nil {
		t.Errorf("expected error for unknown harness")
	}
}

func TestValidatePin_AcceptsKnownHarness(t *testing.T) {
	if err := validatePin(config.Pin{Harness: "claude-code"}); err != nil {
		t.Errorf("claude-code should validate: %v", err)
	}
}

// --- resolveAPIKey --------------------------------------------------------

func TestResolveAPIKey_LocalProvidersReturnEmpty(t *testing.T) {
	for _, p := range []string{"ollama", "lmstudio"} {
		spec := &config.LLMSpec{Provider: p}
		got, err := resolveAPIKey(spec)
		if err != nil {
			t.Errorf("%s should not error: %v", p, err)
		}
		if got != "" {
			t.Errorf("%s should resolve to empty key, got %q", p, got)
		}
	}
}

func TestResolveAPIKey_PrefersAuthValueOverEnv(t *testing.T) {
	t.Setenv("TEST_LLM_KEY", "from-env")
	spec := &config.LLMSpec{
		Provider: "openai",
		Auth:     &config.LLMAuth{Value: "from-vb", Env: "TEST_LLM_KEY"},
	}
	got, err := resolveAPIKey(spec)
	if err != nil {
		t.Fatalf("resolveAPIKey: %v", err)
	}
	if got != "from-vb" {
		t.Errorf("expected literal value to win, got %q", got)
	}
}

func TestResolveAPIKey_FallsBackToHostEnv(t *testing.T) {
	t.Setenv("TEST_LLM_KEY_2", "from-env-fallback")
	spec := &config.LLMSpec{
		Provider: "openai",
		Auth:     &config.LLMAuth{Env: "TEST_LLM_KEY_2"},
	}
	got, err := resolveAPIKey(spec)
	if err != nil {
		t.Fatalf("resolveAPIKey: %v", err)
	}
	if got != "from-env-fallback" {
		t.Errorf("expected env value, got %q", got)
	}
}

func TestResolveAPIKey_ErrorsWhenEnvUnset(t *testing.T) {
	// Clear the var to ensure unset state.
	t.Setenv("DEFINITELY_UNSET_LLM_KEY_VIBRATE_TEST", "")
	// t.Setenv unsets at end of test; we also need it unset NOW, so
	// rely on os.Unsetenv via Setenv(""). The function reads via
	// os.Getenv which returns "" for empty AND unset values uniformly.
	spec := &config.LLMSpec{
		Provider: "openai",
		Auth:     &config.LLMAuth{Env: "DEFINITELY_UNSET_LLM_KEY_VIBRATE_TEST"},
	}
	_, err := resolveAPIKey(spec)
	if err == nil {
		t.Errorf("expected error for unset env var")
	}
}

func TestResolveAPIKey_ErrorsWhenNoAuthConfigured(t *testing.T) {
	spec := &config.LLMSpec{Provider: "openai", Auth: nil}
	_, err := resolveAPIKey(spec)
	if err == nil {
		t.Errorf("expected error when neither env nor value is set")
	}
}

// --- buildContainerEnv ----------------------------------------------------

func TestBuildContainerEnv_ClaudeCodeForwardsAuthVars(t *testing.T) {
	// Claude Code's AuthEnvVars: CLAUDE_CODE_OAUTH_TOKEN, ANTHROPIC_API_KEY.
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "tok-123")
	t.Setenv("ANTHROPIC_API_KEY", "ak-456")
	got, err := buildContainerEnv(config.Pin{Harness: "claude-code"}, nil)
	if err != nil {
		t.Fatalf("buildContainerEnv: %v", err)
	}
	envMap := envToMap(got)
	if envMap["CLAUDE_CODE_OAUTH_TOKEN"] != "tok-123" {
		t.Errorf("expected OAuth token forwarded, got %v", envMap)
	}
	if envMap["ANTHROPIC_API_KEY"] != "ak-456" {
		t.Errorf("expected ANTHROPIC_API_KEY forwarded, got %v", envMap)
	}
}

func TestBuildContainerEnv_CodexWithCloudLLM(t *testing.T) {
	got, err := buildContainerEnv(config.Pin{
		Harness: "codex",
		LLM: &config.LLMSpec{
			Provider: "openai", Model: "gpt-4o",
			Auth: &config.LLMAuth{Value: "sk-test"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("buildContainerEnv: %v", err)
	}
	envMap := envToMap(got)
	if envMap["OPENAI_API_KEY"] != "sk-test" {
		t.Errorf("expected OPENAI_API_KEY=sk-test, got %q", envMap["OPENAI_API_KEY"])
	}
}

func TestBuildContainerEnv_CodexWithOllama(t *testing.T) {
	got, err := buildContainerEnv(config.Pin{
		Harness: "codex",
		LLM: &config.LLMSpec{
			Provider: "ollama", Model: "qwen3:32b",
			BaseURL: "http://host.docker.internal:11434",
		},
	}, nil)
	if err != nil {
		t.Fatalf("buildContainerEnv: %v", err)
	}
	envMap := envToMap(got)
	if envMap["OPENAI_API_KEY"] != "ollama" {
		t.Errorf("expected literal 'ollama' key, got %q", envMap["OPENAI_API_KEY"])
	}
	if envMap["OPENAI_BASE_URL"] != "http://host.docker.internal:11434/v1" {
		t.Errorf("expected /v1 suffix appended, got %q", envMap["OPENAI_BASE_URL"])
	}
}

func TestBuildContainerEnv_PinEnvOverridesWithDollarIndirection(t *testing.T) {
	t.Setenv("MY_HOST_TOKEN", "host-val")
	got, err := buildContainerEnv(config.Pin{
		Harness: "claude-code",
		Env: map[string]string{
			"LITERAL":  "literal-val",
			"INDIRECT": "$MY_HOST_TOKEN",
		},
	}, nil)
	if err != nil {
		t.Fatalf("buildContainerEnv: %v", err)
	}
	envMap := envToMap(got)
	if envMap["LITERAL"] != "literal-val" {
		t.Errorf("literal env var lost: %v", envMap)
	}
	if envMap["INDIRECT"] != "host-val" {
		t.Errorf("$NAME indirection failed: %v", envMap)
	}
}

func TestBuildContainerEnv_ExtensionAuthForwarded(t *testing.T) {
	// Regression: previously, declaring `auth.env: OPENAI_API_KEY` on
	// an extension entry rendered a badge in the wizard but NEVER
	// forwarded the host's value at `docker run -e` time. The plugin
	// then failed inside the container with "no API key" — silently
	// for users who hadn't read the runtime-needs notes.
	t.Setenv("OPENAI_API_KEY", "sk-ext-host")
	ext := &extensions.Entry{
		ID:      "codex-plugin-cc",
		Harness: "claude-code",
		Kind:    extensions.KindPlugin,
		Auth:    &extensions.AuthSpec{Env: "OPENAI_API_KEY"},
	}
	got, err := buildContainerEnv(config.Pin{Harness: "claude-code"}, []*extensions.Entry{ext})
	if err != nil {
		t.Fatalf("buildContainerEnv: %v", err)
	}
	envMap := envToMap(got)
	if envMap["OPENAI_API_KEY"] != "sk-ext-host" {
		t.Errorf("OPENAI_API_KEY not forwarded from extension auth.env: %v", envMap)
	}
}

func TestBuildContainerEnv_ExtensionAuth_SilentWhenUnset(t *testing.T) {
	// If the user hasn't set the host env var, we silently skip the
	// extension's auth.env rather than erroring out — they may be
	// using an alternative auth path (OAuth, ambient identity, etc.).
	t.Setenv("DEFINITELY_UNSET_VIBRATE_TEST_AUTH", "")
	ext := &extensions.Entry{
		ID:      "x",
		Harness: "claude-code",
		Kind:    extensions.KindPlugin,
		Auth:    &extensions.AuthSpec{Env: "DEFINITELY_UNSET_VIBRATE_TEST_AUTH"},
	}
	got, err := buildContainerEnv(config.Pin{Harness: "claude-code"}, []*extensions.Entry{ext})
	if err != nil {
		t.Fatalf("buildContainerEnv: %v", err)
	}
	envMap := envToMap(got)
	if _, present := envMap["DEFINITELY_UNSET_VIBRATE_TEST_AUTH"]; present {
		t.Errorf("unset env var should not appear in output: %v", envMap)
	}
}

func TestBuildContainerEnv_HarnessAuthBeatsExtensionAuth(t *testing.T) {
	// Same env name declared by both the harness (AuthEnvVars) and an
	// extension (auth.env) — the harness value wins because it's a
	// more fundamental declaration. Pin this so a future refactor
	// doesn't silently invert the precedence.
	t.Setenv("ANTHROPIC_API_KEY", "from-harness")
	ext := &extensions.Entry{
		ID:      "ext-wants-anthropic",
		Harness: "claude-code",
		Kind:    extensions.KindPlugin,
		// Pathological case — extension declares the same env name
		// the harness already forwards.
		Auth: &extensions.AuthSpec{Env: "ANTHROPIC_API_KEY"},
	}
	got, _ := buildContainerEnv(config.Pin{Harness: "claude-code"}, []*extensions.Entry{ext})
	envMap := envToMap(got)
	if envMap["ANTHROPIC_API_KEY"] != "from-harness" {
		t.Errorf("harness auth should win over extension auth, got %v", envMap)
	}
}

func TestBuildContainerEnv_PinEnvWinsOverExtensionAuth(t *testing.T) {
	// pin.Env is the user's explicit per-workspace override; it must
	// win over an extension's auth.env hint.
	t.Setenv("OPENAI_API_KEY", "from-host-env")
	ext := &extensions.Entry{
		ID:      "x",
		Harness: "claude-code",
		Kind:    extensions.KindPlugin,
		Auth:    &extensions.AuthSpec{Env: "OPENAI_API_KEY"},
	}
	got, _ := buildContainerEnv(
		config.Pin{
			Harness: "claude-code",
			Env:     map[string]string{"OPENAI_API_KEY": "from-pin"},
		},
		[]*extensions.Entry{ext},
	)
	envMap := envToMap(got)
	if envMap["OPENAI_API_KEY"] != "from-pin" {
		t.Errorf("pin.Env should beat extension auth, got %v", envMap)
	}
}

func TestBuildContainerEnv_PinEnvWinsOverHarnessAuth(t *testing.T) {
	// pin.Env precedence: a user override should win even over a
	// harness-declared auth env var.
	t.Setenv("ANTHROPIC_API_KEY", "from-host")
	got, _ := buildContainerEnv(config.Pin{
		Harness: "claude-code",
		Env:     map[string]string{"ANTHROPIC_API_KEY": "from-pin"},
	}, nil)
	envMap := envToMap(got)
	if envMap["ANTHROPIC_API_KEY"] != "from-pin" {
		t.Errorf("pin.Env should win over harness auth, got %v", envMap)
	}
}

func TestBuildContainerEnv_StableOrder(t *testing.T) {
	// Two calls with the same pin should produce identical (sorted) output.
	pin := config.Pin{
		Harness: "claude-code",
		Env:     map[string]string{"A": "1", "B": "2", "C": "3"},
	}
	a, _ := buildContainerEnv(pin, nil)
	b, _ := buildContainerEnv(pin, nil)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("buildContainerEnv not deterministic: %v vs %v", a, b)
	}
	// And the names should be sorted.
	names := make([]string, len(a))
	for i, ev := range a {
		names[i] = ev.Name
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(names, sorted) {
		t.Errorf("env vars not sorted by name: %v", names)
	}
}

// --- sanitizeUsername ------------------------------------------------------

func TestSanitizeUsername(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// Common host names — pass through unchanged once lowercased.
		{"wlame", "wlame"},
		{"alice", "alice"},
		{"john_doe", "john_doe"},
		{"user-1", "user-1"},

		// macOS-style mixed case → lowercased.
		{"Wlame", "wlame"},
		{"JohnDoe", "johndoe"},

		// Invalid chars replaced with `_`.
		{"john.doe", "john_doe"},
		{"jane doe", "jane_doe"},
		{"user@host", "user_host"},

		// Leading digit → prefixed with `_`.
		{"1user", "_1user"},

		// Leading dash: `-` is a valid follow-char but not a valid
		// starter, so the starter-fix pass prepends `_`. Result is
		// `_-leading`, which matches the useradd regex `[a-z_][a-z0-9_-]*`.
		// Ugly but legal — and a user with a leading `-` in their host
		// name is in pathological territory anyway.
		{"-leading", "_-leading"},

		// Long names — truncated to 32 chars.
		{strings.Repeat("a", 40), strings.Repeat("a", 32)},

		// Empty → empty (HostUsername then falls back).
		{"", ""},

		// All-invalid → underscores; if first char wasn't already a
		// letter/underscore, prepend `_`. "@@@" → "___" → starts with `_`,
		// which is valid, so result is "___".
		{"@@@", "___"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := sanitizeUsername(tc.in); got != tc.want {
				t.Errorf("sanitizeUsername(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// --- hostMountsToVolumes (claude-code harness) ----------------------------

// claudeEncoded mirrors claudecode.encodedProjectDir (the "/"→"-" path
// encoding) so the test can predict the scoped projects mount name
// without importing the unexported helper.
func claudeEncoded(wsDir string) string { return strings.ReplaceAll(wsDir, "/", "-") }

// claudeSessionDirs is the D5b contract the test asserts against; it
// mirrors claudecode.sessionPersistDirs.
var claudeSessionDirs = []string{"file-history", "sessions", "tasks", "paste-cache"}

func claudeHarness(t *testing.T) harness.Harness {
	t.Helper()
	h, ok := harness.ByID("claude-code")
	if !ok {
		t.Fatal("claude-code harness not registered")
	}
	return h
}

func TestHostMountsToVolumes_SkipsMissingPaths(t *testing.T) {
	// HOME points at an empty tempdir → no host-state files exist → no
	// config/settings/rules/hooks mounts. Session-persist dirs DO get
	// auto-created (D5 contract), so we still see those mounts.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	const wsDir = "/home/alice/project"
	var stderr bytes.Buffer
	got := hostMountsToVolumes(claudeHarness(t), "alice", wsDir, &stderr)

	mounts := make(map[string]docker.Volume, len(got))
	for _, v := range got {
		mounts[v.Container] = v
	}
	for _, c := range []string{
		"/home/alice/.claude.host.json",
		"/home/alice/.claude/settings.host.json",
		"/home/alice/.claude/rules-host",
		"/home/alice/.claude/hooks",
	} {
		if _, present := mounts[c]; present {
			t.Errorf("unexpected mount %s on bare HOME — should only mount when host source exists", c)
		}
	}

	// projects/ must be scoped to the workspace's encoded-cwd subdir, not the whole dir.
	wantProjectsContainer := "/home/alice/.claude/projects/" + claudeEncoded(wsDir)
	if _, present := mounts[wantProjectsContainer]; !present {
		t.Errorf("missing scoped projects mount %s — D5a contract requires auto-create", wantProjectsContainer)
	}
	if _, present := mounts["/home/alice/.claude/projects"]; present {
		t.Errorf("unexpected full projects/ mount — should be scoped to workspace subdir")
	}

	// Remaining session-persist dirs (D5b) are still mounted wholesale.
	for _, name := range claudeSessionDirs {
		c := "/home/alice/.claude/" + name
		if _, present := mounts[c]; !present {
			t.Errorf("missing session-persist mount %s — D5b contract requires auto-create", c)
		}
	}
}

func TestHostMountsToVolumes_MountsExistingHostState(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	mustWriteFile(t, filepath.Join(tmp, ".claude.json"), `{"oauthAccount":"x"}`)
	mustMkdirAll(t, filepath.Join(tmp, ".claude"))
	mustWriteFile(t, filepath.Join(tmp, ".claude", "settings.json"), `{}`)
	mustMkdirAll(t, filepath.Join(tmp, ".claude", "rules"))
	mustMkdirAll(t, filepath.Join(tmp, ".claude", "hooks"))

	const wsDir = "/home/alice/project"
	var stderr bytes.Buffer
	got := hostMountsToVolumes(claudeHarness(t), "alice", wsDir, &stderr)

	mounts := make(map[string]docker.Volume, len(got))
	for _, v := range got {
		mounts[v.Container] = v
	}

	cases := []struct {
		container string
		wantHost  string
		wantRO    bool
	}{
		{"/home/alice/.claude.host.json", filepath.Join(tmp, ".claude.json"), true},
		{"/home/alice/.claude/settings.host.json", filepath.Join(tmp, ".claude", "settings.json"), true},
		{"/home/alice/.claude/rules-host", filepath.Join(tmp, ".claude", "rules"), true},
		// hooks is now READ-ONLY (container-escape hardening).
		{"/home/alice/.claude/hooks", filepath.Join(tmp, ".claude", "hooks"), true},
	}
	for _, c := range cases {
		v, ok := mounts[c.container]
		if !ok {
			t.Errorf("missing mount %s", c.container)
			continue
		}
		if v.Host != c.wantHost {
			t.Errorf("mount %s host = %q, want %q", c.container, v.Host, c.wantHost)
		}
		if v.ReadOnly != c.wantRO {
			t.Errorf("mount %s ReadOnly = %v, want %v", c.container, v.ReadOnly, c.wantRO)
		}
	}
}

// A MountFileIfExists descriptor must NOT mount when the host source is a
// directory (and vice versa) — the kind guards against type confusion.
func TestHostMountsToVolumes_FileKindIgnoresDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// Create ~/.claude.json as a DIRECTORY — a MountFileIfExists mount
	// must skip it rather than bind a dir where a file is expected.
	mustMkdirAll(t, filepath.Join(tmp, ".claude.json"))

	var stderr bytes.Buffer
	got := hostMountsToVolumes(claudeHarness(t), "alice", "/home/alice/project", &stderr)
	for _, v := range got {
		if v.Container == "/home/alice/.claude.host.json" {
			t.Errorf("MountFileIfExists bound a directory source: %+v", v)
		}
	}
}

// joinUnderRoot must reject a descriptor that climbs out of its home root.
func TestJoinUnderRoot_RejectsEscape(t *testing.T) {
	root := "/home/alice"
	if _, ok := joinUnderRoot(root, "../bob/.ssh"); ok {
		t.Error("joinUnderRoot accepted a path escaping root")
	}
	if p, ok := joinUnderRoot(root, ".claude/settings.json"); !ok || p != "/home/alice/.claude/settings.json" {
		t.Errorf("joinUnderRoot(%q) = (%q, %v), want (/home/alice/.claude/settings.json, true)", ".claude/settings.json", p, ok)
	}
}

// --- buildOptionalMounts (D6 + D7) ----------------------------------------

func TestBuildOptionalMounts_NoAWSNoExtension_ReturnsEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	var stderr bytes.Buffer
	got := buildOptionalMounts("alice", config.Pin{}, "abc12345", &stderr)
	if len(got) != 0 {
		t.Errorf("bare HOME + no extensions: want 0 mounts, got %d: %+v", len(got), got)
	}
}

func TestBuildOptionalMounts_AWSDirMountsReadOnly(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	mustMkdirAll(t, filepath.Join(tmp, ".aws"))

	var stderr bytes.Buffer
	got := buildOptionalMounts("alice", config.Pin{}, "abc12345", &stderr)

	var found *docker.Volume
	for i := range got {
		if got[i].Container == "/home/alice/.aws" {
			found = &got[i]
		}
	}
	if found == nil {
		t.Fatalf("expected /home/alice/.aws mount, got %+v", got)
	}
	if found.Host != filepath.Join(tmp, ".aws") {
		t.Errorf("host path = %q, want %q", found.Host, filepath.Join(tmp, ".aws"))
	}
	if !found.ReadOnly {
		t.Error("AWS creds mount must be read-only — container should not be able to rotate or wipe them")
	}
}

func TestBuildOptionalMounts_ClaudeMemExtensionMountsCacheRW(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	pin := config.Pin{Extensions: []string{"claude-mem"}}
	var stderr bytes.Buffer
	got := buildOptionalMounts("alice", pin, "abc12345", &stderr)

	wantHost := filepath.Join(tmp, ".cache", "vibrator", "claude-mem", "abc12345")
	wantContainer := "/home/alice/.claude-mem/cache"

	var found *docker.Volume
	for i := range got {
		if got[i].Container == wantContainer {
			found = &got[i]
		}
	}
	if found == nil {
		t.Fatalf("expected %s mount, got %+v", wantContainer, got)
	}
	if found.Host != wantHost {
		t.Errorf("host = %q, want %q", found.Host, wantHost)
	}
	if found.ReadOnly {
		t.Error("claude-mem cache mount must be RW — the plugin writes to it")
	}
	// The mount-creation helper must have auto-created the host dir;
	// otherwise the docker mount would silently create it as root and
	// the unprivileged container user couldn't write there.
	if !isDir(wantHost) {
		t.Errorf("buildOptionalMounts should have created host cache dir %s", wantHost)
	}
}

// --- buildClaudeMemEnv -------------------------------------------------------

func TestBuildClaudeMemEnv_ReturnsNilWithoutPrereqs(t *testing.T) {
	// Pin has no bootstrapped claude-mem prereqs → nothing to forward.
	got := buildClaudeMemEnv(config.Pin{Harness: "claude-code"})
	if len(got) != 0 {
		t.Errorf("expected nil env with no prereqs, got %v", got)
	}
}

func TestBuildClaudeMemEnv_ReturnsNilWithoutAdminConfig(t *testing.T) {
	// Prereqs present but no admin config file → can't learn ServerURL.
	tmp := t.TempDir()
	t.Setenv("VIBRATOR_CLAUDE_MEM_CONFIG", filepath.Join(tmp, "missing.toml"))

	pin := config.Pin{
		Prereqs: map[string]map[string]string{
			prereq.ClaudeMemPrereqID: {"api_key": "cmem_abc", "team_id": "t1", "project_id": "p1"},
		},
	}
	got := buildClaudeMemEnv(pin)
	if len(got) != 0 {
		t.Errorf("expected nil env when admin config missing, got %v", got)
	}
}

func TestBuildClaudeMemEnv_ReturnsNilWhenServerURLEmpty(t *testing.T) {
	// Admin config present but ServerURL is blank → incomplete config.
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "claude-mem.toml")
	mustWriteFileDirect(cfgPath, `runtime = "server-beta"`+"\n")
	t.Setenv("VIBRATOR_CLAUDE_MEM_CONFIG", cfgPath)

	pin := config.Pin{
		Prereqs: map[string]map[string]string{
			prereq.ClaudeMemPrereqID: {"api_key": "cmem_abc"},
		},
	}
	got := buildClaudeMemEnv(pin)
	if len(got) != 0 {
		t.Errorf("expected nil env when ServerURL empty, got %v", got)
	}
}

func TestBuildClaudeMemEnv_ForwardsAllFields(t *testing.T) {
	// Full admin config + full bootstrap result → all CLAUDE_MEM_* vars emitted.
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "claude-mem.toml")
	mustWriteFileDirect(cfgPath, `runtime = "server-beta"`+"\n"+
		`server_url = "http://host.docker.internal:37877"`+"\n")
	t.Setenv("VIBRATOR_CLAUDE_MEM_CONFIG", cfgPath)

	pin := config.Pin{
		Prereqs: map[string]map[string]string{
			prereq.ClaudeMemPrereqID: {
				"api_key":    "cmem_testkey",
				"team_id":    "team-42",
				"project_id": "proj-99",
			},
		},
	}
	got := buildClaudeMemEnv(pin)
	env := make(map[string]string, len(got))
	for _, e := range got {
		env[e.Name] = e.Value
	}

	wants := map[string]string{
		"CLAUDE_MEM_RUNTIME":                "server-beta",
		"CLAUDE_MEM_SERVER_BETA_URL":        "http://host.docker.internal:37877",
		"CLAUDE_MEM_SERVER_BETA_API_KEY":    "cmem_testkey",
		"CLAUDE_MEM_SERVER_BETA_TEAM_ID":    "team-42",
		"CLAUDE_MEM_SERVER_BETA_PROJECT_ID": "proj-99",
	}
	for name, want := range wants {
		if got := env[name]; got != want {
			t.Errorf("env %s = %q, want %q", name, got, want)
		}
	}
}

func TestBuildClaudeMemEnv_SkipsMissingOptionalFields(t *testing.T) {
	// Bootstrap result has only api_key (team/project not yet minted) — the
	// function must not emit empty-string entries for the absent fields.
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "claude-mem.toml")
	mustWriteFileDirect(cfgPath, `runtime = "server-beta"`+"\n"+
		`server_url = "http://host.docker.internal:37877"`+"\n")
	t.Setenv("VIBRATOR_CLAUDE_MEM_CONFIG", cfgPath)

	pin := config.Pin{
		Prereqs: map[string]map[string]string{
			prereq.ClaudeMemPrereqID: {"api_key": "cmem_only"},
		},
	}
	got := buildClaudeMemEnv(pin)
	env := make(map[string]string, len(got))
	for _, e := range got {
		env[e.Name] = e.Value
	}

	if _, ok := env["CLAUDE_MEM_SERVER_BETA_TEAM_ID"]; ok {
		t.Error("TEAM_ID should not be emitted when absent from bootstrap result")
	}
	if _, ok := env["CLAUDE_MEM_SERVER_BETA_PROJECT_ID"]; ok {
		t.Error("PROJECT_ID should not be emitted when absent from bootstrap result")
	}
	if env["CLAUDE_MEM_SERVER_BETA_API_KEY"] != "cmem_only" {
		t.Errorf("API_KEY = %q, want cmem_only", env["CLAUDE_MEM_SERVER_BETA_API_KEY"])
	}
}

// mustWriteFileDirect writes body to path unconditionally, creating parent dirs.
// Separate from mustWriteFile (which uses t.Helper and testing.T) so we can
// call it before the testing.T helper is defined in file order.
func mustWriteFileDirect(path, body string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic("mustWriteFileDirect mkdir: " + err.Error())
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		panic("mustWriteFileDirect write: " + err.Error())
	}
}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

// --- buildSpecs: extensions deps fold into feature set ----------------------

// Regression: extension entries declare `deps.features` in their frontmatter
// (e.g., filesystem-mcp needs `node` for `npm install -g …`). Before the
// fix in buildSpecs, those declarations were documentation-only and the
// resolved feature list contained only the profile baseline — so the
// install snippet hit "npm: not found" at docker build time.
func TestBuildSpecs_ExtensionDepsAreFoldedIntoFeatures(t *testing.T) {
	// `backend` profile has no node; filesystem-mcp's deps.features = [node]
	// — so a successful merge means the resolved feature list includes node.
	pin := config.Pin{
		Harness:    "claude-code",
		Profile:    "backend",
		Extensions: []string{"filesystem-mcp"},
	}
	_, ws, err := buildSpecs(pin, Options{})
	if err != nil {
		t.Fatalf("buildSpecs: %v", err)
	}
	if !containsString(ws.Features, "node") {
		t.Errorf("expected resolved Features to include \"node\" via extensions dep, got %v", ws.Features)
	}
}

// When --dind is set, buildSpecs must auto-inject the docker-cli feature so
// the container has the docker binary needed to talk to the mounted socket.
// The user can still strip it with --no=docker-cli if they supply their own.
func TestBuildSpecs_DinDInjectsDockerCLIFeature(t *testing.T) {
	pin := config.Pin{
		Harness: "claude-code",
		Profile: "minimal", // minimal has no features — clean baseline
	}
	_, ws, err := buildSpecs(pin, Options{DinD: true})
	if err != nil {
		t.Fatalf("buildSpecs with DinD: %v", err)
	}
	if !containsString(ws.Features, "docker-cli") {
		t.Errorf("expected docker-cli in resolved Features when DinD=true, got %v", ws.Features)
	}
}

// --no=docker-cli must still be able to override the auto-injected feature.
func TestBuildSpecs_DinDDockerCLICanBeRemovedWithNo(t *testing.T) {
	pin := config.Pin{
		Harness: "claude-code",
		Profile: "minimal",
		No:      []string{"docker-cli"},
	}
	_, ws, err := buildSpecs(pin, Options{DinD: true})
	if err != nil {
		t.Fatalf("buildSpecs with DinD + --no=docker-cli: %v", err)
	}
	if containsString(ws.Features, "docker-cli") {
		t.Errorf("--no=docker-cli should override DinD auto-inject, got %v", ws.Features)
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// --- helper ---------------------------------------------------------------

func envToMap(vars []docker.EnvVar) map[string]string {
	m := make(map[string]string, len(vars))
	for _, e := range vars {
		m[e.Name] = e.Value
	}
	return m
}
