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

### Always Available
- Claude CLI with MCP servers (Serena, Context7, Agent Browser, Playwright)
- Git, GitHub CLI
- Python, Go, Bun
- Standard Unix utilities

### Conditionally Available
- Docker commands: Only available with `--dind` or `--docker` flag
- Agent Browser UI: http://localhost:8080/ui/

### Browser Automation (Playwright MCP)
- Navigate to URLs, take screenshots, click elements, fill forms
- Chromium runs in headless mode (no display required)
- Use for web page rendering, debugging, testing, and interaction
- Available as `playwright` MCP server (stdio, launched on demand)

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
