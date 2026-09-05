# MySoc Updates Server — Release Notes

Newest first. Version numbers follow `MAJOR.MINOR.PATCH` with per-candidate
build numbers `MAJOR.MINOR.PATCH.BUILD` (see UPDATER-GUIDELINES §Dev Cycle).

---

## 1.15.0 — Product delivery telemetry (SWF)

Makes "**is SWF actually sending logs?**" answerable from a node's
fleet-dashboard entry. Additive, agent-authored, pure visibility — no command
channel, no host metrics.

### New

- **`products[].telemetry`** — optional delivery-counter object on each product
  in a heartbeat (`ProductTelemetry`: `ready`, `connection`, `sent`, `seen`,
  `admitted`, `delivery_eps_milli`, `last_write_utc`, `spool_events`,
  `spool_bytes`, `status_utc`, `last_error`). First producer: the Secure SWF
  Updater 2.2.0.15 (reads SWF `status.ini` `format_version = 2`, fail-open,
  omits the key when unavailable). Types in `pkg/types/types.go`; contract in
  [Relay 1.15.0 Contract Addendum](RELAY-1.15.0-CONTRACT-ADDENDUM.md) and
  API-CONTRACT §7.3.
- **Rollup preserved** — relays copy the child `products[]` slice into their
  upward rollup, so telemetry rides the cascade with no relay logic change.
  Stored in `last_heartbeat_data` (JSONB); **no migration**.
- **Dashboard** — instance detail Products card shows a delivering / silent /
  stopped signal (from `connection`, `status_utc` freshness, and product
  `status`) plus rate, sent, spool, and `last_error`; `health_status` now
  renders too.

### Deploy ordering (important)

Each relay hop decodes the heartbeat into typed Go structs and re-encodes it, so
a relay or server **older than 1.15.0 silently drops** `telemetry` on the way
up (no error — the panel just stays empty). Roll the **updates server**, the
**mysoc-updater**, and the **siemcore-cascade-updater** to 1.15.0+ **before or
together with** the SWF 2.2.0.15 fleet rollout so the data is visible as soon as
SWF starts sending it. Any mixed combination is safe; an old hop only hides the
panel.

---

## 1.7.0 — Operator / Reseller / Customer license ownership

**Build:** 1.7.0.3 · **Status:** candidate, pending deployment

Aligns the platform with the real sales flow: a **SOC operator** buys the
mysoc platform and sells siemcore+swf to end **customers**, directly or via a
**reseller**. One mysoc installation serves all of an operator's customers.

### New

- **License ownership fields** — `operator_id`, `reseller_id`,
  `reseller_name` on licenses (DB migration `011_license_ownership`). A
  *platform* license (`type = mysoc-cloud`, or `operator_id` equal to its own
  `customer_id`) defines an operator; every other license is a customer
  license grouped under its operator. Resellers are sales metadata only.
- **Cross-license parent resolution** — a customer's siemcore that declares
  the operator's mysoc as its parent now links correctly instead of being
  flagged as an orphan. `orphan` now strictly means "declared parent is not
  enrolled anywhere in the fleet".
- **Operator-grouped tree** — `GET /api/v1/instances/tree` returns
  `operator → customer → siemcore → swf`, with the operator's mysoc nodes as
  `platform_roots`. Customer licenses appear even before any instance enrolls.
- **Dashboard**
  - Create License is scope-aware: **Customer (siemcore + swf)** or
    **SOC Operator (mysoc platform)**, with an operator dropdown, optional
    reseller fields, and an `swf` product chip.
  - Instances → Tree view renders the operator grouping with reseller badges.
  - License detail gains an admin-only **Ownership** card to set
    operator/reseller (use it to classify pre-1.7.0 licenses).
- **Docs** — new `LICENSE-OWNERSHIP-GUIDE.md`; API contract and updater
  guidelines updated for the ownership model.

### Breaking

- **`GET /api/v1/instances/tree` response shape changed** from
  `{ "customers": [...] }` to `{ "operators": [...] }` (see API-CONTRACT
  §5.8). The bundled dashboard ships in lockstep; only external consumers of
  this endpoint need to adapt.

### Unchanged

- **Agent wire protocol** — heartbeats/update checks still send
  `product_tier` + `parent_instance_id`; existing siemcore/swf configs keep
  working. No updater-kit code changes; nothing for the SWF team to do.

### Upgrade notes

1. Apply migration `011_license_ownership.up.sql` (additive, idempotent;
   run as the `licenses` table owner).
2. Deploy the server binary and dashboard build together.
3. Optionally: create the operator's platform license, then set
   `operator_id` on existing customer licenses via the Ownership card.
   Until then they render under the *Unassigned* group — functionally harmless.

---

## 1.6.0 — Product hierarchy (mysoc > siemcore > swf)

**Build:** 1.6.0.1 · **Deployed:** 2026-08-18

- Canonical three-tier product catalog (`GET /api/v1/products`).
- Agents self-report `product_tier` and `parent_instance_id` on heartbeat and
  update-check; server validates tier and parent adjacency, accepts children
  that enroll before their parent (orphans). Migration `010`.
- First fleet tree endpoint (`GET /api/v1/instances/tree`) and dashboard tree
  view with tier filter; hierarchy fields in the updater kit config
  (`product_tier`, `parent_id`) with example configs for all three tiers.

## 1.5.0 — Managed API keys & credential hygiene

**Build:** 1.5.0.2 · **Deployed:** 2026-08-18 (with the 1.6.0.1 rollout)

- Dashboard-managed, scoped, revocable API keys (`releases` / `admin` scopes),
  hash-only storage, one-time plaintext reveal. Migration `009`. Lets external
  teams (e.g. SWF) upload releases without the master `ADMIN_API_KEY`.
- Client-side credential hygiene in the updater kit: trims whitespace and
  rejects quoted license/API keys with a clear error; server matching stays
  exact.

## 1.4.0 — IP allowlist & filesystem executor

**Build:** 1.4.0.1 · **Deployed:** 2026-08-11

- Server-side IP allowlist for the updater channel (observe/enforce modes,
  admin CRUD, `last_ip_address` tracking). Migration `008`.
- Filesystem install executor in the updater simulator: real artifact
  download, checksum verification, staged install with rollback.

## 1.3.0 — Desired-state reconcile model

**Deployed:** 2026-01 (see git history)

- Updater simulator gained the full reconcile pipeline: system-template
  manifest, ordered plan (migrate → containers → config → self-update),
  health/security stages, failure rollback, and state persistence.

## 1.2.0 — Dashboard and API repair

**Deployed:** 2026-01 (see git history)

- Comprehensive dashboard/API hardening: auth flows, error states,
  accessibility and responsive fixes, version stamping via build flags.
