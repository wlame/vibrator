// Package prereq describes host-side prerequisites that catalog entries
// can depend on, and provides probes ("verifiers") plus optional auto-fixers
// ("bootstrappers") to satisfy them.
//
// The canonical example is `claude-mem-server-beta`: the `claude-mem` catalog
// entry only works if a claude-mem server is running on the host AND the
// workspace has a project-scoped API key cached in `.vb`. The verifier checks
// reachability + presence of the cached key; the bootstrapper mints a fresh
// key against the host postgres (via a one-shot docker container) and returns
// the values to persist into the workspace pin.
//
// # Two-phase UX
//
// Vibrator probes prereqs at TWO distinct moments:
//
//  1. **Inside the wizard** — informational. If a catalog entry's prereq fails
//     to verify, we print an inline ⚠ with a doc-anchor link, but the user can
//     proceed (they may be setting up the host server in parallel).
//
//  2. **At launch time** — strict. Just before `docker run`/`exec`, we re-probe
//     every prereq of every installed catalog entry. On failure we hard-stop
//     with an actionable error (and the same doc anchor). This is the gate
//     that prevents the user from entering a container that's missing
//     required host wiring.
//
// # Design choices
//
//   - Verifier and Bootstrapper are tiny interfaces so a future prereq can be
//     added by implementing them and calling Register() from an init() — no
//     core changes needed.
//
//   - Bootstrapper returns the values to persist (rather than persisting
//     directly) so the caller controls where they land. This keeps unit tests
//     free of filesystem mocks.
//
//   - Sensitive material (e.g., a freshly-minted API key) is returned as a
//     map[string]string keyed by short field names (`api_key`, `team_id`,
//     `project_id`). The schema is loose by design — each prereq decides its
//     own field set.
package prereq

import "context"

// Prereq is a host-side dependency that one or more catalog entries can
// require. Catalog entries reference a Prereq by ID via the `prereq:` field
// in their YAML frontmatter.
type Prereq struct {
	// ID is the stable identifier referenced from catalog entries and from
	// `vibrate prereqs bootstrap <id>`. Lowercase kebab-case.
	ID string

	// Name is the human-readable display name shown in the wizard and in
	// status output.
	Name string

	// SetupDoc points at a section in the catalog markdown that documents
	// the manual host-side setup steps. Format: "<harness>/<id>.md#anchor".
	// Used as a fallback when Bootstrapper is nil OR when Bootstrap fails.
	SetupDoc string

	// Verifier probes whether this prereq is satisfied. Required.
	Verifier Verifier

	// Bootstrapper optionally automates the setup. nil = manual-only.
	// Returns a key/value map suitable for storing under
	// pin.Prereqs[id] = result.
	Bootstrapper Bootstrapper
}

// Verifier probes a single prereq. Verify is allowed to take a few seconds
// (we'll add per-implementation timeouts) and MUST NOT mutate host state.
type Verifier interface {
	Verify(ctx context.Context) Result
}

// Bootstrapper attempts to make a prereq pass. Bootstrap MAY mutate host
// state (mint a key, write a config row, pull a docker image). It returns
// the values the caller should persist into the workspace pin file under
// pin.Prereqs[id] = result.
//
// Workspace identifies which workspace the bootstrap is for — used by the
// claude-mem flow to scope a token to the project basename and to derive a
// unique actor_id.
type Bootstrapper interface {
	Bootstrap(ctx context.Context, ws Workspace) (map[string]string, error)
}

// Workspace bundles the per-workspace identifiers that a bootstrapper might
// care about. Filled in by the caller before Bootstrap is invoked.
type Workspace struct {
	// Path is the absolute path to the project root (typically `$PWD` of the
	// `vibrate` invocation, walked to the git root if applicable).
	Path string

	// ProjectName is the basename of Path. Used by claude-mem as the
	// project name in the (team, project) tuple.
	ProjectName string

	// Hostname is the host machine name (output of `hostname` or
	// os.Hostname). Used to build a unique actor_id so multiple machines
	// minting keys for the same project don't collide.
	Hostname string
}

// Result is the outcome of a Verifier probe.
//
// We deliberately don't make a probe failure an `error` — most callers want
// to keep going (e.g., to probe the next prereq, or to print all results
// before deciding whether to abort). Result.OK is the gate.
type Result struct {
	// OK is true when the prereq is satisfied. When false, Message describes
	// the failure in one line and Hint (optional) gives a short actionable
	// next step.
	OK bool

	// Message is a one-line summary (good or bad). Always populated.
	// Examples: "reachable at http://...", "connection refused".
	Message string

	// Hint is an optional follow-up suggesting the next step on failure.
	// Examples: "start claude-mem server-beta on the host (see docs/...)".
	// Empty on success.
	Hint string
}

// Registry is the global lookup of built-in prereqs by ID. Populated by
// Register() calls from each prereq's source file. Lookups via ByID().
var Registry = map[string]*Prereq{}

// Register adds a prereq to the global registry. Called from init() in each
// prereq's source file. Panics on duplicate ID — duplicates indicate a
// programming bug (two files claiming the same prereq).
func Register(p *Prereq) {
	if p == nil || p.ID == "" {
		panic("prereq.Register: nil or empty-ID prereq")
	}
	if _, dup := Registry[p.ID]; dup {
		panic("prereq.Register: duplicate id " + p.ID)
	}
	Registry[p.ID] = p
}

// ByID returns the prereq with the given ID, or (nil, false) if not found.
// Mirrors the lookup pattern in internal/feature and internal/harness.
func ByID(id string) (*Prereq, bool) {
	p, ok := Registry[id]
	return p, ok
}

// IDs returns the registered prereq IDs in stable (insertion) order. Used by
// `vibrate prereqs status` to iterate predictably.
func IDs() []string {
	ids := make([]string, 0, len(Registry))
	for id := range Registry {
		ids = append(ids, id)
	}
	return ids
}
