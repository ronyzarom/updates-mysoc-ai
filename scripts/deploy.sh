#!/bin/bash
set -e

# =============================================================================
# MySoc Updates Platform - Deployment Script
# Target: updates.mysoc.ai (AWS Lightsail)
# =============================================================================

# Configuration
SERVER_USER="bitnami"
SERVER_IP="18.201.144.150"
SSH_KEY="/Users/ronyzaromil/Downloads/LightsailDefaultKey-eu-west-updates-mysoc.ai.pem"
REMOTE_DIR="/home/bitnami/updates-mysoc-ai"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# SSH command helper
ssh_cmd() {
    ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "$SERVER_USER@$SERVER_IP" "$@"
}

# SCP command helper
scp_cmd() {
    scp -i "$SSH_KEY" -o StrictHostKeyChecking=no "$@"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check SSH key exists
    if [ ! -f "$SSH_KEY" ]; then
        log_error "SSH key not found: $SSH_KEY"
    fi
    
    # Check Go is installed
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed"
    fi
    
    # Check SSH connectivity
    log_info "Testing SSH connection..."
    if ! ssh_cmd "echo 'Connection successful'" &> /dev/null; then
        log_error "Cannot connect to server. Check SSH key and network."
    fi
    
    log_success "Prerequisites check passed"
}

# Build binaries for Linux
build_binaries() {
    log_info "Building binaries for Linux (amd64)..."
    
    cd "$PROJECT_ROOT"
    
    # Build update-server
    log_info "Building update-server..."
    GOOS=linux GOARCH=amd64 go build -o bin/update-server-linux-amd64 ./cmd/update-server
    
    # Build mysoc-updater
    log_info "Building mysoc-updater..."
    GOOS=linux GOARCH=amd64 go build -o bin/mysoc-updater-linux-amd64 ./cmd/mysoc-updater
    
    log_success "Binaries built successfully"
    ls -lh bin/
}

# Prepare remote server
prepare_remote() {
    log_info "Preparing remote server..."
    
    ssh_cmd "mkdir -p $REMOTE_DIR/{bin,migrations,config,data/releases,logs}"
    
    log_success "Remote directories created"
}

# Deploy binaries
deploy_binaries() {
    log_info "Deploying binaries to server..."
    
    # Upload binaries
    scp_cmd "$PROJECT_ROOT/bin/update-server-linux-amd64" "$SERVER_USER@$SERVER_IP:$REMOTE_DIR/bin/update-server"
    scp_cmd "$PROJECT_ROOT/bin/mysoc-updater-linux-amd64" "$SERVER_USER@$SERVER_IP:$REMOTE_DIR/bin/mysoc-updater"
    
    # Make executable
    ssh_cmd "chmod +x $REMOTE_DIR/bin/*"
    
    log_success "Binaries deployed"
}

# Deploy migrations
deploy_migrations() {
    log_info "Deploying migrations..."
    
    scp_cmd -r "$PROJECT_ROOT/migrations/"* "$SERVER_USER@$SERVER_IP:$REMOTE_DIR/migrations/"
    
    log_success "Migrations deployed"
}

# Deploy systemd service files
deploy_services() {
    log_info "Deploying systemd service files..."
    
    # Create update-server service file
    cat > /tmp/update-server.service << 'EOF'
[Unit]
Description=MySoc Update Server
After=network.target postgresql.service

[Service]
Type=simple
User=bitnami
Group=bitnami
WorkingDirectory=/home/bitnami/updates-mysoc-ai
ExecStart=/home/bitnami/updates-mysoc-ai/bin/update-server
Restart=always
RestartSec=5
StandardOutput=append:/home/bitnami/updates-mysoc-ai/logs/update-server.log
StandardError=append:/home/bitnami/updates-mysoc-ai/logs/update-server.error.log

# Environment
EnvironmentFile=-/home/bitnami/updates-mysoc-ai/config/.env

[Install]
WantedBy=multi-user.target
EOF

    # Upload service file
    scp_cmd /tmp/update-server.service "$SERVER_USER@$SERVER_IP:/tmp/update-server.service"
    ssh_cmd "sudo mv /tmp/update-server.service /etc/systemd/system/update-server.service"
    ssh_cmd "sudo systemctl daemon-reload"
    
    log_success "Systemd service files deployed"
}

# Create environment config template
create_env_template() {
    log_info "Creating environment config template..."
    
    cat > /tmp/.env.template << 'EOF'
# MySoc Update Server Configuration
# Copy this file to .env and fill in the values

# Server
PORT=8080
HOST=0.0.0.0

# Database
DB_HOST=localhost
DB_PORT=5432
DB_NAME=mysoc_updates
DB_USER=postgres
DB_PASSWORD=changeme
DB_SSL_MODE=disable

# Storage
STORAGE_PATH=/home/bitnami/updates-mysoc-ai/data/releases

# Security
ADMIN_API_KEY=changeme-generate-secure-key
JWT_SECRET=changeme-generate-secure-jwt-secret

# Logging
LOG_LEVEL=info
EOF

    scp_cmd /tmp/.env.template "$SERVER_USER@$SERVER_IP:$REMOTE_DIR/config/.env.template"
    
    # Check if .env exists, if not copy template
    ssh_cmd "[ -f $REMOTE_DIR/config/.env ] || cp $REMOTE_DIR/config/.env.template $REMOTE_DIR/config/.env"
    
    log_success "Environment config template created"
}

# Restart services
restart_services() {
    log_info "Restarting services..."
    
    ssh_cmd "sudo systemctl restart update-server || sudo systemctl start update-server"
    ssh_cmd "sudo systemctl enable update-server"
    
    # Wait a moment and check status
    sleep 2
    
    if ssh_cmd "sudo systemctl is-active update-server" &> /dev/null; then
        log_success "update-server is running"
    else
        log_warn "update-server may not have started. Check logs:"
        log_warn "  ssh -i $SSH_KEY $SERVER_USER@$SERVER_IP 'tail -50 $REMOTE_DIR/logs/update-server.log'"
    fi
}

# Show deployment info
show_info() {
    echo ""
    echo "=============================================="
    echo -e "${GREEN}Deployment Complete!${NC}"
    echo "=============================================="
    echo ""
    echo "Server: $SERVER_USER@$SERVER_IP"
    echo "Directory: $REMOTE_DIR"
    echo ""
    echo "Binaries:"
    echo "  - $REMOTE_DIR/bin/update-server"
    echo "  - $REMOTE_DIR/bin/mysoc-updater"
    echo ""
    echo "Configuration:"
    echo "  - Edit: $REMOTE_DIR/config/.env"
    echo ""
    echo "Commands:"
    echo "  SSH:     ssh -i $SSH_KEY $SERVER_USER@$SERVER_IP"
    echo "  Logs:    ssh -i $SSH_KEY $SERVER_USER@$SERVER_IP 'tail -f $REMOTE_DIR/logs/update-server.log'"
    echo "  Status:  ssh -i $SSH_KEY $SERVER_USER@$SERVER_IP 'sudo systemctl status update-server'"
    echo "  Restart: ssh -i $SSH_KEY $SERVER_USER@$SERVER_IP 'sudo systemctl restart update-server'"
    echo ""
    echo "Next Steps:"
    echo "  1. Configure PostgreSQL database"
    echo "  2. Edit $REMOTE_DIR/config/.env with your settings"
    echo "  3. Run migrations: psql -d mysoc_updates -f $REMOTE_DIR/migrations/001_initial.up.sql"
    echo "  4. Restart: sudo systemctl restart update-server"
    echo ""
}

# Main deployment flow
main() {
    echo ""
    echo "=============================================="
    echo "  MySoc Updates Platform - Deployment"
    echo "=============================================="
    echo ""
    
    check_prerequisites
    build_binaries
    prepare_remote
    deploy_binaries
    deploy_migrations
    deploy_services
    create_env_template
    restart_services
    show_info
}

# Parse arguments
case "${1:-}" in
    --build-only)
        build_binaries
        ;;
    --deploy-only)
        prepare_remote
        deploy_binaries
        deploy_migrations
        deploy_services
        create_env_template
        restart_services
        show_info
        ;;
    --restart)
        restart_services
        ;;
    --status)
        ssh_cmd "sudo systemctl status update-server"
        ;;
    --logs)
        ssh_cmd "tail -f $REMOTE_DIR/logs/update-server.log"
        ;;
    --ssh)
        ssh -i "$SSH_KEY" "$SERVER_USER@$SERVER_IP"
        ;;
    *)
        main
        ;;
esac
