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
  listen: ":18443"           # child-facing address — the ONE relay port cascade-wide
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
  url: https://mysoc-op1.internal:18443  # the mysoc relay, not updates.mysoc.ai
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
  listen: ":18443"
  cache_dir: /var/lib/siemcore-updater/relay-cache
```

Run the same `relay` subcommand. On its first heartbeat the mysoc relay
issues this node a `relay_token`; the updater persists it in its state file
and presents it as `X-Relay-Token` from then on.

## 4. Tier 3 — swf updater (leaf)

```yaml
server:
  url: https://siemcore-a.customer.lan:18443 # the siemcore relay
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

### Port protection (1.10.0+)

The relay port (`:18443`, the one relay port cascade-wide) is designed to be
opened broadly — children never configure firewalls, and there is no ACL to
maintain anywhere. The listener protects itself, with zero configuration:

- **Auto-learned sources**: IPs that authenticate successfully (credential /
  relay token) get full service. Unknown IPs can only reach the heartbeat
  enrollment path, never release metadata or downloads. Learning is
  in-memory and self-heals after restarts (children re-authenticate every
  heartbeat).
- **Per-IP rate limits**: generous for learned children, strict (5/min) for
  unknown sources.
- **Auth-failure temp-bans**: repeated failures from one IP earn escalating
  bans (5m, 30m, 6h), answered before any request parsing.
- **Bounded state**: per-IP tracking and the child registry are capped, so
  scanners cannot balloon relay memory.

Counters (`blocked`, `rate_limited`, `banned`) roll up in the relay's
heartbeat and appear on the instance page as "Port Protection".

### Identity adoption and decommission (1.11.0+)

Full contract: `docs/RELAY-1.11.0-CONTRACT.md`. In short:

- **Identity adoption**: every heartbeat response carries an `identity`
  object (the relay's own instance id as `parent_instance_id`, plus
  `customer_id`/`customer_name` when configured) so a bootstrap needs only
  the relay host — clients adopt once and echo thereafter. Fields the relay
  does not know are omitted, never sent empty.
- **Decommission**: `POST /api/v1/decommission` (same credentials as a
  heartbeat) marks a child as cleanly removed. The mark reaches the updates
  server as a `decommissioned` child status in the next rollup, renders
  neutrally on the dashboard, survives relay restarts (tombstones in the
  cache dir), is pruned from relay state after 7 days, and is reversed by
  any subsequent genuine heartbeat. It is a state change, never a deletion —
  hard delete stays an admin dashboard action. Direct (tier-1) nodes use the
  same endpoint on the updates server itself.

### Scaling to 20k customers per operator (1.12.0+)

Full design: the Fleet Scalability 1.12 plan. What an operator needs to know:

- **Delta reporting.** A relay set with `relay.delta_reporting: true` stops
  shipping the full O(fleet) rollup every cycle and instead sends **change-only
  deltas**: a per-customer summary and only the leaf rows that changed since the
  parent last acked. Steady-state upward heartbeats stay KB-sized whether a
  relay fronts 50 customers or 20,000. A relay sends the full rollup *or*
  deltas, never both, so the server never double-counts; both paths are
  accepted, so a fleet can transition relay-by-relay.
- **Child cap.** `relay.max_children` bounds the enrolled-child registry. A
  mysoc relay fronting 20k customer relays sets this well above the 10k default
  (e.g. `max_children: 25000`). The port guard's per-IP state cap is sized
  automatically from `max_children` plus headroom, so the mysoc hop's ~20k
  distinct customer-relay source IPs all stay learned rather than being evicted.
- **NAT.** The guard's learned-tier rate bucket scales with the number of
  children observed behind each source IP, so a customer site NATing thousands
  of leaves behind one address is not starved, while a single compromised NAT is
  still capped.
- **Heartbeat interval hint.** `relay.child_heartbeat_interval` advertises a
  preferred child cadence in the heartbeat response (see
  `docs/RELAY-1.12.0-CONTRACT-ADDENDUM.md`). Advisory and opt-in; use it at the
  mysoc hop to flatten the request rate from 20k customer relays.

**Sizing guide (per hop, indicative):**

| Hop | Children | Reporting | `max_children` | Notes |
|---|---|---|---|---|
| mysoc relay | up to 20k siemcore relays | `delta_reporting: true` | `25000` | set `child_heartbeat_interval: 5m` to flatten load |
| siemcore relay | up to ~5k swf leaves | `delta_reporting: true` | default (10k) | one summary per customer, leaves as deltas |
| swf leaf | — | n/a | — | no changes required |

**Acceptance gates** (verify with the load rig below):

- server heartbeat **p99 < 1s** at 20k customers;
- dashboard home and customer directory render **< 2s** at 20k customers
  (both are SQL-aggregated / paged, never O(fleet));
- mysoc relay steady-state CPU/memory documented for the target fleet;
- ingest lag bounded and measured.

**Load rig.** `updater-simulator loadgen` synthesizes N customer subtrees and
drives them at a target as delta-reporting heartbeats, printing per-cycle
throughput and p50/p95/p99 latency. Cycle 1 enrolls the whole fleet; later
cycles carry only the churn (the O(changes) steady state):

```bash
updater-simulator loadgen \
  --target https://mysoc-relay.internal:8443 \
  --customers 20000 --leaves 5 --cycles 3 --concurrency 128 \
  --parent mysoc-op1 --insecure \
  --license-env UPDATER_SIM_LICENSE_KEY
```

That run drives 20k customers / ~120k nodes; read the `p99` line per cycle
against the gate above. Point `--target` at the updates server to measure the
delta-ingest path directly, or at a mysoc relay to measure the full chain.

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
- [Relay 1.12.0 Contract Addendum — Heartbeat Interval Hint](RELAY-1.12.0-CONTRACT-ADDENDUM.md)
