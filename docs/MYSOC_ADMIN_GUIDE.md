# MySoc Updates Server - Admin Guide

| | |
|---|---|
| **Document Version** | 1.0.0 |
| **Last Updated** | February 3, 2026 |
| **Status** | Production |
| **Maintained By** | MySoc Platform Team |

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0.0 | 2026-02-03 | MySoc Team | Initial release - complete admin guide |

---

This guide is for MySoc administrators managing the Updates Server at `updates.mysoc.ai`.

## Table of Contents

1. [Overview](#overview)
2. [Server Architecture](#server-architecture)
3. [Dashboard Access](#dashboard-access)
4. [Managing Instances](#managing-instances)
5. [Managing Releases](#managing-releases)
6. [Managing Licenses](#managing-licenses)
7. [User Management](#user-management)
8. [API Reference](#api-reference)
9. [Server Deployment](#server-deployment)
10. [Troubleshooting](#troubleshooting)
11. [Support](#support)

---

## Overview

The Updates Server (`updates.mysoc.ai`) provides centralized management for all MySoc and SiemCore deployments:

| Feature | Description |
|---------|-------------|
| **Instance Management** | Track all connected instances, their versions, and health status |
| **Release Distribution** | Upload and distribute software releases with staged rollouts |
| **License Management** | Create and manage customer licenses |
| **Heartbeat Monitoring** | Real-time health metrics from all instances |
| **Staged Rollouts** | Control which instances receive updates via target groups |

### Server URL

- **Production**: https://updates.mysoc.ai
- **Dashboard**: https://updates.mysoc.ai (login required)

---

## Server Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     updates.mysoc.ai                             │
│                                                                  │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐       │
│  │   Nginx      │───►│ Update Server│───►│  PostgreSQL  │       │
│  │   (HTTPS)    │    │   (Go API)   │    │  (Database)  │       │
│  └──────────────┘    └──────────────┘    └──────────────┘       │
│         │                                                        │
│         │            ┌──────────────┐                           │
│         └───────────►│  Dashboard   │                           │
│                      │  (Next.js)   │                           │
│                      └──────────────┘                           │
└─────────────────────────────────────────────────────────────────┘
          ▲                    ▲
          │                    │
    ┌─────┴─────┐        ┌─────┴─────┐
    │ SiemCore  │        │  MySoc    │
    │ Instances │        │ Instances │
    └───────────┘        └───────────┘
```

### Components

| Component | Port | Description |
|-----------|------|-------------|
| **Nginx** | 443 | HTTPS termination, static files, reverse proxy |
| **Update Server** | 8080 | Go API server handling all backend logic |
| **Dashboard** | 3000 | Next.js frontend for admin UI |
| **PostgreSQL** | 5432 | Database for instances, releases, licenses, users |

### Services (systemd)

```bash
# Check status
sudo systemctl status update-server
sudo systemctl status dashboard
sudo systemctl status nginx

# Restart services
sudo systemctl restart update-server
sudo systemctl restart dashboard
sudo systemctl reload nginx
```

---

## Dashboard Access

### URL

**https://updates.mysoc.ai**

### Login Credentials

| User | Email | Role |
|------|-------|------|
| System Admin | admin@mysoc.ai | admin |

> **Note:** Contact the platform team for password or to create additional users.

### Dashboard Sections

| Section | Description |
|---------|-------------|
| **Dashboard** | Overview with instance counts and status |
| **Instances** | View all connected instances, configure settings |
| **Releases** | View, upload, edit, and delete releases |
| **Licenses** | Manage customer licenses |
| **Users** | Manage dashboard users (admin only) |
| **Settings** | Server configuration |
| **Security** | Audit logs and security events |

---

## Managing Instances

### Viewing Instances

Navigate to **Instances** to see all connected instances:

- **Instance ID** - Unique identifier set by the updater
- **Display Name** - Friendly name (e.g., `cloud.siemcore.ai`)
- **Status** - Online (heartbeat < 5 min), Offline, Degraded
- **Last Heartbeat** - When the instance last checked in
- **Products** - Installed products and versions
- **System Metrics** - CPU, Memory, Disk usage

### Setting Display Name

1. Click on an instance card
2. Click the **pencil icon** next to the hostname
3. Enter a friendly name (e.g., `cloud.siemcore.ai`)
4. Click the **checkmark** to save

### Configuring Update Settings

On the instance detail page:

#### Auto-Update Toggle

| Setting | Behavior |
|---------|----------|
| **Enabled** | Instance automatically downloads and applies updates |
| **Disabled** | Instance only reports available updates |

#### Update Group

| Group | Purpose |
|-------|---------|
| `alpha` | Internal testing, receives updates first |
| `beta` | Pre-production validation |
| `stable` | Default group for most instances |
| `production` | Customer production systems |

### Deleting Instances

To remove stale or duplicate instances:

1. Click on the instance card
2. Click the red **Delete** button
3. Confirm deletion

> **Note:** The instance will reappear if its updater is still running.

### Instance API

```bash
# List all instances
curl https://updates.mysoc.ai/api/v1/instances

# Get instance details
curl https://updates.mysoc.ai/api/v1/instances/{id}

# Update instance (requires admin auth)
curl -X PUT https://updates.mysoc.ai/api/v1/instances/{id} \
  -H "X-API-Key: YOUR_ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{"display_name": "cloud.siemcore.ai", "update_group": "beta"}'

# Delete instance (requires admin auth)
curl -X DELETE https://updates.mysoc.ai/api/v1/instances/{id} \
  -H "X-API-Key: YOUR_ADMIN_KEY"
```

---

## Managing Releases

### Viewing Releases

Navigate to **Releases** to see all uploaded releases:

- **Product** - Product name (e.g., `siemcore`)
- **Version** - Semantic version (e.g., `2.0.17`)
- **Channel** - Release channel (stable, beta, nightly)
- **Target Groups** - Which instance groups receive this release
- **Size** - Artifact file size
- **Uploaded** - When the release was created

### Uploading Releases

#### Via Dashboard

1. Go to **Releases**
2. Click **Upload Release**
3. Fill in the form:
   - Product: `siemcore`
   - Version: `2.0.18`
   - Channel: `stable`
   - Target Groups: Select groups
   - Release Notes: Description of changes
   - Artifact: Upload the binary file
4. Click **Upload**

#### Via Script

```bash
./scripts/upload-release.sh \
  --product siemcore \
  --version 2.0.18 \
  --file ./bin/siemcore-linux-amd64 \
  --channel stable \
  --groups alpha,beta,stable,production \
  --api-key "YOUR_ADMIN_API_KEY" \
  --notes "Bug fixes and improvements"
```

#### Via cURL

```bash
curl -X POST https://updates.mysoc.ai/api/v1/releases \
  -H "X-API-Key: YOUR_ADMIN_API_KEY" \
  -F "product=siemcore" \
  -F "version=2.0.18" \
  -F "channel=stable" \
  -F "target_groups=alpha,beta,stable,production" \
  -F "release_notes=Bug fixes and improvements" \
  -F "artifact=@./bin/siemcore-linux-amd64"
```

### Editing Releases

1. Go to **Releases**
2. Click the **pencil icon** on a release
3. Modify:
   - **Target Groups** - Add/remove groups
   - **Release Notes** - Update description
4. Click **Save Changes**

Or via API:

```bash
curl -X PUT https://updates.mysoc.ai/api/v1/releases/siemcore/2.0.18 \
  -H "X-API-Key: YOUR_ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "target_groups": ["alpha", "beta", "stable", "production"],
    "release_notes": "Updated release notes"
  }'
```

### Deleting Releases

1. Go to **Releases**
2. Click the **trash icon** on a release
3. Confirm deletion

Or via API:

```bash
curl -X DELETE https://updates.mysoc.ai/api/v1/releases/siemcore/2.0.18 \
  -H "X-API-Key: YOUR_ADMIN_API_KEY"
```

> **Warning:** Deleting a release removes the artifact. Instances will no longer be able to download this version.

### Target Groups - Important

When uploading releases, always include the groups that should receive the update:

| Group | Description |
|-------|-------------|
| `alpha` | Testing instances |
| `beta` | Staging instances |
| `stable` | **Most instances default to this!** |
| `production` | Production instances |

> **Common Issue:** If you forget to include `stable`, most instances won't see the update because they default to `update_group: stable`.

### Version Comparison

The server uses semantic version comparison:
- `2.0.17` > `2.0.2` (compared as numbers, not strings)
- `2.1.0` > `2.0.99`
- Uploading an older version won't cause downgrades

---

## Managing Licenses

### License Format

```
SIEM-XXXX-XXXX-XXXX-XXXX
```

### Creating Licenses

1. Go to **Licenses**
2. Click **Create License**
3. Fill in:
   - Customer Name
   - Products (e.g., siemcore)
   - Expiration Date
4. Click **Create**

### License Fields

| Field | Description |
|-------|-------------|
| **License Key** | Unique identifier (SIEM-XXXX-XXXX-XXXX-XXXX) |
| **Customer** | Customer/organization name |
| **Products** | Licensed products |
| **Expires** | Expiration date |
| **Status** | Active/Inactive/Expired |

### How Licenses Work

1. Instance sends `X-License-Key` header with every heartbeat
2. Server validates:
   - License exists
   - License is active
   - License hasn't expired
   - Products are covered
3. Instance is linked to the license in the database
4. Updates are only available if license is valid

---

## User Management

### Creating Users

Users can be created via the API:

```bash
curl -X POST https://updates.mysoc.ai/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_ADMIN_JWT" \
  -d '{
    "email": "user@mysoc.ai",
    "password": "secure_password",
    "name": "User Name",
    "role": "viewer"
  }'
```

### User Roles

| Role | Permissions |
|------|-------------|
| `admin` | Full access - manage users, releases, instances, licenses |
| `editor` | Manage releases and instances |
| `viewer` | Read-only access to dashboard |

### Authentication Methods

| Method | Used By | Header |
|--------|---------|--------|
| **JWT Token** | Dashboard users | `Authorization: Bearer <token>` |
| **API Key** | Scripts/automation | `X-API-Key: <key>` |
| **License Key** | Instance updaters | `X-License-Key: <key>` |

---

## API Reference

### Admin API Key

The admin API key is required for administrative operations:

```bash
# Get the key from the server
ssh bitnami@updates.mysoc.ai "grep ADMIN_API_KEY /home/bitnami/updates-mysoc-ai/config/.env"
```

Store securely:

```bash
echo "mysoc-admin-key-XXXXXXXX" > keys/ADMIN-API-KEY.txt
chmod 600 keys/ADMIN-API-KEY.txt
```

### Public Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Server health check |
| `/api/v1/releases` | GET | List all releases |
| `/api/v1/instances` | GET | List all instances |

### Instance Endpoints (License Key Auth)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/heartbeat` | POST | Send heartbeat with metrics |
| `/api/v1/updates/{product}/check` | POST | Check for updates |
| `/api/v1/releases/{product}/{version}/download` | GET | Download release |

### Admin Endpoints (API Key Auth)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/releases` | POST | Upload release |
| `/api/v1/releases/{product}/{version}` | PUT | Edit release |
| `/api/v1/releases/{product}/{version}` | DELETE | Delete release |
| `/api/v1/instances/{id}` | PUT | Update instance |
| `/api/v1/instances/{id}` | DELETE | Delete instance |
| `/api/v1/admin/licenses` | GET/POST | Manage licenses |

### Example: Check for Updates

```bash
curl -X POST https://updates.mysoc.ai/api/v1/updates/siemcore/check \
  -H "Content-Type: application/json" \
  -H "X-License-Key: SIEM-XXXX-XXXX-XXXX-XXXX" \
  -d '{
    "instance_id": "siemcore-production",
    "current_version": "2.0.16",
    "channel": "stable"
  }'
```

Response:

```json
{
  "update_available": true,
  "latest_version": "2.0.17",
  "download_url": "https://updates.mysoc.ai/api/v1/releases/siemcore/2.0.17/download",
  "update_url": "https://updates.mysoc.ai/api/v1/releases/siemcore/2.0.17/download",
  "sha256": "d6ee561126c8ba6821bb4036332c621a66e41a7b23536b2fa8be42d83dd25d1a",
  "channel": "stable",
  "update_group": "stable"
}
```

---

## Server Deployment

### Server Location

- **Provider**: AWS Lightsail (eu-west-1)
- **OS**: Ubuntu 22.04 (Bitnami)
- **Domain**: updates.mysoc.ai

### SSH Access

```bash
ssh -i /path/to/LightsailDefaultKey-eu-west-1.pem bitnami@updates.mysoc.ai
```

### File Locations

| Path | Description |
|------|-------------|
| `/home/bitnami/updates-mysoc-ai/` | Application root |
| `/home/bitnami/updates-mysoc-ai/bin/update-server` | Backend binary |
| `/home/bitnami/updates-mysoc-ai/config/.env` | Environment config |
| `/home/bitnami/updates-mysoc-ai/data/releases/` | Release artifacts |
| `/home/bitnami/updates-mysoc-ai/dashboard/` | Dashboard app |
| `/opt/bitnami/nginx/conf/` | Nginx configuration |

### Environment Variables

```bash
# /home/bitnami/updates-mysoc-ai/config/.env
DB_HOST=localhost
DB_PORT=5432
DB_USER=mysoc_admin
DB_PASSWORD=xxxxx
DB_NAME=mysoc_updates
JWT_SECRET=xxxxx
ADMIN_API_KEY=mysoc-admin-key-xxxxx
```

### Deploying Updates

#### Backend

```bash
# Build locally
GOOS=linux GOARCH=amd64 go build -o bin/update-server-linux ./cmd/update-server

# Deploy
scp -i KEY.pem bin/update-server-linux bitnami@updates.mysoc.ai:/tmp/update-server
ssh -i KEY.pem bitnami@updates.mysoc.ai "sudo systemctl stop update-server && sudo cp /tmp/update-server /home/bitnami/updates-mysoc-ai/bin/update-server && sudo systemctl start update-server"
```

#### Dashboard

```bash
# Build locally
cd dashboard && npm run build

# Package
tar -czf /tmp/dashboard-standalone.tar.gz -C .next/standalone/updates-mysoc-ai/dashboard .
tar -czf /tmp/dashboard-static.tar.gz -C .next/static .

# Deploy
scp -i KEY.pem /tmp/dashboard-*.tar.gz bitnami@updates.mysoc.ai:/tmp/
ssh -i KEY.pem bitnami@updates.mysoc.ai "
  sudo systemctl stop dashboard
  cd /home/bitnami/updates-mysoc-ai/dashboard/.next/standalone/updates-mysoc-ai/dashboard
  tar -xzf /tmp/dashboard-standalone.tar.gz
  cd .next/static && tar -xzf /tmp/dashboard-static.tar.gz
  sudo systemctl start dashboard
"
```

### Database

```bash
# Connect to PostgreSQL
ssh -i KEY.pem bitnami@updates.mysoc.ai
source /home/bitnami/updates-mysoc-ai/config/.env
sudo -u postgres psql -d $DB_NAME

# Common queries
SELECT * FROM instances;
SELECT * FROM releases ORDER BY created_at DESC LIMIT 10;
SELECT * FROM licenses;
SELECT * FROM users;
```

### Running Migrations

```bash
ssh -i KEY.pem bitnami@updates.mysoc.ai
sudo -u postgres psql -d mysoc_updates -f /path/to/migration.sql
```

---

## Troubleshooting

### Server Not Responding (502)

1. Check backend service:
   ```bash
   sudo systemctl status update-server
   sudo journalctl -u update-server -n 50
   ```

2. Check nginx:
   ```bash
   sudo systemctl status nginx
   sudo tail -f /opt/bitnami/nginx/logs/error.log
   ```

3. Restart services:
   ```bash
   sudo systemctl restart update-server
   sudo systemctl reload nginx
   ```

### Dashboard Not Loading

1. Check dashboard service:
   ```bash
   sudo systemctl status dashboard
   sudo journalctl -u dashboard -n 50
   ```

2. Check port:
   ```bash
   curl http://localhost:3000
   ```

### Instances Not Appearing

1. Check instance is sending heartbeats
2. Verify license key is valid
3. Check server logs for errors

### Releases Not Showing for Instances

1. Check target groups include `stable` (most common issue)
2. Verify instance's update_group matches release's target_groups
3. Check version comparison (release must be newer)

### Upload Failing (401)

- Verify API key is correct
- API key must be passed via `X-API-Key` header

### Upload Failing (502)

- Large file uploads may timeout
- Server has 5-minute upload timeout
- Check nginx proxy settings

### Viewing Logs

```bash
# Backend logs
sudo journalctl -u update-server -f

# Dashboard logs
sudo journalctl -u dashboard -f

# Nginx access logs
sudo tail -f /opt/bitnami/nginx/logs/access.log

# Nginx error logs
sudo tail -f /opt/bitnami/nginx/logs/error.log
```

---

## Quick Reference

### Common Commands

```bash
# Check all services
sudo systemctl status update-server dashboard nginx

# Restart backend
sudo systemctl restart update-server

# Restart dashboard
sudo systemctl restart dashboard

# Reload nginx
sudo systemctl reload nginx

# View backend logs
sudo journalctl -u update-server -f

# Connect to database
sudo -u postgres psql -d mysoc_updates

# Get admin API key
grep ADMIN_API_KEY /home/bitnami/updates-mysoc-ai/config/.env
```

### API Quick Reference

```bash
# List releases
curl https://updates.mysoc.ai/api/v1/releases | jq .

# List instances
curl https://updates.mysoc.ai/api/v1/instances | jq .

# Upload release
./scripts/upload-release.sh --product siemcore --version X.Y.Z --file ./binary --api-key KEY --groups alpha,beta,stable,production

# Edit release groups
curl -X PUT https://updates.mysoc.ai/api/v1/releases/siemcore/X.Y.Z \
  -H "X-API-Key: KEY" -H "Content-Type: application/json" \
  -d '{"target_groups": ["alpha", "beta", "stable", "production"]}'

# Delete release
curl -X DELETE https://updates.mysoc.ai/api/v1/releases/siemcore/X.Y.Z -H "X-API-Key: KEY"
```

---

## Support

For Updates Server issues:
- Check logs first (see Troubleshooting section)
- Contact: platform@mysoc.ai

---

*Copyright 2026 MySoc. All rights reserved.*
