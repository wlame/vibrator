package hostprobe

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// --- Core registry --------------------------------------------------------

func TestRegistry_BuiltinsPresent(t *testing.T) {
	want := []string{"claude-code", "codex", "opencode", "pi"}
	for _, id := range want {
		if _, ok := ByID(id); !ok {
			t.Errorf("expected built-in prober %q to be registered", id)
		}
	}
}

func TestHarnessIDs_Sorted(t *testing.T) {
	ids := HarnessIDs()
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(ids, sorted) {
		t.Errorf("HarnessIDs not sorted: %v", ids)
	}
}

func TestProbeAll_RunsAllProbers(t *testing.T) {
	tmp := t.TempDir() // empty home — every harness reports Installed=false
	results, err := ProbeAll(tmp)
	if err != nil {
		t.Fatalf("ProbeAll: %v", err)
	}
	for _, id := range []string{"claude-code", "codex", "opencode", "pi"} {
		d, ok := results[id]
		if !ok {
			t.Errorf("missing result for %s", id)
		}
		if d.HarnessID != id {
			t.Errorf("HarnessID = %q, want %q", d.HarnessID, id)
		}
	}
}

// --- Registration safety --------------------------------------------------

func TestRegister_NilPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic on nil register")
		}
	}()
	Register(nil)
}

// --- Claude Code prober ---------------------------------------------------

func TestClaudeCodeProber_NotInstalled(t *testing.T) {
	tmp := t.TempDir()
	d, err := claudeCodeProber{}.Probe(tmp)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if d.Installed {
		t.Errorf("expected Installed=false when ~/.claude is absent")
	}
	if d.HomeDir == "" {
		t.Errorf("expected HomeDir to be populated even when not installed")
	}
}

func TestClaudeCodeProber_NewManifestParsing(t *testing.T) {
	tmp := t.TempDir()
	seedClaudeHome(t, tmp, map[string]string{
		"plugins/installed_plugins.json": `{
			"version": 2,
			"plugins": {
				"context7@claude-plugins-official": [{"scope":"user"}],
				"claude-mem@thedotmack":             [{"scope":"user"}],
				"my-local-plugin":                   [{"scope":"user"}]
			}
		}`,
	})

	d, err := claudeCodeProber{}.Probe(tmp)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if !d.Installed {
		t.Errorf("expected Installed=true after seeding ~/.claude")
	}
	want := []string{"claude-mem", "context7", "my-local-plugin"}
	if !reflect.DeepEqual(d.PluginIDs, want) {
		t.Errorf("PluginIDs = %v, want %v", d.PluginIDs, want)
	}
	if len(d.Notes) == 0 || !strings.Contains(d.Notes[0], "installed_plugins.json") {
		t.Errorf("expected installed_plugins.json note, got %v", d.Notes)
	}
}

func TestClaudeCodeProber_LegacyFallback(t *testing.T) {
	tmp := t.TempDir()
	// New manifest absent; legacy settings.json present.
	seedClaudeHome(t, tmp, map[string]string{
		"settings.json": `{
			"enabledPlugins": {
				"serena@claude-plugins-official":      true,
				"feature-dev@claude-plugins-official": true,
				"some-disabled-plugin@foo":            false
			}
		}`,
	})

	d, err := claudeCodeProber{}.Probe(tmp)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	want := []string{"feature-dev", "serena"}
	if !reflect.DeepEqual(d.PluginIDs, want) {
		t.Errorf("PluginIDs = %v, want %v (legacy fallback)", d.PluginIDs, want)
	}
}

func TestClaudeCodeProber_PrefersNewOverLegacy(t *testing.T) {
	tmp := t.TempDir()
	// Both files present — new manifest should win.
	seedClaudeHome(t, tmp, map[string]string{
		"plugins/installed_plugins.json": `{
			"version": 2,
			"plugins": {"new-plugin@m": [{}]}
		}`,
		"settings.json": `{
			"enabledPlugins": {"old-plugin@m": true}
		}`,
	})

	d, _ := claudeCodeProber{}.Probe(tmp)
	if !contains(d.PluginIDs, "new-plugin") || contains(d.PluginIDs, "old-plugin") {
		t.Errorf("expected new-plugin only, got %v", d.PluginIDs)
	}
}

func TestClaudeCodeProber_MarketplacesFromKnownMarketplaces(t *testing.T) {
	tmp := t.TempDir()
	// Mirror the real known_marketplaces.json shape: top-level object
	// whose keys are short marketplace IDs.
	seedClaudeHome(t, tmp, map[string]string{
		"plugins/known_marketplaces.json": `{
			"claude-plugins-official": {
				"source": {"source": "github", "repo": "anthropics/claude-plugins-official"}
			},
			"umputun-cc-thingz": {
				"source": {"source": "github", "repo": "umputun/cc-thingz"}
			},
			"thedotmack": {
				"source": {"source": "github", "repo": "thedotmack/claude-mem"}
			}
		}`,
	})

	d, err := claudeCodeProber{}.Probe(tmp)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	want := []string{"claude-plugins-official", "thedotmack", "umputun-cc-thingz"}
	if !reflect.DeepEqual(d.Marketplaces, want) {
		t.Errorf("Marketplaces = %v, want %v (note: short IDs, NOT github repo paths)", d.Marketplaces, want)
	}
}

func TestClaudeCodeProber_MCPServersFromRootConfig(t *testing.T) {
	tmp := t.TempDir()
	seedClaudeHome(t, tmp, nil) // empty ~/.claude/

	rootCfg := filepath.Join(tmp, ".claude.json")
	if err := os.WriteFile(rootCfg, []byte(`{
		"mcpServers": {
			"context7":             {"url":"http://x"},
			"serena":               {"command":"serena"},
			"sequential-thinking":  {"command":"npx"}
		}
	}`), 0o600); err != nil {
		t.Fatalf("seed root config: %v", err)
	}

	d, err := claudeCodeProber{}.Probe(tmp)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	want := []string{"context7", "sequential-thinking", "serena"}
	if !reflect.DeepEqual(d.MCPServers, want) {
		t.Errorf("MCPServers = %v, want %v", d.MCPServers, want)
	}
}

func TestClaudeCodeProber_BrokenJSONIsSilent(t *testing.T) {
	tmp := t.TempDir()
	// Garbage JSON should not crash the prober — degraded detection is OK.
	seedClaudeHome(t, tmp, map[string]string{
		"plugins/installed_plugins.json": `{not valid json`,
	})
	d, err := claudeCodeProber{}.Probe(tmp)
	if err != nil {
		// Probe itself doesn't error — the read+parse errors are swallowed.
		t.Fatalf("Probe should not error on bad JSON: %v", err)
	}
	if !d.Installed {
		t.Errorf("expected Installed=true since ~/.claude exists")
	}
	if len(d.PluginIDs) != 0 {
		t.Errorf("expected empty PluginIDs on broken JSON, got %v", d.PluginIDs)
	}
}

// --- splitAtMarketplace -------------------------------------------------

func TestSplitAtMarketplace(t *testing.T) {
	cases := map[string]string{
		"context7@claude-plugins-official": "context7",
		"claude-mem@thedotmack":            "claude-mem",
		"local-only":                       "local-only",
		"@just-marketplace":                "@just-marketplace", // leading @ → not a split
	}
	for in, want := range cases {
		if got := splitAtMarketplace(in); got != want {
			t.Errorf("splitAtMarketplace(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- Codex prober ---------------------------------------------------------

func TestCodexProber_NotInstalled(t *testing.T) {
	tmp := t.TempDir()
	d, _ := codexProber{}.Probe(tmp)
	if d.Installed {
		t.Errorf("expected Installed=false")
	}
}

func TestCodexProber_InstalledWithSkills(t *testing.T) {
	tmp := t.TempDir()
	codex := filepath.Join(tmp, ".codex")
	if err := os.MkdirAll(filepath.Join(codex, "skills"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// One dir-style skill, one single-file skill, one dotfile (ignored).
	if err := os.MkdirAll(filepath.Join(codex, "skills", "plugin-creator"), 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codex, "skills", "single.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write single-file skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codex, "skills", ".hidden"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write dotfile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codex, "config.toml"), []byte("# codex"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	d, err := codexProber{}.Probe(tmp)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if !d.Installed {
		t.Errorf("expected Installed=true")
	}
	want := []string{"plugin-creator", "single"}
	if !reflect.DeepEqual(d.PluginIDs, want) {
		t.Errorf("PluginIDs = %v, want %v", d.PluginIDs, want)
	}
	noteHasConfig := false
	for _, n := range d.Notes {
		if strings.Contains(n, "config.toml") {
			noteHasConfig = true
		}
	}
	if !noteHasConfig {
		t.Errorf("expected config.toml mention in Notes, got %v", d.Notes)
	}
}

// --- OpenCode prober ------------------------------------------------------

func TestOpenCodeProber_DetectsAlternativeLocations(t *testing.T) {
	tmp := t.TempDir()
	// Drop ~/.local/share/opencode/ — the canonical location for the auth dir.
	if err := os.MkdirAll(filepath.Join(tmp, ".local", "share", "opencode"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	d, _ := openCodeProber{}.Probe(tmp)
	if !d.Installed {
		t.Errorf("expected Installed=true after seeding ~/.local/share/opencode")
	}
	if !strings.Contains(d.HomeDir, "opencode") {
		t.Errorf("HomeDir should point at detected dir, got %q", d.HomeDir)
	}
}

func TestOpenCodeProber_NotInstalled(t *testing.T) {
	tmp := t.TempDir()
	d, _ := openCodeProber{}.Probe(tmp)
	if d.Installed {
		t.Errorf("expected Installed=false")
	}
}

// --- Pi prober ------------------------------------------------------------

func TestPiProber_DetectsHomeDir(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".pi"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	d, _ := piProber{}.Probe(tmp)
	if !d.Installed {
		t.Errorf("expected Installed=true after seeding ~/.pi")
	}
}

// --- helpers --------------------------------------------------------------

// seedClaudeHome creates `tmp/.claude/` and writes the supplied files
// relative to that root. Keys are paths relative to .claude/; values are
// file contents. Parent directories are created automatically.
func seedClaudeHome(t *testing.T, tmp string, files map[string]string) {
	t.Helper()
	home := filepath.Join(tmp, ".claude")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	for rel, content := range files {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
