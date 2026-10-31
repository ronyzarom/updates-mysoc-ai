# MySoc Updates Server — Release Notes

Newest first. Version numbers follow `MAJOR.MINOR.PATCH` with per-candidate
build numbers `MAJOR.MINOR.PATCH.BUILD` (see UPDATER-GUIDELINES §Dev Cycle).
Every entry states the build commit and tag, the ring(s) it is on, the soak
result before any widening, and a **Relay compatibility** block (what an older
relay or server drops, minimum hop version if any, child-facing changes) —
see the Relay section of [UPDATER-RELEASE.md](UPDATER-RELEASE.md).

---

## 1.18.0 — Updater survivability: self-update always works, no deadlocks

**Build:** 1.18.0.2 on `version/1.18.0` · **Status:** in local validation; not
yet published to any ring. Not merged to `main`, not tagged. (`1.18.0.1` was the
Phase 1 binary-only checkpoint; `1.18.0.2` adds the Phase 2 kit — the systemd
watchdog pinger, `Type=notify`/`WatchdogSec`, and the root restore backstop —
so the binary changed and the build number was incremented per the dev cycle.)

**Theme:** make the updater's own self-update unconditional, bounded, and
self-healing so a node can always pull itself forward and can never wedge the
fleet. Eight design invariants (see [UPDATER-RELEASE.md](UPDATER-RELEASE.md)
§Survivability):

1. Self-update runs **first** each cycle, in isolation, with its own deadline
   (`self_update.timeout`, 10m). A heartbeat failure is recorded, never fatal;
   each product is bounded by `product_timeout`, the whole cycle by
   `cycle_timeout`, so a stuck product never starves self-update or later ticks.
2. **Prove-it-boots:** a new read-only `preflight --config --state` subcommand;
   the staged candidate must pass `version` AND `preflight` against the host's
   real config/state before the symlink swap.
3. **Bounded downloads:** a separate download client with no overall timeout, a
   `download_timeout` context deadline (30m) and a no-progress stall guard
   (`download_stall_timeout`, 2m); `server.timeout` now bounds only metadata.
4. **Key rollover:** `signing.public_keys` — a signature valid under any trusted
   key is accepted, so rotating the signing key never strands a node.
5. **Fallback parent:** `server.fallback_url` is used for the self-update
   check/download after the parent fails (tier-1 only; a relay warns).
6. **Compatibility is a build-time test:** a golden `testdata/compat/1.17.1.1/`
   config+state must load unchanged under 1.18.0.
7. **No data backup/restore, no outage beyond the restart window** (Entrypoint
   Contract v1.4, proposed): the updater measures `unavailable_seconds` and the
   server refuses to widen a release past alpha when an alpha node reported an
   `availability_violation`, unless an audited `?override=`. A free-disk
   preflight skips (not fails) an install that will not fit.
8. **The relay is the backbone:** a self-update offer is forwarded
   field-for-field (including unknown future fields) so it survives any relay
   hop; a relay drains children before a self-update exit (503 + Retry-After,
   in-flight downloads finish); its child-facing listener health gates the
   systemd watchdog; its cache self-heals corrupt artifacts at start; and
   `parent_rejected_credential` surfaces an expired/rotated license upward.

Observability is additive: the heartbeat gains `self_update{capable,reason,
current_version,last_result}`, `relay_listener_ok`, and
`parent_rejected_credential`; state keeps `last_self_update`. `sd_notify`
READY/WATCHDOG is wired (inert without a notify unit) ahead of the Phase 2 kit.

**Relay compatibility:** additive only. New heartbeat/report fields
(`self_update`, `relay_listener_ok`, `parent_rejected_credential`,
`unavailable_seconds`, `availability_violation`) are ignored by an older server
or relay. The self-update offer field set is frozen and forwarded verbatim, so a
1.18.0 offer traverses an older relay unchanged and vice versa. Minimum
relay/server version: unchanged. `self_update.timeout` and the new
`simulation`/`relay`/`filesystem` bounds all default when absent, so a
pre-1.18.0 config runs unchanged (proven by the golden compat test).

### Added

- `preflight` subcommand; `signing.public_keys`; `server.fallback_url` +
  `server.download_timeout`/`download_stall_timeout`;
  `simulation.cycle_timeout`/`product_timeout`;
  `filesystem.max_unavailable_seconds`; `relay.drain_timeout`.
- Server: streaming version-stamp guard rejects a mislabelled `updater-*`
  artifact upload; promotion gate refuses to widen a flagged release past alpha
  without an audited override.

---

## 1.17.1 — Restore `signature` in the staged release receipt

**Build:** 1.17.1.1 on `version/1.17.1`, commit `e860959`, tag `v1.17.1.1`,
merged to `main` · **Status:** `alpha` since 2026-10-31 05:43Z; `beta`,
`stable`, `production` remain on 1.17.0.2 pending soak and per-ring approval.

**Alpha soak (2026-10-31):** testing.siemcore.ai, bezeq-pod-test-a and -b
self-updated 1.17.0.2 → 1.17.1.1 on their next cycle, re-staged siemcore
3.3.175.9 with `signature` present, applied it and passed its 5-minute health
gate (siemcore confirmation 05:51Z). The pod observer has not taken it: its
updater cycle is wedged on a retained observer operation that predates this
release (open with siemcore). Relay check: testing.mysoc.ai (parent of the pod)
restarted for its own self-update; pod-b logged one transient connection
refused at 05:43:19Z and recovered on the next cycle.

**Relay compatibility:** none. No new wire fields; child-facing behaviour
unchanged. The restored `signature` is in the on-disk receipt, not on the
wire; relays never saw a difference. Minimum relay/server version: unchanged.

The filesystem executor again records the verified origin `signature` in each
`.updater-release.json` it stages. A privileged root consumer (siemcore's
recovery runtime / `greenfield-hook.py`) reads that receipt and re-verifies the
checksum and signature against its pinned fleet key before it will apply a
release; with the field missing it rejects the apply (`unapproved release` /
`signed_release_not_provisioned`) and auto-rolls-back.

The field had been added on `version/1.16.1` (commit `1d658e1`) and on two
`codex/*` branches (commit `c6aa171`), but neither reached `main`, so the
`version/1.17.0` line that became updater `1.17.0.2` — promoted to all rings —
wrote receipts without it. This caused siemcore `3.3.175.9` to fail to apply on
every alpha host that had self-updated to `1.17.0.2`.

### Fixed

- **`releaseMetadata.signature` restored** — `Apply` writes `ArtifactSignature`
  (carried from the offer through `processOffer`) into the receipt, omitted when
  empty so an unsigned/dev build stays distinguishable. Staged metadata remains
  untrusted input: privileged consumers must verify it against their own key.
- Tests: receipt retains the signature across apply and rollback; an empty
  signature is omitted; and a signed offer produces a receipt a root consumer
  can verify end to end (`receipt_signature_test.go`).

---

## 1.17.0 — Fresh-host one-command install (`first_install`)

**Build:** 1.17.0.3 on `version/1.17.0`, merged to `main` · **Status:** deployed.
Updater self-update artifact is **1.17.0.2** (build commit `4b19ca4`,
2026-10-28; tag `v1.17.0.2` to be backfilled), on all rings. 1.17.0.3 is the
kit (install.sh `--clean` fix) at `/install/`; its binary is the same source as
1.17.0.2 plus no updater code change.

**Known regression (fixed in 1.17.1.1):** 1.17.0.2 writes `.updater-release.json`
without `signature`, which makes siemcore's root recovery runtime reject every
apply staged by it. Hosts on 1.17.0.2 cannot apply a SiemCore release until
they self-update.

**Relay compatibility:** none for the updater artifact. The kit/installer
changes are host-local.
— `/install/` serves kit 1.17.0.3 (reinstall fix), empty-VM e2e verified; updater
self-update `1.17.0.2` promoted to **all rings**, verified self-update on
testing.mysoc.ai (alpha), mysoc-cloud (beta) and the cyfox-il production server
(each `1.16.1.x → 1.17.0.2`, staying healthy).

The updater can now bring up a brand-new host from nothing. On a box with no
app, no database and no prior version, one command enrols the updater and the
cascade self-provisions the whole server — dependencies, database baseline,
`.env`, TLS, systemd units, nginx, code and a health gate — then lands on a
first-run `/setup` page. The same command installs a normal server or a DR node;
DR differs only afterward, at the dashboard role step. Verified end to end on an
empty Ubuntu 24.04 VM (see
[fresh-host one-command verification](verification/fresh-host-one-command.md)).

### New

- **`first_install` lifecycle phase** — when a node's current version is
  `0.0.0`, the updater offers the highest release on its ring and runs the
  artifact's entrypoint with `UPDATER_PHASE=first_install`. The entrypoint starts
  with a **read-only preflight** (no mutation) that either passes or refuses with
  a **classified exit code** (10 seed · 11 OS/arch · 12 network · 13 DNS/ports ·
  14 TLS · 15 PostgreSQL · 16 disk · 17 preflight-internal · 1 other). On any
  failure the updater does **not** roll back: it cleans up the staged release and
  **backs off (5m → 15m → 60m), then resumes automatically** on the next cycle
  until the host is healthy. `first_install` has its own timeout
  (`first_install_timeout`, default 30m), independent of normal updates.
- **One-command fresh-host enrolment** — a hosted bootstrap plus a versioned kit:

  ```bash
  curl -fsSL https://updates.mysoc.ai/install/mysoc-enroll.sh | sudo bash -s -- \
    --license-key <KEY> --domain <fqdn> --tls letsencrypt --email <ops@…>
  ```

  `mysoc-enroll.sh` writes a `0600` seed (domain, TLS mode/paths, SMTP, SiemCore
  URL, admin email), fetches the checksum-verified kit, installs the updater with
  the filesystem executor, starts it, and waits for `first_install` to report
  `DONE`. Hosted at `https://updates.mysoc.ai/install/` (served by a deploy-gated
  nginx `location ^~ /install/`).
- **Filesystem executor wiring in the kit** — `install.sh --executor filesystem`
  installs the root apply shim `/usr/local/sbin/mysoc-apply-update` (passes the
  phase through to the artifact entrypoint), a sudoers drop-in, a unit drop-in
  (`NoNewPrivileges=no`, `ProtectSystem=no`), and the staging root
  `/opt/mysoc-cascade` with atomic symlink swaps.
- **Entrypoint contract v1.3** — adds the `first_install` phase, the
  `UPDATER_PHASE` env value, and the classified exit-code table
  ([UPDATE-ENTRYPOINT-CONTRACT](UPDATE-ENTRYPOINT-CONTRACT.md)).

### Changed

- **Config directory permissions fix** — `/etc/mysoc-updater` is now created
  `0750 root:<service-user>` (was inheriting `0700 root:root` from the seed
  write's `umask`), so the unprivileged updater can traverse to read its `0640`
  config. The seed write in `mysoc-enroll.sh` runs under a **scoped** `umask`
  subshell so it no longer leaks into the kit install. (Found and fixed during
  the live e2e.)
- Self-update artifacts are published per platform as `updater-linux-amd64` /
  `updater-linux-arm64`; the shim refuses `first_install` for entrypoint-less
  releases rather than guessing.
- **Reinstall fix (1.17.0.3)** — `install.sh --clean` now clears a stale
  `/var/lib/<name>/state.json` so a re-enrol on a host that still carries updater
  state restarts at `0.0.0` and re-runs `first_install`. Previously the persisted
  `product_versions` out-ranked the config's `current_version: "0.0.0"`, so a
  reinstall reported "product is up to date" and silently skipped `first_install`.
  (A genuinely fresh VM has no updater, so this only affected reinstall/wipe of a
  previously-enrolled host.)

### Upgrade and rollback

Fresh hosts receive the kit updater directly from `/install/` (kit `1.17.0.3`).
Existing hosts self-update through their ring: `updater-linux-<arch>` `1.17.0.2`
is now on **all rings** (alpha → beta → stable → production, staged), verified by
clean self-updates on testing.mysoc.ai (alpha), mysoc-cloud (beta) and the
cyfox-il production server — each `1.16.1.x → 1.17.0.2` in ~40–80 s, staying
online. Updaters never downgrade, so a fresh kit host (stamped `1.17.0.3`) stays
on it even though the self-update product is `1.17.0.2` (same code, different
build stamp). No server-side schema change. To retract, re-point a ring to the
prior `1.16.1.x` build — already-upgraded hosts will not downgrade.

---

## 1.16.2 — Issuer sealing (verify and badge, never reject)

**Build:** 1.16.2.3 on `version/1.16.2`, tag `v1.16.2.3` · **Status:** merged to `main`; first fleet-wide binary containing it is updater 1.17.0.2 (no standalone 1.16.2 rollout).

Each issuer can now seal its own releases. The server checks the seal on
upload and shows the result; nothing is rejected in this phase and customer
updaters are unchanged, because every issuer seals with today's release key.

### New

- **Issuer seal on upload** — optional `issuer_signature` and
  `issuer_key_id` on `POST /api/v1/releases`, over the existing
  `mysoc-release-v1` message. Recorded as `seal_status`: `sealed`,
  `unsealed`, `invalid` (logged as `ALERT issuer-seal`) or `unknown_key`.
  A verified seal becomes the release signature; every other upload is still
  signed by the server.
- **Trusted issuer keys** — `trusted_keys` registry with admin API and a
  Settings card. The server registers its release key on first start; up to
  two active keys per issuer scope for rotation.
- **Dashboard** — Issuer seal badge (Sealed with issuer and key id,
  Unsealed, Invalid, Unknown key) and seal filter on Releases; per-issuer
  "% sealed, last 30 days" on the home page.
- **`/health` reports `commit`** next to `version`; Settings shows it.

### Changed

- **Published releases are immutable** — re-uploading an existing
  product+version returns `409`, as does overwriting a file through
  `PUT /releases/{product}/{version}/{filename}`. Concurrent uploads of one
  version publish exactly one artifact.
- Build stamp `GitCommit` is the short commit hash (plus `-dirty` for an
  uncommitted tree) instead of `git describe`.

### Upgrade and rollback

Migration `018_release_channel_length` (already applied in production by hand on 2026-09-18; byte-identical, so it is skipped there) and additive migration `019_issuer_sealing` (new table, four defaulted columns).
Rolling back to 1.16.1.19 needs no schema change: it ignores the new columns
and its inserts read as `unsealed`.

---

## 1.16.0 — Delivery destination telemetry (SWF)

**Build:** 1.16.0.1 · **Status:** merged to `main`; shipped to the fleet inside the 1.16.1.x line and later builds (no standalone 1.16.0 rollout).

When a node reads **silent** or **stopped**, the detail page now also says
**where** SWF is trying to deliver, so "right collector, blocked by the
customer firewall" and "wrong host:port" stop looking identical. Additive,
read-only, no control surface; the SOC channel still owns configuration
(see the reconciliation note in the addendum).

### New

- **Destination fields inside `products[].telemetry`** — `target_endpoint`
  (`host:port`), `target_resolved_ip`, `target_tls`, `target_sni`,
  `last_connect_ok_utc`. Each is omitted individually when unset;
  `target_tls` is omitted when false (read "TLS off" only when
  `target_endpoint` is present). Types in `pkg/types/types.go`; contract in
  [Relay 1.16.0 Contract Addendum](RELAY-1.16.0-CONTRACT-ADDENDUM.md)
  (proposed for SWF-team countersign) and API-CONTRACT §7.3.
- **Dashboard** — Products→swf telemetry block gains **Destination**
  (endpoint + resolved IP), **TLS** (`on · sni` / `off`) and **Last connect
  OK** rows, rendered only when reported. The delivering / silent / stopped
  signal is unchanged.

### Fixed

- **Telemetry timestamps no longer re-encode as the Go zero time.** 1.15.0
  typed `last_write_utc` / `status_utc` as `time.Time`, which `omitempty`
  never omits — an agent that left them out could reappear downstream as
  `0001-01-01T00:00:00Z` ("about 2000 years ago"). All three telemetry
  timestamps are now nullable so an omitted value stays omitted through every
  hop. The dashboard also treats a `0001-01-01…` telemetry timestamp as absent,
  covering a 1.15.0 relay left mid-cascade. Wire format when present is
  unchanged.

### Deploy ordering

Same soft rule as 1.15.0: a relay or server older than 1.16.0 silently drops
the five new fields (the 1.15.0 counters still arrive). Roll the **updates
server**, **mysoc-updater**, and **siemcore-cascade-updater** to 1.16.0+ before
or together with the destination-capable SWF updater. Any mix is safe.

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

**Build:** 1.7.0.3, tag `v1.7.0.3` · **Status:** merged to `main`; shipped with later builds (no standalone 1.7.0 rollout).

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
