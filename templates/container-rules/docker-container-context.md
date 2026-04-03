# Docker Container Environment Context

You are running inside a Docker container managed by vibrator.

## Container Limitations

### File System
- Your workspace is mounted from the host at `$WORKSPACE_PATH`
- Changes to files in the workspace persist on the host
- Files outside the workspace are ephemeral (lost on container restart)
- Do NOT modify system files or configuration outside your workspace

### Network
- Container uses host network mode by default
- You have full network access to host services
- Be cautious with localhost connections (they reach the host)

### Process Management
- Do NOT start long-running background services without user approval
- Use Docker-in-Docker mode (--dind) for container operations
- Avoid process operations that affect the host system

## Available Tools

### MCP Servers
- **Serena**: Semantic code analysis (LSP-based symbol search, references, rename)
- **Context7**: Library documentation lookup (resolve-library-id then query-docs)
- **Sequential Thinking**: Structured multi-step reasoning. Call the `sequentialthinking` tool
  to break down complex problems into steps before acting.
- **SQLite**: Query SQLite databases via natural language. Default: `/tmp/scratch.db`.
  To use a project database: `claude mcp add sqlite --scope=project -- mcp-server-sqlite --db-path=./mydb.sqlite`

### Data Processing
- **jq**: JSON processing. Example: `cat data.json | jq '.users[] | .name'`
- **yq**: YAML/TOML/XML processing. Example: `yq '.services' docker-compose.yml`
- **sqlite3**: SQLite CLI. Example: `sqlite3 data.db "SELECT * FROM users LIMIT 5"`
- **csvkit**: CSV tools suite.
  - `csvlook data.csv` — pretty-print CSV
  - `csvsql --query "SELECT col FROM data" data.csv` — SQL on CSV
  - `csvgrep -c name -m "pattern" data.csv` — filter rows
  - `csvstat data.csv` — column statistics

### API & Network Debugging
- **httpie**: HTTP client (better than curl for APIs).
  - `http GET api.example.com/users` — GET request
  - `http POST api.example.com/users name=John` — POST with JSON
  - `http --form POST api.example.com/upload file@data.csv` — file upload
- **websocat**: WebSocket CLI client.
  - `websocat ws://localhost:8080/ws` — connect to WebSocket
  - `echo '{"type":"ping"}' | websocat ws://localhost:8080/ws` — send message

### Code Quality
- **ruff**: Python linter + formatter (replaces flake8, isort, black).
  - `ruff check .` — lint Python files
  - `ruff format .` — format Python files
  - `ruff check --fix .` — auto-fix issues
- **hadolint**: Dockerfile linter.
  - `hadolint Dockerfile` — lint a Dockerfile
  - `hadolint --ignore DL3008 Dockerfile` — ignore specific rule

### Git Enhancement
- **delta**: Syntax-highlighted git diffs. Configure with:
  `git config --global core.pager delta`
- **lazygit**: Terminal UI for git. Launch: `lazygit`

### Search & Navigation
- **ripgrep (rg)**: Fast regex search. Example: `rg "TODO" --type=py`
- **fd**: Fast file finder. Example: `fd "\.py$" src/`
- **fzf**: Fuzzy finder. Example: `rg --files | fzf`
- **tree**: Directory tree. Example: `tree -L 2 src/`

### AI Pair Programming (opt-in with --aider flag)
- **aider**: Multi-model AI coding assistant. Only available when container built with `--aider`.
  - `aider --model claude-3.5-sonnet` — start with Claude
  - `aider --model gpt-4o` — start with GPT-4o
  - Uses ANTHROPIC_API_KEY / OPENAI_API_KEY (already forwarded by vibrator)

### Always Available (Core)
- Claude CLI with all MCP servers listed above
- Git, GitHub CLI (gh)
- Python 3, Go
- Standard Unix utilities (curl, wget, vim, htop, etc.)

### Conditionally Available
- Docker commands: Only available with `--dind` or `--docker` flag

## Security Context

### Default Mode (Secure)
- Running with minimal privileges
- Cannot execute Docker commands
- Cannot modify host system
- Safe for most development workflows

### Docker-in-Docker Mode (--dind)
- Elevated privileges enabled
- Full access to host Docker daemon
- Can build and run containers
- Use only when necessary

## Best Practices

1. **Respect the host system**
   - Don't attempt to escape the container
   - Don't install system-wide packages
   - Keep operations within your workspace

2. **Ask before dangerous operations**
   - System configuration changes
   - Installing global dependencies
   - Running containers (if not in --dind mode)

3. **Use appropriate tools**
   - Git for version control
   - Docker CLI for container operations (in --dind mode)
   - MCP servers for code analysis and documentation

## What NOT to do

- ❌ Don't attempt to install Docker if not in --dind mode
- ❌ Don't modify files in `/etc`, `/usr`, or other system directories
- ❌ Don't try to access host's root filesystem
- ❌ Don't create privileged processes
- ❌ Don't attempt container escape techniques
