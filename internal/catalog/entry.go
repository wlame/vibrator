// Package catalog loads the curated per-harness inventory of plugins, MCP
// servers, skills, subagents, and tools from a markdown-with-YAML-frontmatter
// representation.
//
// Each entry is one file under catalog/<harness>/<id>.md. The frontmatter
// (between the first two `---` lines) supplies metadata; the markdown body
// is the user-facing prose docs (host setup, verification, troubleshooting).
//
// Loaders accept an fs.FS so unit tests can run against fstest.MapFS, and
// production code can hand in an embed.FS rooted at the module's catalog/.
package catalog

// Kind classifies what a catalog entry installs. The five kinds are the
// concepts most users will be familiar with from Claude Code's terminology;
// other harnesses are mapped onto the same set even when their native
// terminology differs (Codex calls these "plugins", OpenCode calls them
// "agents", Pi calls them "extensions" — they all fit one of the kinds below).
type Kind string

const (
	// KindPlugin is a complete pluggable package — typically bundles a
	// command, skill, hook set, or MCP server under a single install entry.
	KindPlugin Kind = "plugin"

	// KindSkill is a slash-command-triggered specialised behaviour.
	KindSkill Kind = "skill"

	// KindMCP is a Model Context Protocol server providing extra tools / resources.
	KindMCP Kind = "mcp"

	// KindSubagent is a dispatchable specialised agent.
	KindSubagent Kind = "subagent"

	// KindTool is a CLI / language-runtime tool that the harness optionally
	// uses — used when a catalog entry is really "install this CLI" rather
	// than a harness-native concept.
	KindTool Kind = "tool"
)

// AllKinds is the canonical iteration order — used in CLI output and the
// wizard's grouped display.
var AllKinds = []Kind{KindPlugin, KindSkill, KindMCP, KindSubagent, KindTool}

// Valid reports whether k is one of the known kinds. Frontmatter parsing
// rejects any other value early.
func (k Kind) Valid() bool {
	switch k {
	case KindPlugin, KindSkill, KindMCP, KindSubagent, KindTool:
		return true
	}
	return false
}

// Deps describes what an entry needs from elsewhere in the system.
type Deps struct {
	// Features is a list of internal/feature IDs the entry needs in the
	// image (e.g., "node" so the MCP server's npx can find a runtime).
	Features []string `yaml:"features,omitempty"`

	// Catalog is a list of other entry IDs (within the same harness) the
	// entry needs. Used sparingly — most entries should be independent.
	Catalog []string `yaml:"catalog,omitempty"`
}

// AuthSpec describes how a tool authenticates against an external service.
// Used by the wizard to surface which env vars need to be set on the host.
type AuthSpec struct {
	// Env names the environment variable carrying the credential. Vibrator
	// will forward this var (and treat it as required if the entry is enabled).
	Env string `yaml:"env"`
}

// Entry is one fully-loaded catalog item. Frontmatter fields populate the
// metadata; the markdown body (everything after the second `---`) lands in
// Body verbatim.
//
// `yaml:` tags drive frontmatter parsing. Fields without tags (Harness, ID,
// Body) are populated by the loader from the file path or post-split content.
type Entry struct {
	// Harness is the name of the directory the file lives in (e.g.,
	// "claude-code"). Populated by the loader, not by frontmatter.
	Harness string `yaml:"-"`

	// ID is the basename of the file, sans ".md". Populated by the loader.
	// If the frontmatter also carries an `id:` field, the two must match —
	// the loader returns an error otherwise.
	ID string `yaml:"id,omitempty"`

	// Name is the display label used in the wizard and `catalog list`.
	Name string `yaml:"name"`

	// Kind is one of: plugin, skill, mcp, subagent, tool. Required.
	Kind Kind `yaml:"kind"`

	// Default reports whether the wizard should pre-check this entry.
	// Use for entries that are essentially "you almost certainly want this".
	Default bool `yaml:"default,omitempty"`

	// SizeMB is the approximate image-size impact when this entry is enabled.
	// Best-effort, informational only.
	SizeMB int `yaml:"size_mb,omitempty"`

	// Deps lists features + other catalog entries this one needs.
	Deps Deps `yaml:"deps,omitempty"`

	// Prereq is the ID of a prereq from internal/prereq (Phase 4) that must
	// be satisfied before launch. Empty = no prereq.
	Prereq string `yaml:"prereq,omitempty"`

	// Install is the shell snippet that installs the entry into the image at
	// Docker build time. Run by the Dockerfile generator (Phase 3) as a
	// single RUN step. May be multi-line; use YAML block scalars.
	Install string `yaml:"install,omitempty"`

	// Auth, if set, declares which env var carries the credential.
	Auth *AuthSpec `yaml:"auth,omitempty"`

	// Source is the upstream URL for this entry — a GitHub repo, npm
	// package, or docs page. Required for traceability.
	Source string `yaml:"source"`

	// HostAliases is the list of identifiers the host-side detection
	// (internal/hostprobe) may emit for this entry. Used when the host
	// stores the plugin under a different name from our catalog ID — e.g.
	// host has "playwright", our catalog has "playwright-mcp". Lookups
	// match against both ID and HostAliases. Lowercase, exact match.
	HostAliases []string `yaml:"host_aliases,omitempty"`

	// Body is the markdown content after the frontmatter. Populated by the
	// loader; never set in frontmatter.
	Body string `yaml:"-"`
}

// Key returns the canonical "<harness>/<id>" identifier used as map key in
// LoadAll's return value.
func (e Entry) Key() string {
	return e.Harness + "/" + e.ID
}
