# Vibrate.sh Website

Single-page marketing website for [vibrator](https://github.com/wlame/vibrator) - Claude Code in Docker, auto-configured.

## Quick Deploy

### Prerequisites

- VPS with Docker and Docker Compose installed
- Domain `vibrate.sh` pointed to your VPS IP
- Port 80 and 443 open

### Setup

1. **Copy files to VPS:**

```bash
# On your local machine
scp -r web/* user@your-vps:/opt/vibrate-web/

# Or use rsync
rsync -avz web/ user@your-vps:/opt/vibrate-web/
```

2. **Configure Traefik:**

```bash
# SSH into VPS
ssh user@your-vps

# Navigate to directory
cd /opt/vibrate-web

# Edit Traefik config - CHANGE YOUR EMAIL!
nano traefik/traefik.yml
# Update: email: your-email@example.com
```

3. **Set permissions:**

```bash
chmod 600 traefik/acme.json
```

4. **Start services:**

```bash
docker compose up -d
```

5. **Check logs:**

```bash
# View all logs
docker compose logs -f

# View Traefik logs only
docker compose logs -f traefik

# View web logs only
docker compose logs -f web
```

6. **Verify:**

Visit https://vibrate.sh - you should see the site with automatic HTTPS!

## Configuration

### Domain Configuration

The default configuration expects:
- Main site: `vibrate.sh`
- Traefik dashboard: `traefik.vibrate.sh` (optional)

To change domains, edit `docker-compose.yml` and update the `Host()` rules.

### HTTPS/SSL

Let's Encrypt certificates are automatically obtained and renewed via HTTP challenge.

**Important:**
- Traefik will automatically redirect HTTP to HTTPS
- Certificates are stored in `traefik/acme.json`
- Renewal happens automatically

### Testing vs Production

By default, Traefik uses Let's Encrypt **production** server.

To test with staging server (recommended first):

Edit `traefik/traefik.yml`:
```yaml
certificatesResolvers:
  cloudflare:
    acme:
      # Uncomment for staging/testing:
      caServer: https://acme-staging-v02.api.letsencrypt.org/directory
```

**Note:** Staging certificates will show as untrusted in browsers, but verify the flow works.

## File Structure

```
web/
├── docker-compose.yml          # Main orchestration
├── traefik/
│   ├── traefik.yml            # Traefik config (EDIT EMAIL!)
│   └── acme.json              # SSL certificates (auto-generated)
├── static/
│   ├── index.html             # Main page
│   ├── css/
│   │   └── style.css          # Styles
│   └── js/
│       └── main.js            # Copy functionality
└── README.md                  # This file
```

## Customization

### Updating Content

Edit `static/index.html` to change:
- Hero text and tagline
- Feature descriptions
- Usage examples
- GitHub links

Edit `static/css/style.css` to change:
- Colors (see `:root` variables)
- Fonts
- Layouts
- Animations

### Adding Analytics

Add your analytics snippet to `static/index.html` before `</body>`:

```html
<!-- Google Analytics, Plausible, etc. -->
<script async src="..."></script>
```

### Custom 404 Page

Add `static/404.html` and configure Nginx in docker-compose.yml:

```yaml
web:
  image: nginx:alpine
  volumes:
    - ./static:/usr/share/nginx/html:ro
    - ./nginx.conf:/etc/nginx/nginx.conf:ro  # Add custom config
```

## Maintenance

### View Status

```bash
docker compose ps
```

### Restart Services

```bash
docker compose restart
```

### Update Website Content

```bash
# Edit files locally
nano static/index.html

# Upload changes
rsync -avz static/ user@your-vps:/opt/vibrate-web/static/

# Or if already on VPS, just restart
docker compose restart web
```

### Update Traefik

```bash
docker compose pull traefik
docker compose up -d traefik
```

### Backup SSL Certificates

```bash
# Copy acme.json to safe location
cp traefik/acme.json traefik/acme.json.backup

# Or download from VPS
scp user@your-vps:/opt/vibrate-web/traefik/acme.json ./acme.json.backup
```

### View Logs

```bash
# All logs
docker compose logs -f

# Last 100 lines
docker compose logs --tail=100

# Specific service
docker compose logs -f traefik
docker compose logs -f web
```

## Troubleshooting

### Site not loading

1. Check containers are running:
   ```bash
   docker compose ps
   ```

2. Check logs for errors:
   ```bash
   docker compose logs
   ```

3. Verify DNS points to VPS:
   ```bash
   dig vibrate.sh
   nslookup vibrate.sh
   ```

4. Check ports are open:
   ```bash
   netstat -tlnp | grep -E ':(80|443)'
   ```

### HTTPS not working

1. Check Traefik logs:
   ```bash
   docker compose logs traefik | grep -i error
   ```

2. Verify email is set in `traefik/traefik.yml`

3. Check acme.json permissions:
   ```bash
   ls -la traefik/acme.json
   # Should be: -rw------- (600)
   ```

4. Verify Let's Encrypt rate limits:
   - 50 certificates per domain per week
   - Use staging first if testing

5. Check domain propagation:
   ```bash
   curl -I http://vibrate.sh
   # Should redirect to https://
   ```

### Certificate renewal issues

Traefik automatically renews certificates. If issues occur:

1. Delete old certificate:
   ```bash
   # Backup first!
   cp traefik/acme.json traefik/acme.json.backup
   # Clear file
   echo '{}' > traefik/acme.json
   chmod 600 traefik/acme.json
   ```

2. Restart Traefik:
   ```bash
   docker compose restart traefik
   ```

### Container won't start

1. Check for port conflicts:
   ```bash
   sudo lsof -i :80
   sudo lsof -i :443
   ```

2. Stop conflicting services:
   ```bash
   sudo systemctl stop nginx  # If running
   sudo systemctl stop apache2  # If running
   ```

## Security

### Firewall

Only allow ports 80, 443, and SSH:

```bash
# UFW
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable

# iptables
sudo iptables -A INPUT -p tcp --dport 22 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 80 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 443 -j ACCEPT
```

### Traefik Dashboard

The Traefik dashboard is exposed on `traefik.vibrate.sh` in the default config.

**To disable:** Remove all Traefik router labels from docker-compose.yml

**To secure:** Add basic auth:
```bash
# Generate password
htpasswd -nb admin your-password

# Add to docker-compose.yml traefik labels:
- "traefik.http.routers.traefik-secure.middlewares=auth"
- "traefik.http.middlewares.auth.basicauth.users=admin:$$apr1$$..."
```

### Updates

Keep Docker and system updated:
```bash
sudo apt update && sudo apt upgrade -y
docker compose pull
docker compose up -d
```

## Performance

### Enable Gzip

Create `nginx.conf`:

```nginx
gzip on;
gzip_types text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript;
gzip_min_length 1000;
```

Mount in docker-compose.yml:
```yaml
volumes:
  - ./nginx.conf:/etc/nginx/conf.d/gzip.conf:ro
```

### Enable Caching

Add to nginx config:
```nginx
location ~* \.(css|js|jpg|jpeg|png|gif|ico|svg)$ {
    expires 1y;
    add_header Cache-Control "public, immutable";
}
```

## Production Checklist

- [ ] Update email in `traefik/traefik.yml`
- [ ] Verify DNS points to VPS
- [ ] Set acme.json permissions (600)
- [ ] Test with Let's Encrypt staging first
- [ ] Switch to production certificates
- [ ] Configure firewall
- [ ] Set up monitoring/alerts
- [ ] Add analytics (optional)
- [ ] Backup acme.json
- [ ] Test HTTPS redirect
- [ ] Verify copy buttons work
- [ ] Test on mobile devices

## License

MIT - Same as vibrator project
