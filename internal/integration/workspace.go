package integration

import "context"

// Workspace identifies a vibrator workspace (project directory) for the
// purposes of workspace-scoped credential bootstrap. Mirrors the
// prereq.Workspace type — kept separate so the integration package
// doesn't depend on internal/prereq.
type Workspace struct {
	// Path is the absolute path to the workspace root.
	Path string

	// ProjectName is the derived project name (typically the directory
	// basename, sometimes overridden by the user).
	ProjectName string

	// Hostname is the host machine name. Used in audit logs and as part
	// of the actor identifier for some integrations (e.g., claude-mem).
	Hostname string
}

// WorkspaceDriver mints and rotates per-workspace credentials for an
// integration. Credentials are stored as a flat map[string]string under
// [prereqs.<PrereqID>] in the workspace .vb file.
//
// Integrations with no per-workspace state leave Integration.Workspace
// nil rather than supplying a no-op driver.
type WorkspaceDriver interface {
	// PrereqID is the key used under [prereqs.<id>] in the workspace
	// .vb file (e.g., "claude-mem-server-beta").
	PrereqID() string

	// Bootstrap mints a fresh set of credentials for ws. Returned map
	// keys are integration-specific (e.g., {api_key, team_id,
	// project_id, actor_id} for claude-mem).
	Bootstrap(ctx context.Context, ws Workspace) (map[string]string, error)

	// Rotate revokes the current credentials and mints fresh ones.
	// `current` is the map currently persisted in the .vb file.
	Rotate(ctx context.Context, ws Workspace, current map[string]string) (map[string]string, error)
}
