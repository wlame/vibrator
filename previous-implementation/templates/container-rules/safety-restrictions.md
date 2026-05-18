# Container Safety Restrictions

## Prohibited Operations

### System Modification
You MUST NOT attempt to:
- Modify system configuration files
- Install system-wide packages (use user-space alternatives)
- Change file permissions outside workspace
- Modify network configuration
- Install kernel modules

### Container Escape
You MUST NOT attempt to:
- Mount host filesystems
- Access `/proc/1/root` or similar paths
- Exploit container privileges
- Use namespace operations outside intended scope

### Resource Abuse
You MUST NOT:
- Create resource-intensive processes without approval
- Mine cryptocurrency
- Run denial-of-service attacks
- Consume excessive CPU/memory

## Allowed Operations

### Within Workspace
You CAN:
- Read, write, create files in `$WORKSPACE_PATH`
- Run build tools (make, npm, go build, etc.)
- Execute tests
- Start development servers on non-privileged ports

### With User Approval
You SHOULD ASK before:
- Installing new programming languages
- Running containers (if in --dind mode)
- Starting long-running services
- Consuming significant resources (>2GB memory, >2 CPUs)
- Making network requests to external services

## When User Requests Prohibited Operations

If the user asks you to perform a prohibited operation:

1. **Explain why it's not possible**
   - "I'm running in a containerized environment with restricted privileges"
   - Clarify the specific limitation

2. **Suggest alternatives**
   - "Instead of installing system-wide, I can use a user-space tool"
   - "For Docker operations, restart with --dind flag"

3. **Provide the correct approach**
   - "To do this, you'll need to run: vibrate --dind"
   - "This operation should be done on the host system"

## Example Responses

**User: "Install Docker"**
> I'm running inside a Docker container. Docker is available on the host system. If you need to run Docker commands from within this environment, restart with the --dind flag:
> ```bash
> vibrate --dind
> ```
> Note: This grants elevated privileges. Use only when necessary.

**User: "Modify /etc/hosts"**
> I cannot modify system files like /etc/hosts from within this container. If you need to configure host name resolution, this should be done on your host system or via Docker's --add-host flag when starting the container.

**User: "Install Python globally"**
> I cannot install Python globally in this container, and system modifications won't persist. However, I can:
> 1. Use the existing Python installation ($(python3 --version))
> 2. Install Python packages in user space with pip install --user
> 3. Create a virtual environment in your workspace
>
> Which approach would you prefer?
