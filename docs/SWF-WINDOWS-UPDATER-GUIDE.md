# SWF Windows Updater — Integration Guide (cascade v1.8.4)

Audience: the SWF team building the **Windows updater agent**. You own the
agent's code (Windows service, installer, install/rollback mechanics). This
document is the complete protocol contract your agent must speak. The protocol
is small: five HTTP endpoints, two headers, JSON bodies.

## 1. Where SWF sits in the cascade

```
updates.mysoc.ai  →  mysoc updater (relay)  →  siemcore updater (relay)  →  SWF agent (leaf)
```

Your agent talks to **exactly one endpoint: the siemcore cascade updater's
relay port** on the SIEMCore server that the SWF machine already forwards logs
to. It never contacts `updates.mysoc.ai` or the mysoc platform — no internet
egress, no extra firewall rules beyond reaching the SIEMCore server.

- Base URL: `http://<siemcore-server-address>:18443` (the siemcore team
  confirms the address and port; `18443` is the kit default).
- Transport today is plain HTTP on the customer's internal network. Treat the
  network path accordingly; TLS on the relay port is a coordinated later step.
- Your node appears on the central dashboard automatically through the
  relay's fleet rollup — you do not register anywhere.

## 2. Identity and authentication

Three values, all decided by you at install time and persisted on the device:

| Value | Header / field | Rules |
|---|---|---|
| Instance ID | `instance_id` in every body | Stable and unique per machine, e.g. `swf-<customer>-<hostname>`. Never reuse across machines. |
| Device secret | `X-License-Key` header on **every** request | Self-generated at install (e.g. 32 random hex chars), stored on the device (DPAPI-protected recommended). The relay requires it non-empty; it is your device credential, not a license from us. |
| Relay token | `X-Relay-Token` header | **Issued by the relay** in the first heartbeat response (`relay_token` field, non-empty only on enrollment). Persist it and present it on every later request. Requests for your `instance_id` with a wrong token get `401`. |

Enrollment is trust-on-first-contact: the first heartbeat for a new
`instance_id` enrolls it and returns the token. If the relay restarts and
forgets you, the token you present is re-adopted — keep presenting it.

## 3. The five endpoints

All requests carry `Content-Type: application/json` plus the two headers above.
Errors are `{"error": "<message>"}` with a 4xx/5xx status.

### 3.1 Heartbeat — `POST /api/v1/heartbeat`

Send every **60 seconds**. The server derives liveness from this: silent for
5 minutes reads as offline on the dashboard.

```json
{
  "instance_id": "swf-acme-WS0042",
  "instance_type": "swf",
  "product_tier": "swf",
  "parent_instance_id": "<the siemcore updater's instance id, e.g. siemcore-testing-01>",
  "customer_id": "acme",
  "customer_name": "Acme Corp",
  "hostname": "WS0042",
  "updater_version": "<your agent's own version>",
  "products": [
    { "name": "swf", "version": "2.2.0", "channel": "stable", "status": "running" }
  ],
  "system": {
    "os": "windows",
    "arch": "amd64",
    "cpu_usage": 12.5,
    "memory_total": 17179869184,
    "memory_used": 9663676416,
    "disk_total": 511101108224,
    "disk_used": 214748364800,
    "uptime": 432000
  },
  "timestamp": "2026-08-20T04:30:00Z"
}
```

Response: `{"status":"ok","updates":[],"relay_token":"rt_…"}` —
`relay_token` is non-empty **only on first enrollment**; store it.

Telemetry rules (the dashboard renders only what was measured):

- If you collect host metrics, `memory_total` **must be > 0** — that is the
  marker that measurements are real. Bytes for memory/disk, percent 0–100 for
  `cpu_usage`, `uptime` in whole seconds since boot (host uptime, **not** your
  process uptime).
- If you don't collect metrics yet, send only `"os"` and `"arch"` and leave
  the rest at zero — the dashboard will show "Not reported" instead of fake
  0% readings.
- Omit `license` and `security` entirely. Leaf nodes are covered by the
  operator's platform license upstream; the dashboard knows this.
- After an install attempt, include it in the next heartbeat as
  `"last_update_attempt": {"from_version":"2.2.0","target_version":"2.3.0","success":true,"error":"","timestamp":"…"}`.

### 3.2 Update check — `POST /api/v1/updates/swf/check`

Poll every 5–15 minutes (jittered). The relay forwards the check upstream
under its own credential and rewrites the download URL to itself.

```json
{
  "instance_id": "swf-acme-WS0042",
  "current_version": "2.2.0",
  "updater_version": "<your agent version>",
  "os": "windows",
  "arch": "amd64",
  "hostname": "WS0042",
  "channel": "stable"
}
```

No update: `{"update_available":false,"current_version":"2.2.0","update_group":"stable"}`

Update offered:

```json
{
  "update_available": true,
  "latest_version": "2.3.0",
  "download_url": "/api/v1/releases/swf/2.3.0/download",
  "sha256": "<lowercase hex sha-256 of the artifact>",
  "signature": "<base64 ed25519 signature>",
  "release_notes": "…",
  "channel": "stable",
  "update_group": "stable"
}
```

`download_url` is relative to the relay base URL — download from the relay,
never from anywhere else.

### 3.3 Download — `GET /api/v1/releases/swf/{version}/download`

Headers as always. The relay serves the artifact from its verified
pull-through cache (fetching and verifying from its parent on first request,
so the first download of a new version can take longer). Response headers
repeat `X-Checksum-SHA256` and `X-Signature-Ed25519`.

### 3.4 Release metadata — `GET /api/v1/releases/swf/{version}`

Returns `{"product","version","checksum","signature","size"}`. Optional for a
leaf — the check response already carries checksum and signature — but useful
for re-verifying a previously downloaded artifact.

### 3.5 Install report — `POST /api/v1/updates/swf/report`

Send after **every** install attempt, success or failure. This is how
failures become visible on the central dashboard (via rollup).

```json
{
  "instance_id": "swf-acme-WS0042",
  "from_version": "2.2.0",
  "to_version": "2.3.0",
  "success": false,
  "error": "health check failed after service restart; rolled back"
}
```

## 4. Mandatory verification before install

Every release is signed by the updates server at publish time with ed25519.
**Verify the signature and the checksum before executing anything.** A relay
hop (or anyone on the network path) must not be able to substitute artifacts.

The signed message is this exact byte string (newline-separated, no trailing
newline):

```
mysoc-release-v1\n<product>\n<version>\n<lowercase hex sha-256>
```

For example, for product `swf` version `2.3.0`:
`"mysoc-release-v1\nswf\n2.3.0\nd22bd7fa…"`.

- Signature: base64, ed25519, from the `signature` field / `X-Signature-Ed25519` header.
- Public key (production, pin it in your agent's config — verify out-of-band
  with us, or fetch once from `https://updates.mysoc.ai/api/v1/signing-key`):

```
1f1aa11a80d6ac549a26bb25daac4798c42dd469138680e68f89832bd32e7f57
```

Order of operations: verify the signature over (product, version, checksum
**from the offer**) → download → compute SHA-256 of the downloaded bytes →
compare to the offered checksum → only then install. Reject and report on any
mismatch. .NET note: ed25519 is not in the BCL — use NSec or BouncyCastle.

## 5. Install behavior expected of your agent

The mechanics are yours (MSI, service swap, etc.), but the cascade expects:

1. **Staged install**: download and verify to a staging directory; never
   modify the live install before verification passes.
2. **Health check + rollback**: after swapping, verify SWF is functional; on
   failure roll back to the previous version. Either way, report (3.5) and
   include `last_update_attempt` in the next heartbeat.
3. **Simulate mode first**: ship a config flag that stops short of the actual
   install (download + verify + report only). Pilots run in simulate mode
   until the round-trip is proven — same policy the siemcore tier followed.
4. **Server-side kill switch**: the dashboard's per-instance Auto Update
   toggle gates offers upstream. SWF leaves enrolled through the cascade
   default to **OFF** — during the pilot we flip it on per node, deliberately.
   Your check simply returns `update_available:false` while it's off; no
   special handling needed.

## 6. Publishing SWF releases (your build → the fleet)

Unchanged from today: you build and sign off the SWF executable, and it is
uploaded to `updates.mysoc.ai` as product `swf` with a version and channel by
whoever holds the release-upload key (the updates team, or your existing
upload credential). The cascade handles distribution from there: mysoc and
siemcore relays pull and verify it hop by hop, and your agents receive the
offer from their local relay. Nothing new to build on your side for
distribution.

## 7. Pilot checklist (testing fleet)

1. Pick one Windows machine that can reach the pilot SIEMCore server
   (`siemcore-testing-01`'s host) on port `18443`.
2. Get from the siemcore team: the relay address, and confirmation the port
   is reachable from that machine.
3. Configure your agent: `instance_id` (`swf-<customer>-<hostname>`),
   generated device secret, `parent_instance_id: siemcore-testing-01`,
   `customer_id`/`customer_name` matching what the siemcore node reports,
   pinned signing key, simulate mode ON.
4. Start it. Within ~2 minutes the node must appear on the dashboard under
   the customer, `reported_via` showing the cascade path. That round-trip is
   the pilot's success criterion.
5. We then publish a test SWF release and watch your agent check → verify →
   download → (simulated) install → report, end to end.

Quick smoke test from the Windows machine (curl.exe ships with Windows):

```bat
curl -s http://SIEMCORE-ADDRESS:18443/health
curl -s -X POST http://SIEMCORE-ADDRESS:18443/api/v1/heartbeat ^
  -H "Content-Type: application/json" -H "X-License-Key: <device-secret>" ^
  -d "{\"instance_id\":\"swf-pilot-test\",\"product_tier\":\"swf\",\"hostname\":\"%COMPUTERNAME%\"}"
```

The heartbeat reply's `relay_token` proves enrollment works; the node will
show on the dashboard within a heartbeat cycle. (Delete the test node from
the dashboard afterwards.)

## 8. Field notes

- The relay caps request bodies at 4 MB (heartbeat) / 1 MB (check, report).
- Product and version strings: letters, digits, dots, dashes, underscores.
- Back off on `5xx`/network errors (the relay may be restarting — your
  siemcore peer does the same upstream); resume the normal cadence when it
  recovers. Never skip signature verification because a previous attempt
  verified the same version.
- Your agent's own self-update can ride the same protocol later (product
  `updater-windows-amd64` is reserved for that, mirroring how the Linux
  updaters already update themselves) — out of scope for the pilot.
