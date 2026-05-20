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
	// Harness identifies the target harness ("claudecode", "codex",
	// "opencode", "pi"). Use "*" to apply to all harnesses that speak
	// the same protocol.
	Harness string

	// MCP, when non-nil, declares an MCP server entry to add to the
	// harness's configuration. The container-side runtime (today the
	// claude-exec.sh probe loop; eventually a registry-driven loop for
	// every harness) is responsible for materializing this into the
	// harness's actual config format.
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
// Transport switching (e.g., stdio → http when a host server becomes
// reachable) is the container runtime's responsibility, not ours.
// Wiring just declares the canonical, fully-configured form.
type MCPWiring struct {
	// Name is the entry key in the harness's mcpServers map.
	Name string

	// Transport is "http", "sse", or "stdio".
	Transport string

	// URL is used for the http and sse transports.
	URL string

	// Headers are sent on http / sse requests (e.g., Authorization).
	Headers map[string]string

	// Command and Args specify the stdio transport child process.
	Command []string
	Args    []string

	// Env sets environment variables for the stdio child process.
	Env map[string]string
}
