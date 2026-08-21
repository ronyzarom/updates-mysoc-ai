# Relay Deployment Guide (Cascade Updaters, 1.8.0)

How to run updaters in **relay mode** to build the cascaded distribution
chain:

```
updates.mysoc.ai
    ▲
    │ platform license key
mysoc updater ── relay ──▶ serves siemcore updaters
    ▲
    │ siemcore credential + relay token
siemcore updater ── relay ──▶ serves swf updaters
    ▲
    │ customer credential + relay token
swf updater (leaf)
```

Only the **updaters** participate. No code changes in mysoc, siemcore, or swf
products are required. A relay is the same updater binary with
`relay.enabled: true`; it stays a normal updater toward its parent and
additionally serves the updater API subset to its children.

## 1. Prerequisites

- The operator exists in the dashboard (**Operators** page) and you have its
  platform license key (shown once at creation/rotation).
- The updates server publishes a signing key: `GET https://updates.mysoc.ai/api/v1/signing-key`
  returns `{ "algorithm": "ed25519", "public_key": "<hex>" }`. Pin this value
  in every updater config at provisioning time.
- Network: children must reach their parent relay's listen address. Nothing
  below the mysoc tier needs internet access to `updates.mysoc.ai`.

## 2. Tier 1 — mysoc updater (relay toward siemcore)

`config.yaml`:

```yaml
server:
  url: https://updates.mysoc.ai
  license_key: "MYSOC-XXXX-XXXX-XXXX-XXXX"   # operator platform key

instance:
  id: mysoc-op1
  type: server
  product_tier: mysoc

signing:
  public_key: "<hex public key from /api/v1/signing-key>"
  require: true

relay:
  enabled: true
  listen: ":8443"            # child-facing address
  cache_dir: /var/lib/mysoc-updater/relay-cache
  child_offline_after: 5m

heartbeat:
  interval: 60s
```

Run:

```bash
updater-simulator relay --config config.yaml
```

## 3. Tier 2 — siemcore updater (child of mysoc, relay toward swf)

```yaml
server:
  url: https://mysoc-op1.internal:8443   # the mysoc relay, not updates.mysoc.ai
  ca_file: mysoc-relay-ca.pem            # the mysoc relay's cert.pem
  license_key: "<siemcore instance credential>"

instance:
  id: siemcore-a
  type: server
  product_tier: siemcore
  parent_id: mysoc-op1
  customer_id: acme
  customer_name: "Acme Corp"

signing:
  public_key: "<same pinned hex public key>"
  require: true

relay:
  enabled: true
  listen: ":8443"
  cache_dir: /var/lib/siemcore-updater/relay-cache
```

Run the same `relay` subcommand. On its first heartbeat the mysoc relay
issues this node a `relay_token`; the updater persists it in its state file
and presents it as `X-Relay-Token` from then on.

## 4. Tier 3 — swf updater (leaf)

```yaml
server:
  url: https://siemcore-a.customer.lan:8443  # the siemcore relay
  ca_file: siemcore-relay-ca.pem             # the siemcore relay's cert.pem
  license_key: "<customer credential>"

instance:
  id: swf-pc7
  type: workstation
  product_tier: swf
  parent_id: siemcore-a
  customer_id: acme

signing:
  public_key: "<same pinned hex public key>"
  require: true
```

Leaves run the normal loop (no relay block):

```bash
updater-simulator run --config config.yaml
```

## 5. What a relay does

| Function | Behavior |
| -------- | -------- |
| Child heartbeats | Accepted on `POST /api/v1/heartbeat`. First contact enrolls the child and returns a `relay_token`; later requests must present it. |
| Rollup | Every child (and its children, recursively) is included in this relay's own upward heartbeat as a `children` report. The updates server sees the whole fleet through the mysoc relay alone. |
| Update checks | `POST /api/v1/updates/{product}/check` is forwarded upstream using the relay's credential; download URLs in the offer are rewritten to point at the relay; the release `signature` passes through unchanged. |
| Artifact serving | `GET /api/v1/releases/{product}/{version}/download` is served from a pull-through cache. The relay downloads from its parent once, verifies SHA-256 **and** the ed25519 signature, and only then caches and serves the artifact. |
| Result reports | `POST /api/v1/updates/{product}/report` from children is folded into the rollup so update success/failure is visible centrally. |

Verification happens at **every** hop, so a compromised relay cannot inject a
forged artifact — children re-verify signature and checksum themselves before
installing.

## 6. Operations

- **Rotate a platform key**: dashboard → Operators → Rotate key. Update
  `server.license_key` on the operator's mysoc updater and restart it.
  Children are unaffected (they authenticate to the relay, not the server).
- **Deactivate an operator**: dashboard → Operators → Deactivate. The mysoc
  relay's requests are rejected with `401`, which starves the whole cascade
  of updates within one heartbeat interval.
- **Relay restart**: the artifact cache and child tokens survive restarts via
  the cache directory and state file. A child that lost its token re-enrolls
  on its next heartbeat.
- **Cache hygiene**: the cache holds one verified copy per
  `product/version/filename`, bounded by `relay.max_artifact_bytes` per
  artifact (default 2 GiB). It is safe to delete; the relay re-fetches and
  re-verifies on demand.
- **Monitoring freshness**: the dashboard Fleet page shows `via <relay>` for
  rollup-reported nodes and their last-seen time as observed by the relay.
  A stale subtree usually means the relay itself lost its upstream, not the
  children.

## 7. Related Documents

- [API Contract, Section 9 — Cascade Distribution](API-CONTRACT.md)
- [Updater Guidelines, Section 14 — Cascaded Distribution](UPDATER-GUIDELINES.md)
- [Updater Simulator](UPDATER-SIMULATOR.md)
