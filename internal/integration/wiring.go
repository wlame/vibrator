package integration

// Wiring describes how a single harness inside the workspace container
// should consume an integration. An Integration may have multiple
// wirings — one per supported harness, or one shared entry with
// Harness == "*" for harnesses that share a protocol (e.g., all
// MCP-capable harnesses).
//
// Wiring is intentionally narrow: it covers MCP server entries and
// environment variables today. Anything richer (config-file templates,
// init-script hooks, build-time installs) should be added as
// additional optional fields rather than overloading these two.
type Wiring struct {
	// Harness identifies the target harness by its registry ID
	// ("claude-code", "codex", "opencode", "pi" — see internal/harness).
	// Use "*" to apply to all harnesses that speak the same protocol.
	Harness string

	// MCP, when non-nil, declares an MCP server entry to add to the
	// harness's configuration. The container-side runtime (see
	// templates/scripts/claude-exec.sh) is responsible for materializing
	// this into the harness's actual config format.
	MCP *MCPWiring

	// EnvVars are environment variables to set for the harness process.
	// Keys are variable names; values are literal string values. Use
	// for things like CLAUDE_MEM_RUNTIME=server-beta — anything the
	// harness reads from its environment.
	EnvVars map[string]string
}

// MCPWiring describes an MCP server entry as it should appear in a
// harness's configuration.
//
// Both HTTP and Stdio MAY be set:
//   - When only HTTP is set, the container writes the http entry
//     unconditionally.
//   - When only Stdio is set, the container writes the stdio entry
//     unconditionally.
//   - When BOTH are set, the container probes the HTTP URL on every
//     session entry and picks http when reachable, falling back to
//     stdio otherwise. This is the "host server with local fallback"
//     pattern used by Serena.
type MCPWiring struct {
	// Name is the entry key in the harness's mcpServers map.
	Name string

	// HTTP is the http transport spec (preferred when set).
	HTTP *MCPHTTP

	// Stdio is the stdio transport spec (fallback when HTTP is unset
	// or its URL is unreachable).
	Stdio *MCPStdio
}

// MCPHTTP describes an HTTP MCP endpoint.
type MCPHTTP struct {
	// URL is the http(s) endpoint. For a host-side server reachable
	// from inside a container, use http://host.docker.internal:<port>.
	URL string

	// Headers are sent on every request. Typically Authorization for
	// bearer-token-protected servers.
	Headers map[string]string
}

// MCPStdio describes a stdio MCP child process the harness spawns.
type MCPStdio struct {
	// Command is the executable to spawn. First element is the program
	// name; remaining elements are passed directly as args. (Most
	// integrations leave Args empty and put everything here for
	// readability.)
	Command []string

	// Args is appended to Command when the harness materializes the
	// invocation. Optional — useful when Command is a fixed prefix
	// and Args varies.
	Args []string

	// Env adds environment variables for the spawned child.
	Env map[string]string
}
