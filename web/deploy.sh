#!/usr/bin/env bash
set -euo pipefail

# Deployment script for vibrate.sh website
# Usage: ./deploy.sh [user@host] [deploy-path]

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
REMOTE_HOST="${1:-}"
REMOTE_PATH="${2:-/opt/vibrate-web}"

# Functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_requirements() {
    if [ -z "$REMOTE_HOST" ]; then
        log_error "Remote host not specified!"
        echo ""
        echo "Usage: $0 user@your-vps [/path/to/deploy]"
        echo ""
        echo "Example:"
        echo "  $0 root@vibrate.sh /opt/vibrate-web"
        echo "  $0 ubuntu@203.0.113.1"
        exit 1
    fi

    if ! command -v rsync &> /dev/null; then
        log_error "rsync not found. Please install rsync."
        exit 1
    fi

    if ! command -v ssh &> /dev/null; then
        log_error "ssh not found. Please install openssh-client."
        exit 1
    fi
}

pre_deploy_checks() {
    log_info "Running pre-deployment checks..."

    # Check if email is configured in traefik.yml
    if grep -q "your-email@example.com" traefik/traefik.yml; then
        log_warn "Email not configured in traefik/traefik.yml!"
        log_warn "Please update the email address before deploying to production."
        read -p "Continue anyway? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi

    # Check if acme.json exists and has correct permissions
    if [ ! -f traefik/acme.json ]; then
        log_warn "traefik/acme.json not found. Creating..."
        touch traefik/acme.json
        chmod 600 traefik/acme.json
    else
        chmod 600 traefik/acme.json
    fi

    log_info "Pre-deployment checks completed."
}

deploy_files() {
    log_info "Deploying files to $REMOTE_HOST:$REMOTE_PATH..."

    # Create remote directory if it doesn't exist
    ssh "$REMOTE_HOST" "mkdir -p $REMOTE_PATH"

    # Sync files
    rsync -avz --delete \
        --exclude '.git' \
        --exclude '*.backup' \
        --exclude '*.log' \
        --exclude 'deploy.sh' \
        ./ "$REMOTE_HOST:$REMOTE_PATH/"

    log_info "Files deployed successfully."
}

remote_setup() {
    log_info "Running remote setup..."

    ssh "$REMOTE_HOST" << EOF
set -e

cd $REMOTE_PATH

# Set permissions
chmod 600 traefik/acme.json

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "Docker not found on remote host!"
    exit 1
fi

# Check if Docker-Compose is installed
if ! command -v docker-compose &> /dev/null && ! command -v docker-compose &> /dev/null; then
    echo "Docker-Compose not found on remote host!"
    exit 1
fi

echo "Remote setup completed."
EOF

    log_info "Remote setup completed."
}

start_services() {
    log_info "Starting services on remote host..."

    ssh "$REMOTE_HOST" << EOF
set -e
cd $REMOTE_PATH

# Start services
docker-compose up -d

# Show status
echo ""
echo "Services started:"
docker-compose ps

echo ""
echo "View logs with:"
echo "  docker-compose logs -f"
EOF

    log_info "Services started successfully."
}

show_completion_message() {
    echo ""
    log_info "Deployment completed!"
    echo ""
    echo "Next steps:"
    echo "  1. Verify DNS is pointing to your VPS"
    echo "  2. Check logs: ssh $REMOTE_HOST 'cd $REMOTE_PATH && docker-compose logs -f'"
    echo "  3. Visit https://vibrate.sh"
    echo ""
    echo "Useful commands:"
    echo "  • View status:  ssh $REMOTE_HOST 'cd $REMOTE_PATH && docker-compose ps'"
    echo "  • View logs:    ssh $REMOTE_HOST 'cd $REMOTE_PATH && docker-compose logs -f'"
    echo "  • Restart:      ssh $REMOTE_HOST 'cd $REMOTE_PATH && docker-compose restart'"
    echo "  • Stop:         ssh $REMOTE_HOST 'cd $REMOTE_PATH && docker-compose down'"
    echo ""
}

# Main execution
main() {
    log_info "Starting deployment to $REMOTE_HOST:$REMOTE_PATH"
    echo ""

    check_requirements
    pre_deploy_checks
    deploy_files
    remote_setup
    start_services
    show_completion_message
}

main
