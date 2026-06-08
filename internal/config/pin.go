// Package config loads and saves the `.vb` per-workspace pin file (TOML).
//
// The pin file records:
//   - the harness, profile, and shell the user picked (so future `vibrate` runs
//     in this workspace can skip the wizard entirely)
//   - delta feature toggles on top of the profile (with/no lists)
//   - per-harness extensions selections
//   - cached prerequisite tokens (e.g., claude-mem project-scoped API key)
//     that were minted on first run by host-side bootstrap and shouldn't be
//     re-minted on subsequent invocations
//   - optional environment variable overrides forwarded into the container
//
// `.vb` is found by walking up from $PWD to either the git root (when the
// workspace is inside a git repo) or the filesystem root. The first hit wins.
//
// SECURITY: the pin file holds plaintext credentials when prereqs have been
// bootstrapped. Mode is set to 0600 on write. Callers should ensure `.vb` is
// gitignored — AppendToGitignore() exists for that.
package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// PinFileName is the workspace-scoped pin file Vibrator looks for.
const PinFileName = ".vb"

// Pin is the in-memory representation of the TOML pin file. Field tags drive
// both the encoder and the decoder, so renaming a Go field without changing
// the tag is a backward-compatible change (and vice versa).
type Pin struct {
	Harness string `toml:"harness,omitempty"`
	Profile string `toml:"profile,omitempty"`
	Shell   string `toml:"shell,omitempty"`

	// With and No are deltas relative to the chosen profile's feature bundle.
	// Resolving them happens in internal/feature, not here.
	With    []string `toml:"with,omitempty"`
	No      []string `toml:"no,omitempty"`
	Extensions []string `toml:"extensions,omitempty"`

	// LLM is the chosen LLM provider + model + auth shape. nil for harnesses
	// that don't need it (Claude Code is Anthropic-only and uses the existing
	// AuthEnvVars forwarding). See LLMSpec for the per-field semantics.
	LLM *LLMSpec `toml:"llm,omitempty"`

	// Prereqs[prereq_id] = arbitrary key/value pairs (api_key, team_id, ...).
	// Schema is loose by design — each prereq's bootstrap step decides what
	// it persists. Map iteration order is randomized; we sort keys on save
	// for stable file diffs.
	Prereqs map[string]map[string]string `toml:"prereqs,omitempty"`

	// Env is a host->container environment variable forwarding map. Values
	// of the form "$NAME" are treated as references to the host env and
	// resolved at container-run time, NOT at pin-load time. Plain values
	// are forwarded as-is.
	Env map[string]string `toml:"env,omitempty"`

	// Integrations records the user's hosting preference per host-side
	// integration (e.g. "serena", "claude-mem"). Keyed by integration id;
	// the value is one of the IntegrationMode* constants. A missing key
	// means IntegrationAuto. This is a design-time choice — the container's
	// claude-exec wrapper reads it at runtime to decide whether to use the
	// host server (http) or a container-local fallback (stdio). Persisted
	// under the [integrations] table in .vb.
	Integrations map[string]string `toml:"integrations,omitempty"`
}

// Integration hosting modes. Stored as the value in Pin.Integrations and
// forwarded into the container so claude-exec can wire the right transport.
const (
	// IntegrationAuto probes the host server and falls back to a
	// container-local instance if it's unreachable. The default.
	IntegrationAuto = "auto"
	// IntegrationHost requires the host server: use http and warn loudly
	// if it's unreachable rather than silently falling back.
	IntegrationHost = "host"
	// IntegrationLocal always uses the container-local instance and never
	// probes the host.
	IntegrationLocal = "local"
	// IntegrationOff disables the integration entirely (no MCP wiring).
	IntegrationOff = "off"
)

// IntegrationMode returns the configured hosting mode for the given
// integration id, defaulting to IntegrationAuto when unset or unknown.
func (p Pin) IntegrationMode(id string) string {
	switch p.Integrations[id] {
	case IntegrationHost:
		return IntegrationHost
	case IntegrationLocal:
		return IntegrationLocal
	case IntegrationOff:
		return IntegrationOff
	default:
		return IntegrationAuto
	}
}

// LLMSpec captures the user's LLM-provider choice for harnesses that
// support multiple providers (Codex, OpenCode, Pi). Persisted under the
// `[llm]` table in `.vb`.
//
// Provider values:
//   - "anthropic"      — Anthropic cloud (Claude models)
//   - "openai"         — OpenAI cloud (GPT models)
//   - "ollama"         — local Ollama server (host.docker.internal:11434 by default)
//   - "lmstudio"       — local LM Studio server (host.docker.internal:1234 by default)
//   - "openai-compat"  — any OpenAI-compatible HTTP endpoint (user-supplied URL)
//
// For local providers (`ollama`, `lmstudio`), Auth is nil — no key required.
// For cloud and `openai-compat`, Auth carries either an env var name
// (Approach C path 1) or a literal value (Approach C path 2).
type LLMSpec struct {
	// Provider is the canonical provider id (see list above).
	Provider string `toml:"provider"`

	// Model is the model identifier in the provider's namespace.
	// Examples: "gpt-4o", "claude-3-5-sonnet-20241022", "qwen3:32b".
	Model string `toml:"model"`

	// BaseURL is the endpoint to talk to. Empty = use the provider's
	// canonical default (e.g., https://api.openai.com for "openai",
	// http://host.docker.internal:11434 for "ollama").
	BaseURL string `toml:"base_url,omitempty"`

	// Auth is the credential plan. nil = no credential needed
	// (local providers).
	Auth *LLMAuth `toml:"auth,omitempty"`
}

// LLMAuth carries credentials for cloud providers. Exactly one of Env
// or Value should be set; both empty is a configuration bug.
//
// SECURITY: when Value is set, the pin file holds a plaintext API key.
// The pin is saved with mode 0600 and the workspace's .gitignore is
// updated to cover it. The host env-var path (Env) is preferred — it
// keeps the secret in the user's shell, never in repo-adjacent files.
type LLMAuth struct {
	// Env is the name (not value) of a host environment variable
	// carrying the API key. The orchestrator forwards $Env into the
	// container at `docker run` time. Examples: "OPENAI_API_KEY".
	Env string `toml:"env,omitempty"`

	// Value is a literal API key pasted into the wizard. Mutually
	// exclusive with Env. Stored plaintext in `.vb` (0600 + gitignored).
	Value string `toml:"value,omitempty"`
}

// IsEmpty reports whether the pin carries any data worth saving.
// Used to avoid writing an empty file when the user opts out of pinning
// mid-wizard.
func (p Pin) IsEmpty() bool {
	return p.Harness == "" && p.Profile == "" && p.Shell == "" &&
		len(p.With) == 0 && len(p.No) == 0 && len(p.Extensions) == 0 &&
		p.LLM == nil &&
		len(p.Prereqs) == 0 && len(p.Env) == 0 && len(p.Integrations) == 0
}

// Load reads a .vb file from path and decodes it into a Pin.
// Returns os.ErrNotExist if the file isn't there — callers commonly probe
// without caring whether it exists, so we surface a distinguishable error.
func Load(path string) (*Pin, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Pin
	if _, err := toml.Decode(string(data), &p); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return &p, nil
}

// Save writes a Pin to path as TOML with mode 0600.
//
// Map keys (Prereqs, Env) are emitted in sorted order so the file produces
// stable diffs across reorderings. This is done by walking the maps in
// sorted order and constructing the TOML by hand for those sections;
// scalar/list fields use the encoder.
//
// The default BurntSushi encoder doesn't expose a map-sort hook, hence the
// custom assembly. Trivial — TOML is small and we control the schema.
func Save(path string, p *Pin) error {
	var b strings.Builder
	b.WriteString("# vibrator workspace pin (`.vb`) — auto-managed by `vibrate`.\n")
	b.WriteString("# Plaintext prereq tokens may live in [prereqs.*] subtables — keep gitignored.\n\n")

	// Scalars + simple lists first. We use the encoder for these because it
	// already handles quoting and edge cases (apostrophes etc.). LLM is
	// included here so the encoder emits [llm] and [llm.auth] subtables
	// in deterministic field order — no manual assembly needed.
	scalars := struct {
		Harness string   `toml:"harness,omitempty"`
		Profile string   `toml:"profile,omitempty"`
		Shell   string   `toml:"shell,omitempty"`
		With    []string `toml:"with,omitempty"`
		No      []string `toml:"no,omitempty"`
		Extensions []string `toml:"extensions,omitempty"`
		LLM     *LLMSpec `toml:"llm,omitempty"`
	}{
		Harness: p.Harness,
		Profile: p.Profile,
		Shell:   p.Shell,
		With:    p.With,
		No:      p.No,
		Extensions: p.Extensions,
		LLM:     p.LLM,
	}
	if err := toml.NewEncoder(&b).Encode(scalars); err != nil {
		return fmt.Errorf("encode pin scalars: %w", err)
	}

	// Prereqs subtables in sorted order.
	if len(p.Prereqs) > 0 {
		keys := sortedKeys(p.Prereqs)
		for _, prereqID := range keys {
			b.WriteString("\n[prereqs.")
			b.WriteString(prereqID)
			b.WriteString("]\n")
			innerKeys := sortedKeys(p.Prereqs[prereqID])
			for _, k := range innerKeys {
				fmt.Fprintf(&b, "%s = %q\n", k, p.Prereqs[prereqID][k])
			}
		}
	}

	// Env table in sorted order.
	if len(p.Env) > 0 {
		b.WriteString("\n[env]\n")
		for _, k := range sortedKeys(p.Env) {
			fmt.Fprintf(&b, "%s = %q\n", k, p.Env[k])
		}
	}

	// Integrations table in sorted order (id = "mode").
	if len(p.Integrations) > 0 {
		b.WriteString("\n[integrations]\n")
		for _, k := range sortedKeys(p.Integrations) {
			fmt.Fprintf(&b, "%s = %q\n", k, p.Integrations[k])
		}
	}

	// 0600 — the file may hold plaintext API keys after bootstrap.
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// FindPin walks from startDir upward, looking for a .vb file. Stops at the
// git repo root if one is in the chain, otherwise the filesystem root.
// Returns the path to the file, or "" + os.ErrNotExist if none found.
//
// Returning "" with an ErrNotExist error rather than just an empty string
// makes the API predictable: callers can use errors.Is(err, os.ErrNotExist).
func FindPin(startDir string) (string, error) {
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	gitRoot := detectGitRoot(abs) // may be "" if not in a repo

	dir := abs
	for {
		candidate := filepath.Join(dir, PinFileName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}

		// Stop at git root, if known.
		if gitRoot != "" && dir == gitRoot {
			break
		}
		// Stop at filesystem root.
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", os.ErrNotExist
}

// AppendToGitignore appends ".vb" to the workspace's .gitignore if and only if:
//   - .gitignore exists at workspaceDir (we don't create one — that would be
//     intrusive for projects that deliberately track everything)
//   - .vb isn't already covered by an exact-line match
//
// Idempotent. Does NOT stage or commit anything.
func AppendToGitignore(workspaceDir string) (changed bool, err error) {
	gi := filepath.Join(workspaceDir, ".gitignore")
	info, err := os.Stat(gi)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil // not an error, just nothing to do
		}
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf(".gitignore at %s is a directory", workspaceDir)
	}

	content, err := os.ReadFile(gi)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == ".vb" {
			return false, nil
		}
	}

	// Ensure we don't glue to the previous line.
	f, err := os.OpenFile(gi, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return false, err
	}
	defer f.Close()

	prefix := ""
	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		prefix = "\n"
	}
	if _, err := io.WriteString(f,
		prefix+"\n# vibrator workspace pin — may contain plaintext prereq tokens\n.vb\n",
	); err != nil {
		return false, err
	}
	return true, nil
}

// detectGitRoot returns the absolute path of the git repository containing
// dir, or "" if dir is not inside any git repo or git is unavailable. We
// shell out to `git rev-parse --show-toplevel` rather than walking for
// `.git` directories — that's the canonical way and handles git worktrees,
// submodules, and GIT_DIR overrides.
func detectGitRoot(dir string) string {
	if _, err := exec.LookPath("git"); err != nil {
		return ""
	}
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// sortedKeys returns the keys of m in lexicographic order. Generic over
// map value type. Pure helper for stable TOML emission.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
