# SiemCore Deployment Guide - Updates Server

| | |
|---|---|
| **Document Version** | 2.0.0 |
| **Last Updated** | September 26, 2026 |
| **Status** | Production |
| **Maintained By** | SiemCore Platform Team |

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 2.0.0 | 2026-09-26 | Updates Team | Cascade: SiemCore servers run `siemcore-cascade-updater` and talk only to the operator's mysoc relay; replaced the retired v2 `siemcore-updater` install, commands and paths; credentials, troubleshooting and API reference for the relay path |
| 1.5.0 | 2026-02-03 | SiemCore Team | Added API key requirement, target groups fix (include "stable"), dashboard features (edit/delete), semantic versioning |
| 1.4.0 | 2026-01-31 | SiemCore Team | Added complete update workflow, upload-release.sh script, step-by-step commands |
| 1.3.0 | 2026-01-31 | SiemCore Team | Added deployment policy, removed sensitive keys, clarified channels vs groups |
| 1.2.0 | 2026-01-31 | SiemCore Team | Added quick install script, download links, updated installation instructions |
| 1.1.0 | 2026-01-28 | SiemCore Team | Added SiemCore environments section with server details |
| 1.0.0 | 2026-01-28 | SiemCore Team | Initial release - complete deployment guide |

---

This guide explains how to deploy and manage SiemCore instances using the MySoc Updates Server at `updates.mysoc.ai`.

> **Cascade.** SiemCore servers never connect to `updates.mysoc.ai`. Their
> updater talks only to the SOC operator's mysoc relay, which reaches the
> updates server on their behalf. Releases, update groups and auto-update are
> still managed centrally on the dashboard.

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

Update group and auto-update are set on the instance in the dashboard
(**Instances → Update Settings**), not in the updater config. The updater
config (`/etc/siemcore-cascade-updater/config.yaml`) points only at the
operator's mysoc relay ([Installing the Updater](#installing-the-updater)).

#### Staging (`cloud.siemcore.ai`)

- **Purpose:** Pre-production validation, customer demos
- **Update Group:** `beta`
- **Deployment:** Updates Server only (no deploy-all)
- **Auto-Update:** Enabled
- **Instance ID:** `siemcore-staging`

Set on the dashboard: update group `beta`, auto-update on.

#### Production (`cyfox-il.siemcore.ai`)

- **Purpose:** Live customer environment
- **Update Group:** `production`
- **Deployment:** Updates Server only; requires explicit consent
- **Auto-Update:** Disabled (manual approval required)
- **Instance ID:** `siemcore-production`

Set on the dashboard: update group `production`, auto-update **off**. While
auto-update is off the server withholds product offers from this instance
(updater self-updates still flow). There are no maintenance-window fields;
schedule the consented update by turning auto-update on at the agreed time.

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
updates.mysoc.ai                 update server + dashboard
      ▲  heartbeat + rollup, update checks (operator platform key)
      │
mysoc node (SOC operator)        mysoc-updater, relay :18443
      ▲  heartbeat, check, download (node credential + relay token)
      │
SiemCore server                  siemcore-cascade-updater, relay :18443
      ▲  heartbeat, check, download
      │
SWF forwarders                   leaf updater (Windows)
```

The `siemcore-cascade-updater` systemd service on each SiemCore server:

1. Sends a heartbeat with system metrics to its mysoc relay every 60 seconds;
   the relay rolls it up to `updates.mysoc.ai`.
2. Checks for updates through the relay, which forwards the check with the
   operator's platform key.
3. Downloads artifacts from the relay's verified cache, re-verifies SHA-256
   and the ed25519 signature itself, and applies them through the product's
   `updater/apply` entrypoint ([Update Entrypoint Contract](UPDATE-ENTRYPOINT-CONTRACT.md))
   when auto-update is enabled for the instance.
4. Reports success or failure; results reach the dashboard in the rollup.
5. Serves the relay port to the customer's SWF forwarders, and keeps itself
   updated (product `updater-linux-<arch>`).

> The retired v2 `siemcore-updater` daemon (`/opt/siemcore/bin/siemcore-updater`,
> `/opt/siemcore/updater/config.yaml`) is no longer used and is flagged by v3
> posture audits. Do not install it.

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

Install from the siemcore updater kit (`siemcore-updater-kit-<VERSION>.zip`,
built by `scripts/build-kits.sh`; see [Updater Kit Release Process](UPDATER-RELEASE.md)).
The customer-facing walkthrough is [Self-Service Installation](SELF-SERVICE-INSTALL.md);
the kit's own `README.md` covers every option.

### Prerequisites

- Linux server (amd64 or arm64) with systemd, root access.
- Outbound HTTPS to the operator's mysoc relay (`https://<mysoc-host>:18443`).
  No internet access and no route to `updates.mysoc.ai` are needed.
- From the operator: the relay address, the relay certificate
  (`mysoc-relay-ca.pem`, unless the relay has a public certificate), the
  enrollment credential, and the release-signing public key.

### Install

```bash
unzip siemcore-updater-kit-*.zip && cd siemcore-updater-kit-*
sudo ./install.sh --update \
  --license-key <credential> --parent-url https://<mysoc-host>:18443 \
  --instance-id siemcore-<customer>-01 --parent-id <operator's mysoc instance id> \
  --customer-id <customer> --customer-name "<Customer Name>" \
  --signing-key <hex> --current-version <installed siemcore version> \
  --ca-file ./mysoc-relay-ca.pem     # omit if the relay has a public certificate
sudo systemctl start siemcore-cascade-updater
journalctl -u siemcore-cascade-updater -f
```

Use `--clean` instead of `--update` on a host where SiemCore is not installed
yet. Missing flags are prompted for.

### Configuration

The installer renders `/etc/siemcore-cascade-updater/config.yaml`. The values
that matter:

| Field | Value |
|-------|-------|
| `server.url` | The operator's mysoc relay, `https://<mysoc-host>:18443`. Never `updates.mysoc.ai`. |
| `server.ca_file` | The relay's certificate, unless it serves a public certificate. |
| `server.license_key` | The enrollment credential agreed with the operator. |
| `instance.id` / `instance.parent_id` | This server's id / the operator's mysoc instance id. |
| `instance.customer_id` / `customer_name` | The end customer this server serves (groups the dashboard view). |
| `signing.public_key` | The fleet's release key: `curl -s https://updates.mysoc.ai/api/v1/signing-key` at provisioning time. |
| `products[0].current_version` | The SiemCore version currently installed. |
| `relay.*` | The child-facing port (`:18443`) for this customer's SWF forwarders. |

### After Installation

- Within a minute the log shows `heartbeat accepted`, and the instance appears
  on the dashboard as `via <mysoc relay>`.
- New nodes enroll with **auto-update off** in the `stable` group; set the
  group and auto-update on the dashboard.
- The default executor only downloads, verifies and reports. Real installs
  need the executor block enabled (kit README, "Enabling real installs",
  coordinated with the SiemCore team).

---

## License Key

Two different credentials are involved, at different hops:

| Credential | Held by | Checked by |
|------------|---------|------------|
| Operator platform key (`MYSOC-…`, license type `mysoc-cloud`) | The operator's mysoc updater | `updates.mysoc.ai`, on every heartbeat, update check, report and download. Invalid, deactivated or expired → `401`. |
| Node enrollment credential (`server.license_key` on the SiemCore server) | The siemcore updater | The mysoc relay. It must be present; the node is then bound to the `relay_token` the relay issues on first contact, sent as `X-Relay-Token` from then on. |

A SiemCore server's credential never reaches `updates.mysoc.ai`: the relay
forwards checks and downloads with the operator's platform key. Customer
licenses (types `siemcore` and `siemcore-lite`, keys `SIEM-…`) can also be
created on the dashboard to group a customer's nodes; see the
[License Ownership Guide](LICENSE-OWNERSHIP-GUIDE.md).

### Troubleshooting Credentials

| Symptom | Cause / fix |
|---------|-------------|
| `401` with `relay_token_absent` or `relay_token_mismatch` | The node lost its state file, or another host reuses its instance id. The operator clears the stale enrollment on the relay, then restart the updater. |
| `certificate signed by unknown authority` | `server.ca_file` is missing or wrong; re-copy the relay's `cert.pem`. |
| The whole operator subtree stops updating | The operator's platform key was deactivated, rotated or expired; update `server.license_key` on the mysoc updater. |
| Instance missing from the dashboard | Check `journalctl -u siemcore-cascade-updater`, the relay's reachability (`curl --cacert <ca_file> https://<mysoc-host>:18443/health`), and that the mysoc relay itself is online on the dashboard. |

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
| **Rollback** | A failed apply runs the product's rollback phase inside `updater/apply` ([Update Entrypoint Contract](UPDATE-ENTRYPOINT-CONTRACT.md)). To stop a bad release spreading, remove its target groups. Re-uploading an existing version returns `409`. |
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
  Developer          Updates Server          mysoc relay            SiemCore server
  ─────────          ──────────────          ───────────            ───────────────
      │  1. Upload        │                       │                        │
      │  (target: alpha)  │                       │                        │
      │ ────────────────► │                       │                        │
      │                   │                       │  2. Heartbeat + check  │
      │                   │  forwarded check      │ ◄───────────────────── │ (every 60s)
      │                   │ ◄──────────────────── │                        │
      │                   │  3. Offer (if in      │                        │
      │                   │  group, auto-update)  │                        │
      │                   │ ────────────────────► │  URL rewritten to relay│
      │                   │                       │ ─────────────────────► │
      │                   │  artifact (once,      │  4. Download from      │
      │                   │  verified, cached)    │  relay cache, verify,  │
      │                   │ ────────────────────► │  apply                 │
      │                   │                       │ ◄───────────────────── │
      │                   │  5. Result in rollup  │  report                │
      │                   │ ◄──────────────────── │ ◄───────────────────── │
```

### Step-by-Step Workflow

| Step | Action | Who | Command |
|------|--------|-----|---------|
| 1 | Build binary | Developer | `make build` |
| 2 | Upload to alpha | Developer | `./scripts/upload-release.sh --groups alpha` |
| 3 | Testing auto-updates | Automatic | (heartbeat detects update) |
| 4 | Verify on testing | Developer | Dashboard version + `journalctl -u siemcore-cascade-updater` |
| 5 | Expand to beta | Developer | API call to add beta group |
| 6 | Cloud auto-updates | Automatic | (heartbeat detects update) |
| 7 | Verify on cloud | Developer | Dashboard version + updater log |
| 8 | Expand to production | Developer | API call to add production |
| 9 | **Manual trigger** | Developer | Turn auto-update on for the cyfox-il instance in the dashboard (consent required) |

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

The testing instance's version and last update result appear on its dashboard
page within a heartbeat or two. To watch the updater on the host:

```bash
ssh user@testing.siemcore.ai "sudo journalctl -u siemcore-cascade-updater -f"
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

`cyfox-il.siemcore.ai` has auto-update off, so the server withholds the offer.
With explicit consent, turn auto-update on for that instance in the dashboard
(**Instances → cyfox-il → Update Settings**). The next update check receives
the offer and applies it. Turn auto-update off again afterwards if production
should stay gated.

### Quick Reference Commands

```bash
# Upload release (include stable so default instances see it)
./scripts/upload-release.sh \
  --product siemcore \
  --version 2.0.1 \
  --file ./bin/siemcore-linux-amd64 \
  --groups alpha,beta,stable,production \
  --api-key "$(cat keys/UPDATES-API-KEY.txt)"

# View update logs on a SiemCore server
ssh user@server "sudo journalctl -u siemcore-cascade-updater -f"

# Stop a bad release from spreading: remove its target groups
curl -X PUT https://updates.mysoc.ai/api/v1/releases/siemcore/2.0.1/target-groups \
  -H "X-API-Key: YOUR_KEY" -H "Content-Type: application/json" \
  -d '{"target_groups": []}'

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
   sudo systemctl status siemcore-cascade-updater
   ```

2. **Check logs:**
   ```bash
   sudo journalctl -u siemcore-cascade-updater -f
   ```

3. **Verify the mysoc relay is reachable** (not `updates.mysoc.ai`):
   ```bash
   curl --cacert <server.ca_file> https://<mysoc-host>:18443/health
   ```

4. **Check the updater config:**
   ```bash
   sudo grep -A4 '^server:' /etc/siemcore-cascade-updater/config.yaml
   ```

5. **Check the mysoc relay itself is online** on the dashboard. A whole
   subtree going stale usually means the relay lost its upstream, not the
   SiemCore servers.

### Instance Shows "Offline"

Rollup-reported nodes go offline when their relay stops seeing heartbeats
(`relay.child_offline_after`, default 5 minutes) or the relay stops reporting.

1. Check if the updater service is running
2. Check the network path between the SiemCore server and its mysoc relay
3. Check the mysoc relay's own status on the dashboard
4. Check system resources (CPU/memory exhaustion can prevent heartbeats)

### Updates Not Being Applied

1. **Check auto-update is enabled:**
   - In dashboard: Instance → Update Settings → Auto Update toggle (this is
     the only switch; the server withholds offers while it is off)

2. **Check update group matches:**
   - Instance's `update_group` must be in the release's `target_groups`
   - **Common issue:** Release has `["alpha", "beta", "production"]` but instance has `update_group: stable`
   - **Fix:** Add `stable` to the release's target groups via dashboard or API

3. **Check version comparison:**
   - Update only shows if release version is **higher** than installed version
   - Check the installed version the node reports on its dashboard page

4. **Check the executor is enabled:** the kit's default executor downloads,
   verifies and reports without changing the host (kit README, "Enabling
   real installs").

5. **Check updater logs:**
   ```bash
   sudo journalctl -u siemcore-cascade-updater --since "1 hour ago"
   ```

### Rollback

A failed apply runs the product's rollback phase inside `updater/apply`
([Update Entrypoint Contract](UPDATE-ENTRYPOINT-CONTRACT.md)) and the failure
is reported up the cascade. To stop a bad release reaching more servers,
remove its target groups (`PUT /api/v1/releases/{product}/{version}/target-groups`
with `[]`).

---

## API Reference

### Endpoints Used by Updater

A SiemCore updater calls these on its **mysoc relay** (`https://<mysoc-host>:18443`),
which serves the same paths; the relay calls them on `updates.mysoc.ai` with
the operator's platform key.

| Endpoint | Method | Auth (at the relay) | Description |
|----------|--------|------|-------------|
| `/api/v1/heartbeat` | POST | `X-License-Key` + `X-Relay-Token` | Send instance heartbeat with metrics; first contact returns the `relay_token` |
| `/api/v1/updates/{product}/check` | POST | `X-License-Key` + `X-Relay-Token` | Check for updates; forwarded upstream |
| `/api/v1/releases/{product}/{version}/download` | GET | `X-License-Key` + `X-Relay-Token` | Download from the relay's verified cache |
| `/api/v1/updates/{product}/report` | POST | `X-License-Key` + `X-Relay-Token` | Report update success/failure; carried up in the rollup |

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
| `X-License-Key` | **Updaters** | SiemCore: the node's enrollment credential, checked by the relay. mysoc: the operator's platform key, checked by `updates.mysoc.ai`. |
| `X-Relay-Token` | **Updaters below mysoc** | Issued by the relay on first contact; required on every later request. |
| `X-API-Key` | **Admin/CI only** | Admin or `releases`-scoped API key for release uploads and administration. |

> **Security:** Instances never store or use admin API keys. Every artifact is
> re-verified on the node (SHA-256 and ed25519 signature against the pinned key).

### Heartbeat Payload

```json
{
  "instance_id": "siemcore-production",
  "instance_type": "siemcore",
  "product_tier": "siemcore",
  "parent_instance_id": "mysoc-operator-01",
  "customer_id": "acme",
  "customer_name": "Acme Corp",
  "hostname": "siemcore-prod-01.example.com",
  "updater_version": "1.0.0",
  "config_hash": "abc123",
  "license": {
    "key": "<node credential, masked>",
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
  "download_url": "/api/v1/releases/siemcore/2.0.17/download",
  "update_url": "/api/v1/releases/siemcore/2.0.17/download",
  "sha256": "d6ee561126c8ba6821bb4036332c621a66e41a7b23536b2fa8be42d83dd25d1a",
  "release_notes": "Bug fixes and improvements",
  "channel": "stable",
  "update_group": "stable"
}
```

> **Note:** The relay rewrites `download_url` and `update_url` to paths on
> itself, so a SiemCore server downloads from its mysoc relay, never from
> `updates.mysoc.ai`. The response also carries the release `signature`
> unchanged.

### Updater Files on a SiemCore Server

```
/etc/siemcore-cascade-updater/config.yaml     # Updater configuration
/usr/local/bin/siemcore-cascade-updater       # Symlink to the current binary
/var/lib/siemcore-cascade-updater/
├── state.json                                # Installed versions, relay token
├── artifacts/                                # Downloaded, verified artifacts
├── self-update/                              # Versioned binaries, current symlink
├── relay-cache/                              # Verified artifacts served to SWF children
└── relay-tls/                                # Self-provisioned relay certificate (give cert.pem to SWF)
/etc/systemd/system/siemcore-cascade-updater.service
journalctl -u siemcore-cascade-updater       # Logs
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
