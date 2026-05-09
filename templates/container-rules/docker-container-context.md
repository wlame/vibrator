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
- **Semgrep**: AI-accessible SAST. Tools: `security_check`, `scan`, `rule authoring`, `AST access`.
  Prefer this over the semgrep CLI for interactive analysis; use the CLI for scripted output.

### Autonomous Coding
- **ralphex**: Autonomous coding loop — executes implementation plans task-by-task in fresh sessions.
  - `ralphex run plan.md` — run a plan file
  - `ralphex run plan.md --dry-run` — preview tasks without executing
- **codex**: OpenAI Codex CLI for adversarial code review (used by the `/planning:exec` skill).
  Requires `OPENAI_API_KEY` forwarded from host.
  - `codex "review this function for security issues" < file.go`

### Security & Secrets Scanning
- **gitleaks**: Detect hardcoded secrets in git history and files.
  - `gitleaks detect --source .` — scan working tree
  - `gitleaks detect --source . --log-opts HEAD~5..HEAD` — scan last 5 commits
- **trufflehog**: Secret scanner with live credential validation.
  - `trufflehog filesystem .` — scan current directory
  - `trufflehog git file://. --since-commit HEAD~10` — scan recent commits
- **detect-secrets**: Baseline-based pre-commit secrets detector.
  - `detect-secrets scan > .secrets.baseline` — create baseline
  - `detect-secrets audit .secrets.baseline` — audit findings

### SAST & Static Analysis
- **semgrep**: Multi-language SAST with 5000+ rules (also available via MCP).
  - `semgrep scan --config=auto .` — auto-select rules for detected languages
  - `semgrep scan --config=p/owasp-top-ten .` — OWASP Top 10 ruleset
  - `semgrep scan --config=p/secrets .` — secrets ruleset
- **bandit**: Python-specific security linter.
  - `bandit -r src/` — scan Python source recursively
  - `bandit -r src/ -f json` — JSON output for scripting
- **golangci-lint**: Bundled Go linter (replaces gosec, staticcheck, ~40 others in one pass).
  - `golangci-lint run ./...` — run all configured linters
  - `golangci-lint run --enable=gosec ./...` — enable specific linter
- **gosec**: Go security checker (also included via golangci-lint).
  - `gosec ./...` — scan Go packages
- **staticcheck**: Go static analysis for correctness and performance.
  - `staticcheck ./...` — run all checks

### SBOM & CVE Scanning
- **trivy**: FS + container + IaC + secrets + SBOM in one binary.
  - `trivy fs .` — scan current directory
  - `trivy fs --scanners=secret .` — secrets only
  - `trivy fs --format=json .` — JSON output
- **syft**: Generate SBOMs in CycloneDX or SPDX format.
  - `syft . -o cyclonedx-json` — CycloneDX SBOM
  - `syft . -o spdx-json` — SPDX SBOM
- **grype**: CVE scan against a directory or SBOM (pairs with syft).
  - `grype .` — scan current directory
  - `grype sbom:sbom.json` — scan from a syft SBOM
- **osv-scanner**: Multi-ecosystem lockfile CVE scan against OSV.dev.
  - `osv-scanner .` — scan all lockfiles in current directory
  - `osv-scanner --format=json .` — JSON output
- **pip-audit**: Python dependency vulnerability scanner.
  - `pip-audit` — scan current environment
  - `pip-audit -r requirements.txt` — scan a requirements file

### IaC & Container Linting
- **checkov**: Terraform / CloudFormation / Kubernetes / Dockerfile IaC scanner.
  - `checkov -d .` — scan all IaC in directory
  - `checkov -f Dockerfile` — scan a specific file
  - `checkov --framework dockerfile -d .` — target framework
- **dockle**: Container image best-practices linter (complements hadolint for built images).
  - `dockle image-name:tag` — lint a built image (requires --dind)
- **shellcheck**: Shell script linter.
  - `shellcheck script.sh` — lint a shell script
  - `shellcheck -S warning script.sh` — warnings and above only
- **hadolint**: Dockerfile linter (static, no build needed).
  - `hadolint Dockerfile` — lint a Dockerfile
  - `hadolint --ignore DL3008 Dockerfile` — ignore specific rule

### Code Complexity
- **scc**: Fast LOC counter + cyclomatic complexity + COCOMO cost estimates.
  - `scc .` — count lines and complexity for all files
  - `scc --by-file .` — per-file breakdown
  - `scc --format=json .` — JSON output
- **lizard**: Language-agnostic cyclomatic complexity analyser.
  - `lizard src/` — analyse source directory
  - `lizard -l python src/` — Python only
  - `lizard --CCN 10 src/` — flag functions above CCN 10

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
- Python 3.13, Go 1.26, Node.js
- **uv**: Python package manager (`uv pip install`, `uv tool install`, `uv run`)
- Standard Unix utilities (curl, wget, vim, htop, etc.)

### Conditionally Available
- Docker commands: Only available with `--dind` or `--docker` flag

### Production Audit Prompt
A comprehensive multi-phase production-readiness audit prompt is pre-installed at
`/opt/audit/production-audit-prompt.md`. Copy it into a target repo or paste its contents
into Claude Code to run a full audit using the tools above.

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
