# SiemCore Deployment Guide - Updates Server

| | |
|---|---|
| **Document Version** | 1.5.0 |
| **Last Updated** | February 3, 2026 |
| **Status** | Production |
| **Maintained By** | SiemCore Platform Team |

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.5.0 | 2026-02-03 | SiemCore Team | Added API key requirement, target groups fix (include "stable"), dashboard features (edit/delete), semantic versioning |
| 1.4.0 | 2026-01-31 | SiemCore Team | Added complete update workflow, upload-release.sh script, step-by-step commands |
| 1.3.0 | 2026-01-31 | SiemCore Team | Added deployment policy, removed sensitive keys, clarified channels vs groups |
| 1.2.0 | 2026-01-31 | SiemCore Team | Added quick install script, download links, updated installation instructions |
| 1.1.0 | 2026-01-28 | SiemCore Team | Added SiemCore environments section with server details |
| 1.0.0 | 2026-01-28 | SiemCore Team | Initial release - complete deployment guide |

---

This guide explains how to deploy and manage SiemCore instances using the MySoc Updates Server at `updates.mysoc.ai`.

## Table of Contents

1. [Overview](#overview)
2. [Deployment Policy](#deployment-policy-strict)
3. [SiemCore Environments](#siemcore-environments)
4. [Architecture](#architecture)
5. [Installing the Updater](#installing-the-updater)
6. [License Key](#license-key)
7. [Dashboard Access](#dashboard-access)
8. [Managing Instances](#managing-instances)
9. [Channels vs Update Groups](#channels-vs-update-groups)
10. [Staged Rollouts](#staged-rollouts)
11. [Release Rules](#release-rules)
12. [Uploading Releases](#uploading-releases)
13. [Complete Update Workflow](#complete-update-workflow)
14. [Troubleshooting](#troubleshooting)
15. [API Reference](#api-reference)
16. [Support](#support)

---

## Overview

The Updates Server provides centralized management for all SiemCore deployments:

- **Automatic Registration** - Instances automatically appear in the dashboard when the updater checks in
- **Heartbeat Monitoring** - Real-time health metrics (CPU, memory, disk, security status)
- **Staged Rollouts** - Control which instances receive updates via update groups
- **License Validation** - Instances are linked to their license keys for entitlement tracking

---

## Deployment Policy (Strict)

| Rule | Description |
|------|-------------|
| **Testing only: direct deploy** | `deploy-all.sh` is allowed **only** on `testing.siemcore.ai` |
| **Updates Server for all others** | `cloud.siemcore.ai` and `cyfox-il.siemcore.ai` receive releases **only** via the Updates Server |
| **Production consent gate** | Any action that changes `cyfox-il.siemcore.ai` version requires **explicit consent** |
| **SSH is break-glass only** | SSH access to `cloud`/`cyfox-il` is for verification and emergency troubleshooting only |

> **Important:** Production instances (`cyfox-il.siemcore.ai`) must have `auto_update: false`. Rollout requires deliberate action in the dashboard.

---

## SiemCore Environments

SiemCore operates three server environments:

| Environment | Server | Purpose | Update Group |
|-------------|--------|---------|--------------|
| **Testing** | `testing.siemcore.ai` | Internal testing and development | `alpha` |
| **Staging** | `cloud.siemcore.ai` | Pre-production validation | `beta` |
| **Production** | `cyfox-il.siemcore.ai` | Live customer deployments | `production` |

### Environment Details

> **Security Note:** Never commit real license keys to documentation or repositories. Retrieve keys from the dashboard at https://updates.mysoc.ai/licenses.

#### Testing (`testing.siemcore.ai`)

- **Purpose:** Internal development and QA testing
- **Update Group:** `alpha`
- **Deployment:** Direct deploy (`deploy-all.sh`) allowed, OR Updates Server
- **Auto-Update:** Enabled
- **Instance ID:** `siemcore-testing`

```yaml
# /opt/siemcore/updater/config.yaml
instance:
  id: "siemcore-testing"
  type: "siemcore"
  license_key: "SIEM-XXXX-XXXX-XXXX-XXXX"  # Get from dashboard

update:
  channel: stable
  auto_update: true
```

#### Staging (`cloud.siemcore.ai`)

- **Purpose:** Pre-production validation, customer demos
- **Update Group:** `beta`
- **Deployment:** Updates Server only (no deploy-all)
- **Auto-Update:** Enabled
- **Instance ID:** `siemcore-staging`

```yaml
# /opt/siemcore/updater/config.yaml
instance:
  id: "siemcore-staging"
  type: "siemcore"
  license_key: "SIEM-XXXX-XXXX-XXXX-XXXX"  # Get from dashboard

update:
  channel: stable
  auto_update: true
```

#### Production (`cyfox-il.siemcore.ai`)

- **Purpose:** Live customer environment
- **Update Group:** `production`
- **Deployment:** Updates Server only; requires explicit consent
- **Auto-Update:** Disabled (manual approval required)
- **Instance ID:** `siemcore-production`

```yaml
# /opt/siemcore/updater/config.yaml
instance:
  id: "siemcore-production"
  type: "siemcore"
  license_key: "SIEM-XXXX-XXXX-XXXX-XXXX"  # Get from dashboard

update:
  channel: stable
  auto_update: false  # MUST be false for production
  maintenance_window:
    start: "02:00"
    end: "05:00"
    timezone: "Asia/Jerusalem"
```

### Recommended Rollout Flow

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│     Testing     │     │     Staging     │     │   Production    │
│ testing.siemcore│     │ cloud.siemcore  │     │ cyfox-il.siemcore│
│                 │     │                 │     │                 │
│  Update Group:  │     │  Update Group:  │     │  Update Group:  │
│     alpha       │     │      beta       │     │   production    │
└────────┬────────┘     └────────┬────────┘     └────────┬────────┘
         │                       │                       │
         ▼                       ▼                       ▼
    Day 1: Deploy          Day 3: Deploy          Day 7+: Deploy
    Auto-update: ON        Auto-update: ON        Auto-update: OFF
                                                  (manual approval)
```

### Setting Update Groups

In the dashboard (https://updates.mysoc.ai/instances):

1. Click on an instance
2. Under **Update Settings**, select the appropriate group:
   - `testing.siemcore.ai` → **alpha**
   - `cloud.siemcore.ai` → **beta**
   - `cyfox-il.siemcore.ai` → **production**
3. Click **Save**

Or via API:

```bash
# Set testing to alpha
curl -X PUT https://updates.mysoc.ai/api/v1/instances/{instance-id}/update-group \
  -H "Content-Type: application/json" \
  -d '{"group": "alpha"}'

# Set staging to beta
curl -X PUT https://updates.mysoc.ai/api/v1/instances/{instance-id}/update-group \
  -H "Content-Type: application/json" \
  -d '{"group": "beta"}'

# Set production to production
curl -X PUT https://updates.mysoc.ai/api/v1/instances/{instance-id}/update-group \
  -H "Content-Type: application/json" \
  -d '{"group": "production"}'
```

---

## Architecture

```
┌─────────────────────┐         ┌─────────────────────┐
│   SiemCore Server   │         │   Updates Server    │
│                     │         │  updates.mysoc.ai   │
│  ┌───────────────┐  │  HTTPS  │                     │
│  │siemcore-updater├──────────►│  /api/v1/updates/*  │
│  └───────────────┘  │         │  /api/v1/heartbeat  │
│                     │         │                     │
│  siemcore-api       │         │  ┌───────────────┐  │
│  siemcore-collector │         │  │   Dashboard   │  │
│  siemcore-frontend  │         │  └───────────────┘  │
└─────────────────────┘         └─────────────────────┘
```

The `siemcore-updater` runs as a systemd service on each SiemCore instance. It:

1. Sends heartbeats every 60 seconds with system metrics
2. Checks for available updates
3. Downloads and applies updates when available (if auto-update is enabled)
4. Reports update success/failure back to the server

### What the Updater Reports

Each heartbeat includes comprehensive system information displayed in the dashboard:

| Category | Metrics |
|----------|---------|
| **System Metrics** | CPU usage (%), Memory usage/total, Disk usage/total, Load average, Uptime |
| **Security Status** | Firewall enabled/disabled, SSH hardened, Security score (0-100), Pending updates count |
| **Product Status** | Product name, Version, Running status (running/stopped/crashed), Health endpoint status |
| **Instance Info** | Instance ID, Hostname, Instance type, License validation |

This data appears in the dashboard under each instance's detail page.

---

## Installing the Updater

### Prerequisites

- SiemCore instance running on Linux (Ubuntu 20.04+ or Debian 11+)
- License key for the instance
- Root/sudo access

### Quick Install (Recommended)

**Recommended:** Download and verify before execution:

```bash
# Download the install script
curl -fsSLO https://updates.mysoc.ai/siemcore-updater/latest/install-siemcore-updater.sh

# Review the script (recommended)
less install-siemcore-updater.sh

# Execute with your instance ID and license key
sudo bash install-siemcore-updater.sh <instance-id> <license-key>
```

**Quick install (convenience method):**

```bash
curl -fsSL https://updates.mysoc.ai/siemcore-updater/latest/install-siemcore-updater.sh | sudo bash -s -- <instance-id> <license-key>
```

**For each SiemCore server:**

| Server | Instance ID | License Key |
|--------|-------------|-------------|
| `testing.siemcore.ai` | `siemcore-testing` | Get from dashboard |
| `cloud.siemcore.ai` | `siemcore-staging` | Get from dashboard |
| `cyfox-il.siemcore.ai` | `siemcore-production` | Get from dashboard |

> **Note:** Retrieve license keys from https://updates.mysoc.ai/licenses. Never hardcode keys in scripts or documentation.

The script will:
- Download the updater binary
- Create the configuration file
- Set up the systemd service
- Start the updater daemon

### Download Links

| File | URL |
|------|-----|
| **Updater Binary** | https://updates.mysoc.ai/siemcore-updater/latest/siemcore-updater-linux-amd64 |
| **Install Script** | https://updates.mysoc.ai/siemcore-updater/latest/install-siemcore-updater.sh |

### Manual Installation

If you prefer manual installation:

1. **Download the updater binary:**

```bash
sudo mkdir -p /opt/siemcore/bin
curl -fsSL https://updates.mysoc.ai/siemcore-updater/latest/siemcore-updater-linux-amd64 \
  -o /opt/siemcore/bin/siemcore-updater
chmod +x /opt/siemcore/bin/siemcore-updater
```

2. **Create the configuration file:**

```bash
sudo mkdir -p /opt/siemcore/updater/{versions,backups,temp}
sudo cat > /opt/siemcore/updater/config.yaml << 'EOF'
# SiemCore Updater Configuration

# Update Server Connection
server:
  url: https://updates.mysoc.ai
  api_key: ""  # Optional, for authenticated endpoints

# Instance Identification
instance:
  id: "siemcore-CUSTOMER_NAME"      # Unique instance identifier
  type: "siemcore"                   # Instance type: siemcore or mysoc
  license_key: "SIEM-XXXX-XXXX-XXXX-XXXX"  # Your license key

# Heartbeat Configuration
heartbeat:
  interval: 60s      # How often to send heartbeats
  timeout: 10s       # HTTP request timeout

# Update Configuration
update:
  check_interval: 5m   # How often to check for updates
  channel: stable      # Release channel: stable, beta, nightly
  auto_update: true    # Automatically apply updates
  # Optional: Restrict updates to specific time window
  # maintenance_window:
  #   start: "02:00"
  #   end: "05:00"
  #   timezone: "UTC"

# Products to Manage
products:
  - name: siemcore-api
    service: siemcore-api           # systemd service name
    binary: /opt/siemcore/bin/siemcore-api
    config: /opt/siemcore/config/api.yaml
    health_endpoint: http://localhost:8080/health
    hot_reload: false

  - name: siemcore-collector
    service: siemcore-collector
    binary: /opt/siemcore/bin/siemcore-collector
    config: /opt/siemcore/config/collector.yaml
    health_endpoint: http://localhost:8081/health
    hot_reload: false

  - name: siemcore-frontend
    service: siemcore-frontend
    binary: /opt/siemcore/frontend
    type: data                      # Static files, not a binary
    hot_reload: true

# Security Hardening (optional)
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

# Logging
logging:
  level: info
  file: /var/log/siemcore-updater/updater.log
  max_size: 100MB
  max_backups: 5
EOF
```

**Important Configuration Values:**

| Field | Description | Example |
|-------|-------------|---------|
| `instance.id` | Unique identifier for this instance | `siemcore-acme-prod-01` |
| `instance.type` | Product type | `siemcore` or `mysoc` |
| `instance.license_key` | License key from dashboard | `SIEM-XXXX-XXXX-XXXX-XXXX` |
| `update.channel` | Release channel | `stable`, `beta`, `nightly` |
| `update.auto_update` | Auto-apply updates | `true` or `false` |

3. **Create the systemd service:**

```bash
sudo cat > /etc/systemd/system/siemcore-updater.service << 'EOF'
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
```

4. **Create log directory and enable service:**

```bash
sudo mkdir -p /var/log/siemcore-updater
sudo systemctl daemon-reload
sudo systemctl enable siemcore-updater
sudo systemctl start siemcore-updater
```

5. **Verify the updater is running:**

```bash
sudo systemctl status siemcore-updater
sudo journalctl -u siemcore-updater -f
```

The instance should appear in the dashboard within 60 seconds.

---

## License Key

### What is a License Key?

The license key (`SIEM-XXXX-XXXX-XXXX-XXXX`) identifies your entitlement to use SiemCore products. It is:

- **Unique per customer** - Each customer receives their own license key
- **Product-scoped** - Defines which products you're licensed to use
- **Time-limited** - Has an expiration date

### Obtaining a License Key

1. **From the Dashboard:**
   - Log in to https://updates.mysoc.ai
   - Navigate to **Licenses**
   - Find your organization's license
   - Copy the license key (format: `SIEM-XXXX-XXXX-XXXX-XXXX`)

2. **From your Account Manager:**
   - Contact support@mysoc.ai
   - Provide your organization name
   - Receive license key via secure channel

### License Key Usage

The license key is used in two places:

1. **Updater Configuration:**
   ```yaml
   instance:
     license_key: "SIEM-XXXX-XXXX-XXXX-XXXX"
   ```

2. **HTTP Header (for API calls):**
   ```
   X-License-Key: SIEM-XXXX-XXXX-XXXX-XXXX
   ```

### How License Validation Works

```
┌─────────────────────┐         ┌─────────────────────┐
│   SiemCore Instance │         │   Updates Server    │
│                     │         │                     │
│  1. Send heartbeat  │────────►│  2. Extract license │
│     with license    │         │     from header     │
│                     │         │                     │
│                     │         │  3. Validate:       │
│                     │         │     - Key exists?   │
│                     │         │     - Not expired?  │
│                     │         │     - Products OK?  │
│                     │         │                     │
│  5. Continue/stop   │◄────────│  4. Return status   │
│     based on result │         │                     │
└─────────────────────┘         └─────────────────────┘
```

**On each heartbeat:**
1. Updater sends license key in `X-License-Key` header
2. Server looks up the license in database
3. Server validates:
   - License exists and is active
   - License hasn't expired
   - Instance's products are covered by license
4. Server creates/updates instance record
5. Instance continues operating (updates available based on license)

### License Types

| Type | Description | Typical Use |
|------|-------------|-------------|
| `siemcore` | Standard SiemCore license | Production deployments |
| `enterprise` | Enterprise features enabled | Large organizations |
| `trial` | Time-limited evaluation | New customers |

### License Expiration

- **30 days before expiry** - Warning shown in dashboard
- **On expiry** - Instance continues to run but updates stop
- **Grace period** - 7 days after expiry before instance marked inactive

### Troubleshooting License Issues

**"License not found" error:**
```bash
# Check the license key in config
grep license_key /opt/siemcore/updater/config.yaml

# Verify it matches dashboard exactly (case-sensitive)
```

**"License expired" error:**
```bash
# Check expiration in dashboard: Licenses → Your License → Expires
# Contact support for renewal
```

**Instance not appearing in dashboard:**
```bash
# Verify heartbeat is being sent
sudo journalctl -u siemcore-updater | grep -i heartbeat

# Check network connectivity
curl -v https://updates.mysoc.ai/health

# Verify license key format (should be SIEM-XXXX-XXXX-XXXX-XXXX)
```

---

## Dashboard Access

Access the dashboard at: **https://updates.mysoc.ai**

### Login Credentials

Contact your administrator for login credentials. The dashboard uses:
- Email/password authentication
- Optional MFA (TOTP)

### Dashboard Sections

| Section | Description |
|---------|-------------|
| **Dashboard** | Overview of all instances and their status |
| **Instances** | Detailed view of each instance, configure auto-update and groups |
| **Releases** | View all uploaded releases by product |
| **Licenses** | Manage customer licenses |
| **Security** | View audit logs and security events |

---

## Managing Instances

### Viewing Instance Details

1. Navigate to **Instances** in the sidebar
2. Click on any instance card to view details

### Instance Detail Page Shows:

- **Status** - Online/Offline/Degraded
- **System Metrics** - CPU, Memory, Disk usage
- **Products** - Installed products and versions
- **Security** - Firewall status, SSH hardening, security score
- **Last Heartbeat** - When the instance last checked in

### Configuring Updates

On the instance detail page, you can configure:

#### Display Name

Set a friendly name for the instance (e.g., `cloud.siemcore.ai`):

1. Click the **pencil icon** next to the hostname
2. Enter the display name
3. Click the **checkmark** to save

This helps identify instances in the dashboard without relying on auto-generated hostnames.

#### Auto-Update Toggle

- **Enabled** - Instance will automatically download and apply updates
- **Disabled** - Instance will only report available updates, manual intervention required

#### Update Group

Assign the instance to a rollout group:

| Group | Description | Example |
|-------|-------------|---------|
| `alpha` | Internal testing, receives updates first | `testing.siemcore.ai` |
| `beta` | Pre-production validation | `cloud.siemcore.ai` |
| `stable` | Default group for most instances | Most instances |
| `production` | Customer systems, requires explicit consent | `cyfox-il.siemcore.ai` |

> **Note:** Production instances should always have `auto_update: false`. Updates require deliberate action in the dashboard.

#### Deleting Stale Instances

To remove old/duplicate instances from the dashboard:

1. Click on the instance card to view details
2. Click the red **Delete** button
3. Confirm deletion in the popup

This removes the instance record and heartbeat history. The instance will reappear if its updater is still running.

---

## Channels vs Update Groups

Two separate concepts control update delivery:

| Concept | Purpose | Values |
|---------|---------|--------|
| **Channel** | Build quality/stability of the release | `stable`, `beta`, `nightly` |
| **Update Group** | Which instances receive the release (rollout ring) | `alpha`, `beta`, `production` |

- **Channel** = "What kind of build is this?" (stable releases vs experimental)
- **Update Group** = "Who gets this release?" (internal → pre-prod → customers)

Most instances use `channel: stable` but belong to different update groups.

---

## Staged Rollouts

Staged rollouts let you control which instances receive updates by targeting specific update groups.

### How It Works

1. Upload a new release with target groups (e.g., `alpha`)
2. Only instances in the `alpha` group receive the update
3. After testing, expand target groups to include `beta`
4. After validation, expand to `production` (requires consent)

### Rollout Strategy Example

```
Day 1:  Release v2.0.1 → target_groups: [alpha, stable]
        ↳ testing.siemcore.ai receives update automatically
        ↳ (include "stable" so default instances can see it)

Day 3:  Update release → target_groups: [alpha, beta, stable]
        ↳ cloud.siemcore.ai receives update automatically

Day 7+: Update release → target_groups: [alpha, beta, stable, production]
        ↳ cyfox-il.siemcore.ai sees update available
        ↳ Requires manual approval (auto_update: false)
```

> **Important:** Always include `stable` in target groups! Most instances default to `update_group: stable`.

### Checking Update Availability

An instance receives an update only if:

1. ✅ `auto_update_enabled` is `true` for the instance (or manual trigger)
2. ✅ Instance's `update_group` is in the release's `target_groups`
3. ✅ Release version is newer than installed version

---

## Release Rules

| Rule | Description |
|------|-------------|
| **Immutability** | Releases are immutable once published. Never replace an artifact for the same version. |
| **Rollback** | Rollback restores the previous artifact. Use `siemcore-updater rollback` if needed. |
| **Schema changes** | Database/schema changes must be backward-compatible (expand/contract pattern). |
| **Version format** | Use semantic versioning: `MAJOR.MINOR.PATCH` (e.g., `2.0.17`) |
| **Version comparison** | Server always returns the **highest** semantic version. Uploading older versions won't cause downgrades. |

### Semantic Version Comparison

The Updates Server compares versions numerically:
- `2.0.0` > `1.9.9` (major version wins)
- `2.1.0` > `2.0.9` (minor version wins)
- `2.0.17` > `2.0.2` (patch version compared as numbers, not strings)

If an instance has `v2.0.17` and you upload `v2.0.2`, the instance will **not** receive a downgrade.

---

## Uploading Releases

### API Key Requirement

All release uploads require the **Admin API Key**. Get it from the Updates Server administrator.

```bash
# Store the API key securely
echo "mysoc-admin-key-XXXXXXXX" > keys/UPDATES-API-KEY.txt
chmod 600 keys/UPDATES-API-KEY.txt
```

### Using the Upload Script

```bash
./scripts/upload-release.sh \
  --product siemcore \
  --version 2.0.1 \
  --channel stable \
  --groups alpha,beta,stable,production \
  --file ./bin/siemcore-linux-amd64 \
  --api-key "$(cat keys/UPDATES-API-KEY.txt)" \
  --notes "Bug fixes and performance improvements"
```

> **Important:** Always include `stable` in target groups! Most instances have `update_group: stable` by default. If you omit it, those instances won't see the update.

### Script Options

| Option | Required | Description |
|--------|----------|-------------|
| `--product` | Yes | Product name (e.g., `siemcore`) |
| `--version` | Yes | Semantic version (e.g., `2.0.1`) |
| `--channel` | No | Release channel (default: `stable`) |
| `--groups` | Yes | Comma-separated target groups (include `stable`!) |
| `--file` | Yes | Path to the release artifact |
| `--api-key` | Yes | Admin API key for authentication |
| `--notes` | No | Release notes (markdown supported) |

### Using cURL Directly

```bash
curl -X POST https://updates.mysoc.ai/api/v1/releases \
  -H "X-API-Key: YOUR_ADMIN_API_KEY" \
  -F "product=siemcore" \
  -F "version=2.0.1" \
  -F "channel=stable" \
  -F "target_groups=alpha,beta,stable,production" \
  -F "release_notes=Bug fixes and improvements" \
  -F "artifact=@./bin/siemcore-linux-amd64"
```

### Updating Target Groups for Existing Release

To expand rollout to more groups (or fix missing groups):

```bash
curl -X PUT https://updates.mysoc.ai/api/v1/releases/siemcore/2.0.1/target-groups \
  -H "X-API-Key: YOUR_ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"target_groups": ["alpha", "beta", "stable", "production"]}'
```

### Editing Releases via Dashboard

You can also edit releases directly in the dashboard:

1. Go to **Releases** → Click the **pencil icon** on any release
2. Modify **Target Groups** or **Release Notes**
3. Click **Save Changes**

### Deleting Releases

To delete a release (removes from database and stops availability):

1. Go to **Releases** → Click the **trash icon** on any release
2. Confirm deletion in the popup

Or via API:

```bash
curl -X DELETE https://updates.mysoc.ai/api/v1/releases/siemcore/2.0.1 \
  -H "X-API-Key: YOUR_ADMIN_API_KEY"
```

---

## Complete Update Workflow

### How Updates Flow

```
  Developer                 Updates Server              SiemCore Instances
  ─────────                 ──────────────              ──────────────────
      │                           │                            │
      │  1. Upload release        │                            │
      │  ───────────────────────► │                            │
      │  (target: alpha)          │                            │
      │                           │                            │
      │                           │  2. Heartbeat + check      │
      │                           │ ◄───────────────────────── │ (every 60s)
      │                           │                            │
      │                           │  3. "New version available"│
      │                           │ ─────────────────────────► │ (if in alpha)
      │                           │                            │
      │                           │  4. Download + apply       │
      │                           │ ◄───────────────────────── │ (if auto_update)
      │                           │                            │
      │                           │  5. Report success/fail    │
      │                           │ ◄───────────────────────── │
```

### Step-by-Step Workflow

| Step | Action | Who | Command |
|------|--------|-----|---------|
| 1 | Build binary | Developer | `make build` |
| 2 | Upload to alpha | Developer | `./scripts/upload-release.sh --groups alpha` |
| 3 | Testing auto-updates | Automatic | (heartbeat detects update) |
| 4 | Verify on testing | Developer | SSH check |
| 5 | Expand to beta | Developer | API call to add beta group |
| 6 | Cloud auto-updates | Automatic | (heartbeat detects update) |
| 7 | Verify on cloud | Developer | SSH check |
| 8 | Expand to production | Developer | API call to add production |
| 9 | **Manual trigger** | Developer | SSH to cyfox-il (consent required) |

### Step 1: Upload Release

```bash
# Upload to alpha group first (testing only)
# Include "stable" so default instances can see it
./scripts/upload-release.sh \
  --product siemcore \
  --version 2.0.1 \
  --file ./bin/siemcore-linux-amd64 \
  --channel stable \
  --groups alpha,stable \
  --api-key "$(cat keys/UPDATES-API-KEY.txt)" \
  --notes "Bug fixes and performance improvements"
```

### Step 2: Verify on Testing

```bash
# Check if testing server sees the update
ssh user@testing.siemcore.ai "sudo /opt/siemcore/bin/siemcore-updater update --check"

# View update logs
ssh user@testing.siemcore.ai "sudo journalctl -u siemcore-updater -f"
```

### Step 3: Expand to Beta (Cloud)

```bash
# Add beta group (keep stable for default instances)
curl -X PUT https://updates.mysoc.ai/api/v1/releases/siemcore/2.0.1/target-groups \
  -H "X-API-Key: $(cat keys/UPDATES-API-KEY.txt)" \
  -H "Content-Type: application/json" \
  -d '{"target_groups": ["alpha", "beta", "stable"]}'
```

### Step 4: Expand to Production

```bash
# Add production group (instances will see update available)
curl -X PUT https://updates.mysoc.ai/api/v1/releases/siemcore/2.0.1/target-groups \
  -H "X-API-Key: $(cat keys/UPDATES-API-KEY.txt)" \
  -H "Content-Type: application/json" \
  -d '{"target_groups": ["alpha", "beta", "stable", "production"]}'
```

### Step 5: Trigger Production Update (Manual)

Since `cyfox-il.siemcore.ai` has `auto_update: false`, manually trigger:

```bash
# SSH to production and trigger update
ssh user@cyfox-il.siemcore.ai "sudo /opt/siemcore/bin/siemcore-updater update --force"
```

### Quick Reference Commands

```bash
# Upload release (include stable so default instances see it)
./scripts/upload-release.sh \
  --product siemcore \
  --version 2.0.1 \
  --file ./bin/siemcore-linux-amd64 \
  --groups alpha,beta,stable,production \
  --api-key "$(cat keys/UPDATES-API-KEY.txt)"

# Check if server sees update
ssh user@server "sudo /opt/siemcore/bin/siemcore-updater update --check"

# Force update on any server
ssh user@server "sudo /opt/siemcore/bin/siemcore-updater update --force"

# View update logs
ssh user@server "sudo journalctl -u siemcore-updater -f"

# Rollback if needed
ssh user@server "sudo /opt/siemcore/bin/siemcore-updater rollback"

# Edit release via API (e.g., add missing groups)
curl -X PUT https://updates.mysoc.ai/api/v1/releases/siemcore/2.0.1 \
  -H "X-API-Key: YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{"target_groups": ["alpha", "beta", "stable", "production"]}'
```

---

## Troubleshooting

### Instance Not Appearing in Dashboard

1. **Check updater service:**
   ```bash
   sudo systemctl status siemcore-updater
   ```

2. **Check logs:**
   ```bash
   sudo journalctl -u siemcore-updater -f
   ```

3. **Verify network connectivity:**
   ```bash
   curl -v https://updates.mysoc.ai/health
   ```

4. **Verify license key in config:**
   ```bash
   cat /etc/siemcore/updater.yaml
   ```

### Instance Shows "Offline"

An instance is marked offline if no heartbeat is received for 5 minutes.

1. Check if the updater service is running
2. Check for network issues between instance and updates server
3. Check system resources (CPU/memory exhaustion can prevent heartbeats)

### Updates Not Being Applied

1. **Check auto-update is enabled:**
   - In dashboard: Instance → Update Settings → Auto Update toggle
   - Or check config: `auto_update: true` in `/etc/siemcore/updater.yaml`

2. **Check update group matches:**
   - Instance's `update_group` must be in the release's `target_groups`
   - **Common issue:** Release has `["alpha", "beta", "production"]` but instance has `update_group: stable`
   - **Fix:** Add `stable` to the release's target groups via dashboard or API

3. **Check version comparison:**
   - Update only shows if release version is **higher** than installed version
   - Check installed version: `ssh server "cat /opt/siemcore/updater/versions/siemcore.version"`

4. **Check updater logs:**
   ```bash
   sudo journalctl -u siemcore-updater --since "1 hour ago"
   ```

### Manual Update Check

Force an immediate update check:

```bash
sudo siemcore-updater update --check
```

Force an update (bypass auto-update setting):

```bash
sudo siemcore-updater update --force
```

### Rollback

If an update causes issues, rollback to the previous version:

```bash
sudo siemcore-updater rollback
```

---

## API Reference

### Endpoints Used by Updater

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/api/v1/heartbeat` | POST | `X-License-Key` | Send instance heartbeat with metrics |
| `/api/v1/updates/{product}/check` | POST | `X-License-Key` | Check for available updates (returns download URL) |
| `/api/v1/releases/{product}/{version}/download` | GET | `X-License-Key` | Download release artifact |
| `/api/v1/updates/report` | POST | `X-License-Key` | Report update success/failure |

### Admin-Only Endpoints

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/api/v1/releases` | POST | `X-API-Key` | Upload new release |
| `/api/v1/releases/{product}/{version}` | PUT | `X-API-Key` | Update release (notes, groups) |
| `/api/v1/releases/{product}/{version}` | DELETE | `X-API-Key` | Delete release |
| `/api/v1/releases/{product}/{version}/target-groups` | PUT | `X-API-Key` | Update release target groups |
| `/api/v1/instances/{id}` | PUT | `X-API-Key` | Update instance (display_name, settings) |
| `/api/v1/instances/{id}` | DELETE | `X-API-Key` | Delete instance |
| `/api/v1/admin/licenses` | GET/POST | `X-API-Key` | Manage licenses |

### Authentication

| Header | Used By | Description |
|--------|---------|-------------|
| `X-License-Key` | **Instances** | License key for instance identification. Instances use this for ALL operations. |
| `X-API-Key` | **Admin/CI only** | API key for admin operations (release uploads, license management). |

> **Security:** Instances never store or use admin API keys. All instance operations use `X-License-Key` only.

### Heartbeat Payload

```json
{
  "instance_id": "siemcore-production",
  "instance_type": "siemcore",
  "hostname": "siemcore-prod-01.example.com",
  "updater_version": "1.0.0",
  "config_hash": "abc123",
  "license": {
    "key": "SIEM-XXXX-XXXX-XXXX-XXXX",
    "valid": true,
    "last_check": "2026-01-28T10:00:00Z"
  },
  "products": [
    {
      "name": "siemcore-api",
      "version": "1.4.2",
      "channel": "stable",
      "status": "running",
      "pid": 12345,
      "health_endpoint": "http://localhost:8080/health",
      "health_status": "healthy"
    },
    {
      "name": "siemcore-collector",
      "version": "1.4.2",
      "channel": "stable",
      "status": "running",
      "pid": 12346,
      "health_status": "healthy"
    }
  ],
  "system": {
    "os": "linux",
    "arch": "amd64",
    "cpu_usage": 15.5,
    "memory_total": 8589934592,
    "memory_used": 4294967296,
    "disk_total": 107374182400,
    "disk_used": 53687091200,
    "load_average": 0.75,
    "uptime": 864000
  },
  "security": {
    "firewall_enabled": true,
    "ssh_hardened": true,
    "pending_updates": 5,
    "security_updates": 2,
    "reboot_required": false,
    "last_scan": "2026-01-28T09:00:00Z"
  },
  "timestamp": "2026-01-28T10:00:00Z"
}
```

### Update Check Request

```json
{
  "instance_id": "siemcore-production",
  "product": "siemcore-api",
  "current_version": "1.4.2",
  "channel": "stable"
}
```

### Update Check Response

```json
{
  "update_available": true,
  "current_version": "2.0.16",
  "latest_version": "2.0.17",
  "download_url": "https://updates.mysoc.ai/api/v1/releases/siemcore/2.0.17/download",
  "update_url": "https://updates.mysoc.ai/api/v1/releases/siemcore/2.0.17/download",
  "sha256": "d6ee561126c8ba6821bb4036332c621a66e41a7b23536b2fa8be42d83dd25d1a",
  "release_notes": "Bug fixes and improvements",
  "channel": "stable",
  "update_group": "stable"
}
```

> **Note:** `download_url` and `update_url` are absolute URLs (include `https://updates.mysoc.ai`).

### File Structure on Instance

```
/opt/siemcore/
├── bin/
│   ├── siemcore-api           # Product binaries
│   ├── siemcore-collector
│   └── siemcore-updater       # The updater itself
├── config/
│   ├── api.yaml               # Product configs
│   └── collector.yaml
├── updater/
│   ├── config.yaml            # Updater configuration
│   ├── versions/
│   │   ├── siemcore-api.version      # Current version files
│   │   └── siemcore-collector.version
│   ├── backups/
│   │   └── siemcore-api.1.4.2.bak    # Rollback backups
│   └── temp/                  # Download staging
├── frontend/                  # Static frontend files
└── data/                      # Application data

/var/log/siemcore-updater/
└── updater.log                # Updater logs
```

---

## Support

For issues with the Updates Server:
- Dashboard: https://updates.mysoc.ai
- Email: support@mysoc.ai

For SiemCore product issues:
- Contact your SiemCore support representative

---

## Document Information

### Version Numbering

This document follows [Semantic Versioning](https://semver.org/):

- **MAJOR.MINOR.PATCH** (e.g., 1.2.3)
- **MAJOR** - Breaking changes or major restructuring
- **MINOR** - New sections or significant additions
- **PATCH** - Fixes, clarifications, minor updates

### Status Definitions

| Status | Description |
|--------|-------------|
| **Draft** | Work in progress, not for production use |
| **Review** | Under review, feedback requested |
| **Production** | Approved for production use |
| **Deprecated** | Superseded by newer version |

### Contributing

To suggest changes to this document:

1. Create a pull request with your proposed changes
2. Update the Revision History table
3. Increment the version number appropriately
4. Update the "Last Updated" date

### Related Documents

| Document | Description |
|----------|-------------|
| [README.md](../README.md) | Project overview |
| [DEPLOYMENT.md](../DEPLOYMENT.md) | Server deployment guide |

---

*Copyright 2026 MySoc. All rights reserved.*
