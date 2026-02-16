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
- Claude CLI with MCP servers (Serena, Context7, Playwright)
- Git, GitHub CLI
- Python, Go, Bun
- Standard Unix utilities

### Conditionally Available
- Docker commands: Only available with `--dind` or `--docker` flag
- agent-browser CLI: Token-efficient browser automation (Vercel, installed as Claude skill)
- MCP Hub UI: http://localhost:8080/ui/ (only with --mcp flag)

### Browser Automation (Playwright + Chrome)

Chrome and the Playwright library are pre-installed in this container.
A wrapper script at `/opt/google/chrome/chrome` automatically injects `--no-sandbox`
and other container-safe flags, so Chrome works without elevated privileges.

#### Option 1: Playwright MCP Tools (Preferred)

The Playwright MCP server is pre-configured. Use its tools directly:
- `browser_navigate` — open a URL
- `browser_snapshot` — get accessibility tree (better than screenshots for actions)
- `browser_take_screenshot` — capture PNG screenshots
- `browser_click`, `browser_type`, `browser_hover` — interact with elements
- `browser_evaluate` — run JavaScript on the page
- `browser_console_messages` — read console output

To test local files, start an HTTP server first:
```bash
python3 -m http.server 8765 &
```
Then use `browser_navigate` to `http://localhost:8765/`.

#### Option 2: Playwright Scripts (Advanced)

For complex automation (multi-viewport testing, batch screenshots, data extraction),
write a Node.js script. The `playwright-core` library is available globally via `NODE_PATH`:

```javascript
const { chromium } = require('playwright-core');

(async () => {
    const browser = await chromium.launch({
        executablePath: '/opt/google/chrome/chrome',
        headless: true
        // No need to pass --no-sandbox — the wrapper handles it
    });

    const page = await browser.newPage();
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto('http://localhost:8765/', { waitUntil: 'networkidle' });
    await page.screenshot({ path: '/tmp/screenshot.png', fullPage: true });
    await browser.close();
})();
```

Run with: `node /tmp/pw-script.js`

View screenshots with the Read tool on the PNG files.

#### Key Paths

| Resource | Path |
|----------|------|
| Chrome wrapper | `/opt/google/chrome/chrome` (auto-adds `--no-sandbox`) |
| Chrome binary | `/opt/google/chrome/chrome.real` |
| Playwright library | Available via `require('playwright-core')` (NODE_PATH set) |
| Node.js / Bun | `/usr/local/bin/node`, `/usr/local/bin/bun` |

#### Common Recipes

**Multi-viewport responsive testing:**
```javascript
for (const vp of [{w:1440,h:900,name:'desktop'}, {w:768,h:1024,name:'tablet'}, {w:375,h:812,name:'mobile'}]) {
    await page.setViewportSize({ width: vp.w, height: vp.h });
    await page.goto(url, { waitUntil: 'networkidle' });
    await page.waitForTimeout(2000);
    await page.screenshot({ path: `/tmp/${vp.name}.png`, fullPage: true });
}
```

**CSS debugging — extract computed styles:**
```javascript
const styles = await page.evaluate(() => {
    const el = document.querySelector('.my-element');
    const cs = getComputedStyle(el);
    return { display: cs.display, grid: cs.gridTemplateColumns, color: cs.color };
});
```

**JS debugging — check console errors:**
```javascript
page.on('console', msg => console.log(`[${msg.type()}] ${msg.text()}`));
page.on('pageerror', err => console.log(`[PAGE ERROR] ${err.message}`));
await page.goto(url);
```

**Test hover/click interactions:**
```javascript
await page.hover('.feature-card');
await page.waitForTimeout(500);
await page.screenshot({ path: '/tmp/hover-state.png' });
```

**Network request monitoring:**
```javascript
page.on('request', req => console.log(`>> ${req.method()} ${req.url()}`));
page.on('response', res => console.log(`<< ${res.status()} ${res.url()}`));
```

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
