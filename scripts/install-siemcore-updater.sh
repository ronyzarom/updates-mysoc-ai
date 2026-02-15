#!/bin/bash
# SiemCore Updater Installation Script
# Usage: ./install-siemcore-updater.sh <instance-id> <license-key>
#
# Example:
#   ./install-siemcore-updater.sh siemcore-production SIEM-19B8-B873-824D-F52C

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}SiemCore Updater Installation${NC}"
echo "================================"

# Check arguments
if [ -z "$1" ] || [ -z "$2" ]; then
    echo -e "${RED}Error: Missing arguments${NC}"
    echo "Usage: $0 <instance-id> <license-key>"
    echo ""
    echo "Arguments:"
    echo "  instance-id   Unique instance identifier (e.g., siemcore-production)"
    echo "  license-key   Your license key (e.g., SIEM-XXXX-XXXX-XXXX-XXXX)"
    exit 1
fi

INSTANCE_ID="$1"
LICENSE_KEY="$2"
UPDATE_SERVER="${UPDATE_SERVER:-https://updates.mysoc.ai}"

echo "Instance ID: $INSTANCE_ID"
echo "License Key: ${LICENSE_KEY:0:9}****"
echo "Server: $UPDATE_SERVER"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: This script must be run as root${NC}"
    exit 1
fi

# Create directories
echo -e "${YELLOW}Creating directories...${NC}"
mkdir -p /opt/siemcore/bin
mkdir -p /opt/siemcore/updater/{versions,backups,temp}
mkdir -p /var/log/siemcore-updater

# Download updater binary
echo -e "${YELLOW}Downloading siemcore-updater...${NC}"
DOWNLOAD_URL="$UPDATE_SERVER/siemcore-updater/latest/siemcore-updater-linux-amd64"
if ! curl -fsSL "$DOWNLOAD_URL" -o /opt/siemcore/bin/siemcore-updater 2>/dev/null; then
    echo -e "${YELLOW}Download from server failed, checking for local binary...${NC}"
    if [ -f "./siemcore-updater-linux-amd64" ]; then
        cp ./siemcore-updater-linux-amd64 /opt/siemcore/bin/siemcore-updater
    else
        echo -e "${RED}Error: Could not download or find updater binary${NC}"
        exit 1
    fi
fi
chmod +x /opt/siemcore/bin/siemcore-updater

# Create config file
echo -e "${YELLOW}Creating configuration...${NC}"
cat > /opt/siemcore/updater/config.yaml << EOF
# SiemCore Updater Configuration
# Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)

server:
  url: $UPDATE_SERVER
  api_key: ""

instance:
  id: "$INSTANCE_ID"
  type: "siemcore"
  license_key: "$LICENSE_KEY"

heartbeat:
  interval: 60s
  timeout: 10s

update:
  check_interval: 5m
  channel: stable
  auto_update: true

products:
  - name: siemcore
    service: siemcore
    binary: /opt/siemcore/bin/siemcore
    type: binary

security:
  enabled: true
  scan_interval: 1h
  firewall:
    enabled: true
    default_policy: deny
  ssh:
    enabled: true
    enforce:
      PermitRootLogin: "no"
      PasswordAuthentication: "no"
  os_updates:
    enabled: true
    security_only: true

logging:
  level: info
  file: /var/log/siemcore-updater/updater.log
  max_size: 100MB
  max_backups: 5
EOF

# Create systemd service
echo -e "${YELLOW}Creating systemd service...${NC}"
cat > /etc/systemd/system/siemcore-updater.service << 'EOF'
[Unit]
Description=SiemCore Updater Service
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=/opt/siemcore/bin/siemcore-updater daemon
Restart=always
RestartSec=10
StandardOutput=append:/var/log/siemcore-updater/updater.log
StandardError=append:/var/log/siemcore-updater/updater.log

[Install]
WantedBy=multi-user.target
EOF

# Enable and start service
echo -e "${YELLOW}Enabling and starting service...${NC}"
systemctl daemon-reload
systemctl enable siemcore-updater
systemctl start siemcore-updater

# Wait and check status
sleep 3
if systemctl is-active --quiet siemcore-updater; then
    echo ""
    echo -e "${GREEN}Installation complete!${NC}"
    echo ""
    echo "The siemcore-updater is now running and sending heartbeats."
    echo "Check the dashboard at: $UPDATE_SERVER/instances"
    echo ""
    echo "Useful commands:"
    echo "  systemctl status siemcore-updater    - Check service status"
    echo "  journalctl -u siemcore-updater -f    - View logs"
    echo "  siemcore-updater status              - Check updater status"
else
    echo ""
    echo -e "${RED}Warning: Service may not have started correctly${NC}"
    echo "Check logs with: journalctl -u siemcore-updater -n 50"
fi
