# SiemCore Deployment Guide - Updates Server

| | |
|---|---|
| **Document Version** | 2.1.0 |
| **Last Updated** | September 26, 2026 |
| **Status** | Production |
| **Maintained By** | SiemCore Platform Team |

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 2.1.0 | 2026-09-26 | Updates Team | SiemCore review: four live rings (alpha incl. bench, beta cloud, stable = customer hosts seetech/danshar, production cyfox-il) with live-registry check; promotion by widening target_groups with approval and soak; first upload alpha only (never "include stable"); releases-scoped upload key; universal bundle artifact and 4-part versions; forward-only rollback; withdrawing does not roll back applied hosts; no deploy-all |
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
| **All upgrades through the cascade** | Every SiemCore host, testing included, is upgraded by publishing to the Updates Server. `deploy-all.sh` is not used; `deploy.sh` is greenfield bring-up only. |
| **Promotion needs approval** | Widening a release beyond `alpha` (to `beta`, `stable`, `production`) requires approval and a soak at each step. Production requires **explicit consent**. |
| **Preserve holds** | Instances held with auto-update off (for example the bench host) stay held until Rony removes the hold. |
| **Pods are regular updates** | Pod nodes (A, B, witness, observer) receive releases through the normal cascade like any other host, by ring and auto-update. Pod-specific handling must never change how a normal (standalone) server updates. |
| **SSH is break-glass only** | SSH to customer hosts is for verification and emergency troubleshooting only. Host-local `update.sh --version` is break-glass for when the cascade cannot deliver. |

---

## SiemCore Environments

SiemCore hosts are assigned to four rollout rings (update groups). Ring
membership and auto-update policy change over time: **always check the live
registry on the dashboard (Instances) before publishing or promoting.** The
snapshot below is from 2026-09-26 and only shows the shape:

| Ring | Hosts (instance id) | Notes |
|------|---------------------|-------|
| `alpha` | `siemcore-testing-01`, `siemcore-bench-20260912-01`, Bezeq pod test nodes, other qualification nodes | Not testing alone: bench is in alpha. Bench is held (auto-update off). Pod nodes update like any other alpha host. |
| `beta` | `siemcore-cloud-01` (cloud.siemcore.ai) | Check its live auto-update policy. |
| `stable` | `siemcore-seetech`, `siemcore-danshar` | Two **customer** hosts. |
| `production` | `siemcore-cyfox-il` (cyfox-il.siemcore.ai) | Live customer environment; explicit consent. Check its live auto-update policy. |

Update group and auto-update are set per instance on the dashboard
(**Instances → Update Settings**), not in the updater config. The updater
config (`/etc/siemcore-cascade-updater/config.yaml`) points only at the
operator's mysoc relay ([Installing the Updater](#installing-the-updater)).

While an instance's auto-update is off, the server withholds product offers
from it (updater self-updates still flow). There are no maintenance-window
fields.

> **Security Note:** Never commit keys to documentation or repositories.

### Recommended Rollout Flow

```
 upload           approve + soak     approve + soak     explicit consent
target_groups ──► add beta ────────► add stable ──────► add production
  = [alpha]       (cloud)            (seetech, danshar) (cyfox-il)
```

Widening `target_groups` **is** the promotion act. Instances in the added ring
receive the offer on their next check if their auto-update is on. Turning
auto-update on is an extra gate only for an instance whose live policy is off;
it is not the standing trigger for production.

### Setting Update Groups

In the dashboard (https://updates.mysoc.ai/instances):

1. Click on an instance
2. Under **Update Settings**, select the ring (see the table above)
3. Click **Save**

The equivalent API (`PUT /api/v1/instances/{id}/update-group` and
`/auto-update`) requires admin authentication; ring assignment is an
administrator action, not something the release-upload key can do.

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
| `alpha` | Internal testing and qualification, receives releases first | `siemcore-testing-01`, bench |
| `beta` | Pre-production validation | `siemcore-cloud-01` |
| `stable` | Customer hosts | `siemcore-seetech`, `siemcore-danshar` |
| `production` | Live customer environment, explicit consent | `siemcore-cyfox-il` |

New nodes enroll in `stable` with auto-update off. `stable` holds customer
hosts, so move a new node to its intended ring before turning auto-update on.

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
| **Update Group** | Which instances receive the release (rollout ring) | `alpha`, `beta`, `stable`, `production` |

- **Channel** = "What kind of build is this?" (stable releases vs experimental)
- **Update Group** = "Who gets this release?" (internal → pre-prod → customers)

Most instances use `channel: stable` but belong to different update groups.

---

## Staged Rollouts

Staged rollouts let you control which instances receive updates by targeting specific update groups.

### How It Works

1. Upload a new release with `target_groups=alpha` only
2. Only instances in the `alpha` ring receive it
3. After approval and a soak, add `beta`
4. After approval and a soak, add `stable` (customer hosts)
5. With explicit consent, add `production`

### Rollout Strategy Example

```
Upload:     target_groups: [alpha]
Promote 1:  target_groups: [alpha, beta]                     ← approval + soak
Promote 2:  target_groups: [alpha, beta, stable]             ← approval + soak
Promote 3:  target_groups: [alpha, beta, stable, production] ← explicit consent
```

> **Important:** Never add `stable` on the first upload: `stable` is the
> customer ring (seetech, danshar). And never upload without `target_groups`:
> an omitted or empty list on upload defaults to **all four rings**.

### Checking Update Availability

An instance receives an update only if:

1. ✅ `auto_update_enabled` is `true` for the instance
2. ✅ Instance's `update_group` is in the release's `target_groups`
3. ✅ Release version is newer than installed version

---

## Release Rules

| Rule | Description |
|------|-------------|
| **Immutability** | Releases are immutable once published; re-uploading an existing version returns `409`. Every rebuilt candidate gets a new build number. |
| **Rollback** | A failed apply runs the rollback phase of SiemCore's `updater/apply` ([Update Entrypoint Contract](UPDATE-ENTRYPOINT-CONTRACT.md)): the executor flips the `current` symlink back and re-runs the wrapper. It is **forward-only**: no down-migration and no database restore; the previous binary runs against the schema the failed release already migrated (rollback-compat is set on the rollback phase only). |
| **Stopping a bad release** | Setting its target groups to `[]` stops new offers. It does **not** roll back hosts that already applied it successfully; they stay on it until a newer build is published. |
| **Schema changes** | Database/schema changes must be backward-compatible (expand/contract pattern), because rollback never down-migrates. |
| **Version format** | `MAJOR.MINOR.PATCH.BUILD` (e.g., `3.3.152.57`) |
| **Version comparison** | The server compares every component numerically and offers the **highest** version; uploading an older version never causes a downgrade. |

---

## Uploading Releases

### Upload Key

Upload with the SiemCore **releases-scoped** key
(`siemcore-team-release-upload`), kept in the SiemCore workspace at
`keys/SIEMCORE-RELEASE-UPLOAD-KEY.txt`. Never upload with the server's master
admin key. The releases scope is not limited to the `siemcore` product on the
server, so only touch `siemcore` releases.

### The Artifact

The release artifact is the universal bundle
`siemcore-universal-<MAJOR.MINOR.PATCH.BUILD>.tar.gz`, uploaded as
`product=siemcore`, `channel=stable`. Not a raw Linux binary.

### Using cURL

```bash
KEY=$(tr -d '[:space:]' < keys/SIEMCORE-RELEASE-UPLOAD-KEY.txt)
curl -fsS -X POST https://updates.mysoc.ai/api/v1/releases \
  -H "X-API-Key: $KEY" \
  -F "product=siemcore" \
  -F "version=3.3.152.57" \
  -F "channel=stable" \
  -F "target_groups=alpha" \
  -F "release_notes=…" \
  -F "artifact=@dist/siemcore-universal-3.3.152.57.tar.gz"
```

### Using the Upload Script

`scripts/upload-release.sh` (in the updates-mysoc-ai repo) works too, but
**always pass `--groups alpha`**: without `--groups` it sends no target
groups, which the server treats as all four rings.

```bash
./scripts/upload-release.sh --product siemcore --version 3.3.152.57 \
  --file dist/siemcore-universal-3.3.152.57.tar.gz \
  --groups alpha --api-key "$KEY" --notes "…"
```

### Promoting an Existing Release

After approval and a soak, widen the target groups (see
[Staged Rollouts](#staged-rollouts)):

```bash
curl -fsS -X PUT https://updates.mysoc.ai/api/v1/releases/siemcore/3.3.152.57/target-groups \
  -H "X-API-Key: $KEY" \
  -H "Content-Type: application/json" \
  -d '{"target_groups": ["alpha", "beta"]}'
```

### Editing Releases via Dashboard

You can also edit releases directly in the dashboard:

1. Go to **Releases** → Click the **pencil icon** on any release
2. Modify **Target Groups** or **Release Notes**
3. Click **Save Changes**

### Deleting Releases

Prefer **withdrawing** a bad release (target groups `[]`) over deleting it:
withdrawal keeps the record and the audit trail. Deleting removes the release
from the database and is an administrator decision:

1. Go to **Releases** → Click the **trash icon** on the release
2. Confirm deletion in the popup

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
| 1 | Build the universal bundle | SiemCore | `siemcore-universal-<version>.tar.gz` |
| 2 | Upload to alpha only | SiemCore | `target_groups=alpha` with the releases-scoped key |
| 3 | Alpha hosts with auto-update on apply it | Automatic | (next update check) |
| 4 | Verify alpha, soak | SiemCore | Dashboard version and last result, updater log |
| 5 | Promote to beta | Approval | Add `beta` to target groups |
| 6 | Verify cloud, soak | SiemCore | Dashboard version and last result |
| 7 | Promote to stable | Approval | Add `stable` (seetech, danshar) |
| 8 | Verify customer hosts, soak | SiemCore | Dashboard |
| 9 | Promote to production | **Explicit consent** | Add `production` (cyfox-il); if that instance's live auto-update is off, turning it on is the extra gate |

### Upload

See [Uploading Releases](#uploading-releases): universal bundle, releases-scoped
key, `target_groups=alpha` only.

### Verify

Each host's version and last update result appear on its dashboard page within
a heartbeat or two. To watch the updater on a host:

```bash
ssh user@testing.siemcore.ai "sudo journalctl -u siemcore-cascade-updater -f"
```

### Promote

```bash
KEY=$(tr -d '[:space:]' < keys/SIEMCORE-RELEASE-UPLOAD-KEY.txt)
# after approval + soak: add beta, later stable, finally production (consent)
curl -fsS -X PUT https://updates.mysoc.ai/api/v1/releases/siemcore/3.3.152.57/target-groups \
  -H "X-API-Key: $KEY" -H "Content-Type: application/json" \
  -d '{"target_groups": ["alpha", "beta"]}'
```

### Withdraw

```bash
# stops new offers; hosts that already applied it stay on it until a newer build
curl -fsS -X PUT https://updates.mysoc.ai/api/v1/releases/siemcore/3.3.152.57/target-groups \
  -H "X-API-Key: $KEY" -H "Content-Type: application/json" \
  -d '{"target_groups": []}'
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
   - **Common issue:** a new node still sits in `stable` (the enrollment
     default). Move it to its intended ring on the dashboard; do not add
     `stable` to a release to reach it, because `stable` is the customer ring.

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
  "instance_id": "siemcore-cyfox-il",
  "product": "siemcore",
  "current_version": "3.3.152.56",
  "channel": "stable"
}
```

### Update Check Response

```json
{
  "update_available": true,
  "current_version": "3.3.152.56",
  "latest_version": "3.3.152.57",
  "download_url": "/api/v1/releases/siemcore/3.3.152.57/download",
  "update_url": "/api/v1/releases/siemcore/3.3.152.57/download",
  "sha256": "d6ee561126c8ba6821bb4036332c621a66e41a7b23536b2fa8be42d83dd25d1a",
  "release_notes": "…",
  "channel": "stable",
  "update_group": "production"
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
