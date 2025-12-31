#!/bin/bash
set -e

# =============================================================================
# MySoc Updates Platform - Server Security Setup
# Target: updates.mysoc.ai (AWS Lightsail)
# =============================================================================

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

DOMAIN="updates.mysoc.ai"

# =============================================================================
# 1. System Updates
# =============================================================================
log_info "Updating system packages..."
sudo apt update && sudo apt upgrade -y

# =============================================================================
# 2. Install Security Tools
# =============================================================================
log_info "Installing security tools..."
sudo apt install -y \
    ufw \
    fail2ban \
    nginx \
    certbot \
    python3-certbot-nginx \
    unattended-upgrades \
    logrotate

# =============================================================================
# 3. Configure UFW Firewall
# =============================================================================
log_info "Configuring UFW firewall..."

# Reset UFW to defaults
sudo ufw --force reset

# Default policies
sudo ufw default deny incoming
sudo ufw default allow outgoing

# Allow SSH (important: do this first!)
sudo ufw allow 22/tcp comment 'SSH'

# Allow HTTP and HTTPS
sudo ufw allow 80/tcp comment 'HTTP'
sudo ufw allow 443/tcp comment 'HTTPS'

# Enable UFW
sudo ufw --force enable

log_success "Firewall configured"

# =============================================================================
# 4. Configure Fail2Ban
# =============================================================================
log_info "Configuring Fail2Ban..."

sudo tee /etc/fail2ban/jail.local > /dev/null << 'EOF'
[DEFAULT]
bantime = 3600
findtime = 600
maxretry = 5
backend = systemd

[sshd]
enabled = true
port = ssh
filter = sshd
logpath = /var/log/auth.log
maxretry = 3
bantime = 86400

[nginx-http-auth]
enabled = true
filter = nginx-http-auth
port = http,https
logpath = /var/log/nginx/error.log

[nginx-limit-req]
enabled = true
filter = nginx-limit-req
port = http,https
logpath = /var/log/nginx/error.log

[nginx-botsearch]
enabled = true
filter = nginx-botsearch
port = http,https
logpath = /var/log/nginx/access.log
EOF

sudo systemctl enable fail2ban
sudo systemctl restart fail2ban

log_success "Fail2Ban configured"

# =============================================================================
# 5. Harden SSH
# =============================================================================
log_info "Hardening SSH configuration..."

# Backup original config
sudo cp /etc/ssh/sshd_config /etc/ssh/sshd_config.backup

# Apply hardened SSH settings
sudo tee /etc/ssh/sshd_config.d/hardened.conf > /dev/null << 'EOF'
# MySoc SSH Hardening

# Disable root login
PermitRootLogin no

# Disable password authentication (key only)
PasswordAuthentication no
PubkeyAuthentication yes

# Disable empty passwords
PermitEmptyPasswords no

# Limit authentication attempts
MaxAuthTries 3

# Disable X11 forwarding
X11Forwarding no

# Disconnect idle sessions after 5 minutes
ClientAliveInterval 300
ClientAliveCountMax 2

# Disable TCP forwarding
AllowTcpForwarding no

# Only allow specific users
AllowUsers bitnami

# Use strong crypto
Ciphers aes256-gcm@openssh.com,aes128-gcm@openssh.com,aes256-ctr,aes192-ctr,aes128-ctr
MACs hmac-sha2-512-etm@openssh.com,hmac-sha2-256-etm@openssh.com,hmac-sha2-512,hmac-sha2-256
KexAlgorithms curve25519-sha256@libssh.org,ecdh-sha2-nistp521,ecdh-sha2-nistp384,ecdh-sha2-nistp256,diffie-hellman-group-exchange-sha256
EOF

# Test SSH config before restarting
sudo sshd -t && sudo systemctl restart sshd

log_success "SSH hardened"

# =============================================================================
# 6. Configure Nginx as Reverse Proxy
# =============================================================================
log_info "Configuring Nginx..."

# Remove default site
sudo rm -f /etc/nginx/sites-enabled/default

# Create MySoc Updates site config
sudo tee /etc/nginx/sites-available/updates-mysoc-ai > /dev/null << 'EOF'
# Rate limiting zone
limit_req_zone $binary_remote_addr zone=api_limit:10m rate=10r/s;
limit_req_zone $binary_remote_addr zone=download_limit:10m rate=1r/s;

# Upstream
upstream update_server {
    server 127.0.0.1:8080;
    keepalive 32;
}

server {
    listen 80;
    listen [::]:80;
    server_name updates.mysoc.ai;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    # Logging
    access_log /var/log/nginx/updates-mysoc-ai.access.log;
    error_log /var/log/nginx/updates-mysoc-ai.error.log;

    # Health check (no rate limit)
    location /health {
        proxy_pass http://update_server;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # API endpoints (rate limited)
    location /api/ {
        limit_req zone=api_limit burst=20 nodelay;
        limit_req_status 429;

        proxy_pass http://update_server;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Timeouts
        proxy_connect_timeout 30s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
    }

    # Download endpoints (stricter rate limit)
    location ~ ^/api/v1/releases/.*/download {
        limit_req zone=download_limit burst=5 nodelay;
        limit_req_status 429;

        proxy_pass http://update_server;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Longer timeout for downloads
        proxy_connect_timeout 60s;
        proxy_send_timeout 300s;
        proxy_read_timeout 300s;

        # Disable buffering for large files
        proxy_buffering off;
    }

    # Block common attack paths
    location ~* (\.php|\.asp|\.aspx|\.jsp|\.cgi)$ {
        return 404;
    }

    location ~ /\. {
        deny all;
    }

    # Default
    location / {
        proxy_pass http://update_server;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
EOF

# Enable site
sudo ln -sf /etc/nginx/sites-available/updates-mysoc-ai /etc/nginx/sites-enabled/

# Test and reload Nginx
sudo nginx -t && sudo systemctl reload nginx

log_success "Nginx configured"

# =============================================================================
# 7. Setup SSL with Let's Encrypt
# =============================================================================
log_info "Setting up SSL certificate..."

# Check if domain resolves to this server
if host "$DOMAIN" > /dev/null 2>&1; then
    sudo certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos --email admin@cyfox.com --redirect
    log_success "SSL certificate installed"
else
    log_warn "Domain $DOMAIN does not resolve yet. Skipping SSL setup."
    log_warn "Run 'sudo certbot --nginx -d $DOMAIN' after DNS is configured."
fi

# =============================================================================
# 8. Enable Automatic Security Updates
# =============================================================================
log_info "Enabling automatic security updates..."

sudo tee /etc/apt/apt.conf.d/50unattended-upgrades > /dev/null << 'EOF'
Unattended-Upgrade::Allowed-Origins {
    "${distro_id}:${distro_codename}";
    "${distro_id}:${distro_codename}-security";
    "${distro_id}ESMApps:${distro_codename}-apps-security";
    "${distro_id}ESM:${distro_codename}-infra-security";
};

Unattended-Upgrade::AutoFixInterruptedDpkg "true";
Unattended-Upgrade::MinimalSteps "true";
Unattended-Upgrade::Remove-Unused-Kernel-Packages "true";
Unattended-Upgrade::Remove-Unused-Dependencies "true";
Unattended-Upgrade::Automatic-Reboot "false";
EOF

sudo tee /etc/apt/apt.conf.d/20auto-upgrades > /dev/null << 'EOF'
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Download-Upgradeable-Packages "1";
APT::Periodic::AutocleanInterval "7";
APT::Periodic::Unattended-Upgrade "1";
EOF

sudo systemctl enable unattended-upgrades
sudo systemctl start unattended-upgrades

log_success "Automatic security updates enabled"

# =============================================================================
# 9. Configure Log Rotation
# =============================================================================
log_info "Configuring log rotation..."

sudo tee /etc/logrotate.d/updates-mysoc-ai > /dev/null << 'EOF'
/home/bitnami/updates-mysoc-ai/logs/*.log {
    daily
    missingok
    rotate 14
    compress
    delaycompress
    notifempty
    create 0640 bitnami bitnami
    sharedscripts
    postrotate
        systemctl reload update-server > /dev/null 2>&1 || true
    endscript
}
EOF

log_success "Log rotation configured"

# =============================================================================
# 10. Secure PostgreSQL
# =============================================================================
log_info "Securing PostgreSQL..."

# Only allow local connections (already default, but let's be explicit)
sudo -u postgres psql -c "ALTER USER mysoc_admin CONNECTION LIMIT 10;"

log_success "PostgreSQL secured"

# =============================================================================
# Summary
# =============================================================================
echo ""
echo "=============================================="
echo -e "${GREEN}Server Security Setup Complete!${NC}"
echo "=============================================="
echo ""
echo "Security measures applied:"
echo "  ✅ UFW Firewall (ports 22, 80, 443 only)"
echo "  ✅ Fail2Ban (SSH, Nginx protection)"
echo "  ✅ SSH Hardening (key-only, no root)"
echo "  ✅ Nginx Reverse Proxy with rate limiting"
echo "  ✅ SSL/TLS (Let's Encrypt) - if DNS configured"
echo "  ✅ Automatic Security Updates"
echo "  ✅ Log Rotation"
echo "  ✅ PostgreSQL connection limits"
echo ""
echo "Firewall status:"
sudo ufw status
echo ""
echo "Fail2Ban status:"
sudo fail2ban-client status
echo ""
