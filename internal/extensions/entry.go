// Package extensions loads the curated per-harness inventory of plugins, MCP
// servers, skills, subagents, and tools from a markdown-with-YAML-frontmatter
// representation.
//
// Each entry is one file under extensions/<harness>/<id>.md. The frontmatter
// (between the first two `---` lines) supplies metadata; the markdown body
// is the user-facing prose docs (host setup, verification, troubleshooting).
//
// Loaders accept an fs.FS so unit tests can run against fstest.MapFS, and
// production code can hand in an embed.FS rooted at the module's extensions/.
package extensions

// Kind classifies what an extension installs. The five kinds are the
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
	// uses — used when an extension is really "install this CLI" rather
	// than a harness-native concept.
	KindTool Kind = "tool"
)

// AllKinds is the canonical iteration order — used in CLI output and the
// wizard's grouped display.
var AllKinds = []Kind{KindPlugin, KindSkill, KindMCP, KindSubagent, KindTool}

// Category groups extension entries by semantic purpose, orthogonally to
// Kind. The wizard can group entries by Kind (what they are technically)
// AND by Category (what they help with). A Category is freeform string —
// no fixed taxonomy is enforced by the loader — but we provide a canonical
// list below so the wizard renders predictable order.
//
// Adding a new value: add a constant + entry in AllCategories. Loader
// won't reject unknown values, so contributors can experiment.
type Category string

const (
	CategoryCodeIntel        Category = "code-intelligence"
	CategoryMemory           Category = "memory"
	CategoryDocs             Category = "documentation"
	CategoryWebBrowser       Category = "web-browser"
	CategoryVCS              Category = "version-control"
	CategoryProjectMgmt      Category = "project-management"
	CategoryCommunication    Category = "communication"
	CategoryCloudInfra       Category = "cloud-infrastructure"
	CategoryDatabases        Category = "databases"
	CategoryDesignUI         Category = "design-ui"
	CategoryTesting          Category = "testing"
	CategorySecurity         Category = "security"
	CategoryAIIntegration    Category = "ai-integration"
	CategoryDevTools         Category = "dev-tools"
	CategoryObservability    Category = "observability"
	CategoryHarnessSpecific  Category = "harness-specific"
	CategoryNiche            Category = "niche"
)

// AllCategories is the canonical iteration order, mirroring the structure
// of the research catalogues in .claude/research/harnesses/.
var AllCategories = []Category{
	CategoryCodeIntel,
	CategoryMemory,
	CategoryDocs,
	CategoryWebBrowser,
	CategoryVCS,
	CategoryProjectMgmt,
	CategoryCommunication,
	CategoryCloudInfra,
	CategoryDatabases,
	CategoryDesignUI,
	CategoryTesting,
	CategorySecurity,
	CategoryAIIntegration,
	CategoryDevTools,
	CategoryObservability,
	CategoryHarnessSpecific,
	CategoryNiche,
}

// CategoryLabel returns the user-facing display name for a Category.
// Falls through to the raw value for unrecognized categories so the
// wizard still renders something when contributors invent new ones.
func CategoryLabel(c Category) string {
	switch c {
	case CategoryCodeIntel:
		return "Code Intelligence"
	case CategoryMemory:
		return "Memory & Context"
	case CategoryDocs:
		return "Documentation"
	case CategoryWebBrowser:
		return "Web & Browser"
	case CategoryVCS:
		return "Version Control"
	case CategoryProjectMgmt:
		return "Project Management"
	case CategoryCommunication:
		return "Communication"
	case CategoryCloudInfra:
		return "Cloud & Infrastructure"
	case CategoryDatabases:
		return "Databases"
	case CategoryDesignUI:
		return "Design & UI"
	case CategoryTesting:
		return "Testing & QA"
	case CategorySecurity:
		return "Security"
	case CategoryAIIntegration:
		return "AI/LLM Integration"
	case CategoryDevTools:
		return "Developer Tools"
	case CategoryObservability:
		return "Observability"
	case CategoryHarnessSpecific:
		return "Harness-specific"
	case CategoryNiche:
		return "Niche / Specialized"
	}
	if c == "" {
		return "Uncategorized"
	}
	return string(c)
}

// RuntimeNeeds describes what an extension needs at runtime, beyond the
// build-time install. Used by the wizard to surface security/operational
// constraints before the user commits to enabling something.
//
// All fields default to safe values (false / empty), so omitting this
// block in frontmatter means "purely local, no special needs".
type RuntimeNeeds struct {
	// LocalOnly is true when the extension works entirely offline once
	// installed (no outbound network at runtime, no host services).
	// Flips the wizard badge to "[local]" — the safest signal.
	LocalOnly bool `yaml:"local_only,omitempty"`

	// SelfHosted, when non-empty, names a host-side service the extension
	// depends on (e.g., "serena-host" or "claude-mem-stack"). The wizard
	// uses this to point at the corresponding `vibrate integrations`
	// flow when the user picks the entry.
	SelfHosted string `yaml:"self_hosted,omitempty"`

	// ThirdPartyAPI, when non-empty, names the third-party service the
	// extension calls (e.g., "GitHub", "Linear", "OpenAI"). Forms the
	// "[needs $service token]" badge in the wizard.
	ThirdPartyAPI string `yaml:"third_party_api,omitempty"`

	// OutboundNet is true when the extension makes outbound network
	// calls at runtime to something other than the listed
	// ThirdPartyAPI (e.g., model providers via OpenAI-compatible
	// endpoints, generic web search backends).
	OutboundNet bool `yaml:"outbound_net,omitempty"`
}

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

	// Extensions is a list of other entry IDs (within the same harness) the
	// entry needs. Used sparingly — most entries should be independent.
	Extensions []string `yaml:"extensions,omitempty"`
}

// AuthSpec describes how a tool authenticates against an external service.
// Used by the wizard to surface which env vars need to be set on the host.
type AuthSpec struct {
	// Env names the environment variable carrying the credential. Vibrator
	// will forward this var (and treat it as required if the entry is enabled).
	Env string `yaml:"env"`
}

// Entry is one fully-loaded extensions item. Frontmatter fields populate the
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

	// Name is the display label used in the wizard and `extensions list`.
	Name string `yaml:"name"`

	// Description is an optional one-line summary shown in the wizard
	// next to the entry's name — answers "what does this do" without
	// requiring `vibrate extensions show <id>`. Keep it under ~80
	// chars (huh's MultiSelect renders one option per row; long
	// descriptions truncate visually).
	//
	// When omitted, the wizard falls back to category + runtime
	// badges as the only contextual hint.
	Description string `yaml:"description,omitempty"`

	// Kind is one of: plugin, skill, mcp, subagent, tool. Required.
	Kind Kind `yaml:"kind"`

	// Default reports whether the wizard should pre-check this entry.
	// Use for entries that are essentially "you almost certainly want this".
	Default bool `yaml:"default,omitempty"`

	// SizeMB is the approximate image-size impact when this entry is enabled.
	// Best-effort, informational only.
	SizeMB int `yaml:"size_mb,omitempty"`

	// Deps lists features + other extension entries this one needs.
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
	// stores the plugin under a different name from our extensions ID — e.g.
	// host has "playwright", our extensions has "playwright-mcp". Lookups
	// match against both ID and HostAliases. Lowercase, exact match.
	HostAliases []string `yaml:"host_aliases,omitempty"`

	// Category groups this entry semantically for wizard display. See
	// the Category constants for the canonical set; freeform strings are
	// accepted (the wizard falls through to raw value as the label).
	Category Category `yaml:"category,omitempty"`

	// RuntimeNeeds describes what the extension needs at runtime
	// (vs. install time). Used by the wizard to show security/operational
	// badges next to each option.
	RuntimeNeeds RuntimeNeeds `yaml:"runtime_needs,omitempty"`

	// Body is the markdown content after the frontmatter. Populated by the
	// loader; never set in frontmatter.
	Body string `yaml:"-"`
}

// Key returns the canonical "<harness>/<id>" identifier used as map key in
// LoadAll's return value.
func (e Entry) Key() string {
	return e.Harness + "/" + e.ID
}
