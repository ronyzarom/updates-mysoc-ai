# MySoc Updates Server — Release Notes

Newest first. Version numbers follow `MAJOR.MINOR.PATCH` with per-candidate
build numbers `MAJOR.MINOR.PATCH.BUILD` (see UPDATER-GUIDELINES §Dev Cycle).
Every entry states the build commit and tag, the ring(s) it is on, the soak
result before any widening, and a **Relay compatibility** block (what an older
relay or server drops, minimum hop version if any, child-facing changes) —
see the Relay section of [UPDATER-RELEASE.md](UPDATER-RELEASE.md).

---

## 1.20.0 — WideCore tier support + updater liveness probe & relay telemetry

**Build:** 1.20.0.2 · commit `8dfda96` · tag `v1.20.0.2` · branch `version/1.20.0` (cut from `main` at `92bed80`, which contains `v1.19.0.8`). Build history: `.1` initial candidate (`80a3974`); `.2` corrected `/api/v1/products` endpoint path in `docs/API-CONTRACT.md`.
**Status:** **live across all rings** — server + dashboard deployed to `updates.mysoc.ai` on 2026-11-22 22:48 UTC; **updater `1.20.0.2` on `alpha,beta,stable,production`** (2026-11-22 22:50 `alpha` → 2026-11-23 05:16 `beta` → 05:24 `stable` → 05:28 `production` UTC — all 13 active Linux nodes across all four rings running `1.20.0.2` with zero errors).

**Deploy & ring soak (UTC):**
- 2026-11-22 22:39: backup `~/backups/release-1.20.0.2-20261122T223903Z` (`mysoc_updates.dump`, `update-server-1.19.0.8`, `dashboard-dual-sections.conf`, `SHA256SUMS`) + `bin/update-server.pre-1.20.0.2`. Dashboard built from `git archive 8dfda96` into `~/dashboard-builds/archive-1.20.0.2` and verified HTTP 200 on all 10 routes + CSS on side port 3099 before switching.
- 2026-11-22 22:48:47: `update-server` `1.20.0.2` (`8dfda96a9b84`, `sha256: 3ae980d9…`) and `dashboard` `1.20.0.2` live (schema at head `20`, no migration).
- 2026-11-22 22:50: `updater-linux-amd64` (`sha256: 6a024606…`) and `updater-linux-arm64` (`sha256: 682f6a4a…`) published to `alpha`.
- 2026-11-23 05:16 (`alpha` soak pass, ~6.5 h): 7/7 active Linux `alpha` nodes (`drtest.mysoc.ai`, `mysoc-testing-mysoc-ai`, `testing-dr.mysoc.ai`, `testing-dr-b.mysoc.ai`, `siemcore-testing-01`, `siemcore-bezeq-pod-test-a-20261114`, `siemcore-bezeq-pod-test-b-20261114`) on `1.20.0.2`, all 4 `mysoc` relays `relay_listener_ok: true`, `swf-pilot-ronyassus-2` relaying cleanly, zero errors → widened to `alpha,beta`.
- 2026-11-23 05:24 (`beta` soak pass): 2/2 `beta` nodes (`mysoc-cloud-mysoc-ai` `success=true` at 05:17:40, `relay_listener_ok: true`; `siemcore-cloud-01`) on `1.20.0.2`, zero errors → widened to `alpha,beta,stable`.
- 2026-11-23 05:28 (`stable` soak pass): 2/2 `stable` Linux nodes (`siemcore-danshar`, `siemcore-seetech`) on `1.20.0.2` with all 7 SWF children beneath `siemcore-danshar` relaying fresh heartbeats, zero errors → widened to `alpha,beta,stable,production`.
- 2026-11-23 05:30 (`production` verified): 2/2 `production` nodes (`mysoc-cyfox-il` `success=true` at 05:28:48, `relay_listener_ok: true`; `siemcore-cyfox-il` + active SWF children) on `1.20.0.2`, zero errors. All 13/13 active Linux nodes across all four rings converged on `1.20.0.2`.

### What changed

1. **WideCore tier support (server, updater, dashboard; no DB migration):**
   - Added `widecore` to the canonical tier catalog (`rank: 1`, `parent_tier: mysoc`, sibling of `siemcore`, leaf for now — relays to nothing). WideCore nodes enroll only through their operator's `mysoc` relay and require no separate license on `updates.mysoc.ai`.
   - Updated hierarchy validation, tree child sort order (`siemcore` and `widecore` at rank 1, `swf` at rank 2), `update-group` API-key tier scoping, and issuer sealing (`widecore` issuer; `widecore-updater` / `widecore-cascade-updater` remain under `updates`).
   - Updated the updater config tier check (`product_tier: widecore` accepted, requires `parent_id`).
   - Updated the dashboard (overview `WideCore Gateways` stat card + operator counts, instances tier filter & indigo badge, operators summary, releases product picker, trusted-key issuer scope, scoped API-key tier selector).
   - WideCore kits and install/enroll scripts wait until WideCore defines its install layout and apply shim.

2. **Updater liveness probe & soak follow-ups:**
   - Added optional `simulation.filesystem.availability_probe_url`: when set, outage measurement during `restart_command` polls that URL directly via HTTP GET (2s timeout, `DisableKeepAlives: true`, HTTP 200 on any version = up) instead of spawning `health_command`, avoiding false violations when `health_command` checks the target version or spawns slow commands. Falls back to `health_command` when unset; `health_command` remains the post-apply validation gate.
   - Fixed `Relay.handleChildReport` dropping `unavailable_seconds` and `availability_violation` when recording a child's `POST /api/v1/updates/{product}/report` (because `ChildrenReport()` prefers the newer report timestamp over `Heartbeat.LastUpdateAttempt`, relayed nodes previously lost availability telemetry at the relay hop).
   - Logged `unavailable_seconds` and `availability_violation` on `"update applied and verified"` for direct per-node journal inspection.
   - Sent a non-fatal heartbeat before `selfUpdatePhase` on the first cycle after boot (and while `PendingSelfUpdate != nil`), preventing the one-off `403 unknown source: enroll via heartbeat first` right after a self-update restart while keeping invariant 1 (heartbeat failure never blocks self-update).
   - Enforced `validateCumulativeGroups` on `POST /api/v1/releases` when `target_groups` is non-empty (`400` on non-cumulative or duplicate rings such as `production` or `alpha,stable`), matching `PUT /releases/{product}/{version}` and `PUT …/target-groups`.

### Relay compatibility

- **Mixed-version tolerance:** Fully backwards- and forwards-compatible across any version mix. No wire-breaking changes, no DB migrations.
- **Older relays (< 1.20.0.1):** Continue relaying heartbeats, checks, and downloads unchanged. A `1.19.0.8` mysoc relay already forwards child heartbeats with `product_tier=widecore` verbatim (it does not validate child tier names), and drops `unavailable_seconds` / `availability_violation` only on `POST …/report` (fixed once the relay itself updates to `1.20.0.1`).
- **Minimum server version for WideCore enrollment:** Server `1.20.0.1+` is required before WideCore nodes enroll (older servers reject `product_tier=widecore` in hierarchy validation).
- **Child-facing changes:** None.

---

## 1.19.0 — Self-service rings + target groups are the only selector

**Build:** 1.19.0.8 · commit `952f7bf` · tag `v1.19.0.8` · branch `version/1.19.0`.
**Status:** **live across all rings** — server + dashboard deployed to
updates.mysoc.ai on 2026-11-18; **updater `1.19.0.8` on `alpha,beta,stable,production`**
(2026-11-20: alpha 11:27, beta 13:40, stable 13:50, production 14:03 UTC — all 13
active Linux nodes across all four rings running `1.19.0.8`). Issuing SiemCore a
self-service ring key and updating `/install/` kits remain separate gates. Build history: `.1`
enrol-into-a-ring (#2) + scoped ring key (#3); `.2` makes target groups the sole
update selector (#4, below); `.3` rebased onto `main` (carries 1.18.1.5 and the
1.18.2.1 measurement fix) and removes channel from the publishing tools,
dashboard, configs and docs (#5); `.4` makes deleting a release delete its
files (#6); `.5` adds automatic release retention (#7); `.6` is the dashboard
review fixes and the server changes they need (#8); `.7` restores withdrawing a
release with an explicit empty `target_groups` list, which `.6` had turned
into a `400`; `.8` closes the dual-alpha deploy blocker (#9).

**Local validation of `.8`:** gofmt, `go vet`, full Go suite on PostgreSQL 15.
Upgrade check: the real 1.18.0.2 server (commit `8cab819`) seeded a throwaway
database (releases incl. a dual-alpha release on all rings, license, two nodes,
admin user, API key); 1.19.0.8 applied 020 with no row lost, served the same
normal offers and stopped offering the dual-alpha release. Rollback check: the
1.18.0.2 binary runs on the 020 schema unchanged (ignores the extra ledger row;
refuses the new ring key with `403`), so rollback is a binary swap. Reverting
the schema too needs every `update-group` key row deleted (revoked rows still
block the down migration) **and** the `020` row removed from
`schema_migrations`, or a later re-upgrade skips 020 and listing keys fails.

**Deploy (2026-11-18, UTC):**

- 23:12 hub notice to mysoc-ai, siemcore, swf (alpha-only upload default,
  cumulative ring edits, `target_groups` required on `/target-groups`).
- 23:13 backup `~/backups/release-1.19.0.8-20261118T231354Z` on the server:
  database dump, the 1.18.0.2 binary (also kept as
  `bin/update-server.pre-1.19.0.8`), the dashboard drop-in, and all 90 release
  files.
- Dashboard built on the server from `952f7bf` into
  `~/dashboard-builds/archive-1.19.0.8`; every page and the stylesheet answered
  `200` on a side port before the switch.
- 23:17:27 `update-server` 1.19.0.8 (`952f7bf3071b`, linux/amd64 sha256
  `7333fce6…`) started: `applied 020_api_key_update_group_scope`, one restart,
  no errors. Dashboard drop-in switched to the 1.19.0.8 build right after.
  All 24 devices active in the previous hour heartbeated again within minutes.
- `swf 2.3.0.4` (`dual-alpha-swf`, paired) withdrawn from all four rings with
  `target_groups: []`; the record stays.
- 23:27:27 first retention pass: scanned 37, deleted 10, failed 0, exactly the
  preview (mysoc 1.3.51.7, 1.3.52.2, 1.3.53.3, 1.3.54.20, 1.3.55.3; siemcore
  3.3.183.1, 3.3.184.2, 3.3.184.3, 3.3.191.1; updater-linux-arm64 1.17.1.1).
  Release storage 1.2 GB → 672 MB.

**Updater rings (`updater-linux-amd64` sha256 `f155493b…`, `updater-linux-arm64` sha256 `bc23ef71…`, 2026-11-20 UTC):**

- `alpha` (11:27 UTC): published via `scripts/publish-updater.sh --groups alpha`
  after 1.18.2.1 passed its alpha soak (see 1.18.2 below).
  **Alpha soak passed (11:28–13:39 UTC):** all seven active Linux alpha nodes
  (`drtest.mysoc.ai`, `mysoc-testing-mysoc-ai`, `testing-dr.mysoc.ai`,
  `testing-dr-b.mysoc.ai`, `siemcore-testing-01`,
  `siemcore-bezeq-pod-test-a-20261114`, `siemcore-bezeq-pod-test-b-20261114`)
  upgraded to `1.19.0.8` and held it >2 hours (>100 cycles) with `success=true`
  self-updates, zero pending self-updates, and clean product checks. Relay
  cache verified from nginx access log: `mysoc-testing-mysoc-ai` filled its
  cache once at 11:28:01, finalized on `1.19.0.8` at 11:28:14, and served all
  three SiemCore children with no further server downloads.
- `beta` (13:40 UTC): widened to `["alpha","beta"]` on Rony's explicit go.
  **Beta soak passed (13:41–13:46 UTC):** `mysoc-cloud-mysoc-ai` upgraded
  `1.18.0.2 → 1.19.0.8` (finalized 13:41:33, `success=true`, `relay_listener_ok=true`,
  heartbeats through 13:45:36) and filled its relay cache at 13:41:21;
  `siemcore-cloud-01` upgraded via the relay cache with no direct server fetch
  and heartbeated online through the relay from 13:42:10 through 13:44:59
  (>=3 cycles, none stuck in `pending_self_update`).
- `stable` (13:50 UTC): widened to `["alpha","beta","stable"]` on Rony's
  explicit go. **Stable soak passed (13:50–13:55 UTC):** `siemcore-seetech`
  (on `1.19.0.8` by 13:50:30, heartbeats through 13:54:22) and
  `siemcore-danshar` (heartbeats through 13:54:34) both upgraded
  `1.18.0.2 → 1.19.0.8` from a single relay cache fill on `mysoc-cyfox-il`
  (13:50:16, still on `1.18.0.2`, confirming mixed-version hops), none stuck in
  `pending_self_update`, and all 7 SWF children behind `siemcore-danshar` kept
  heartbeating cleanly through the new `1.19.0.8` relay (13:53:43–13:54:25).
- `production` (14:03 UTC): widened to `["alpha","beta","stable","production"]`
  on Rony's explicit go. **Production verified (14:04–14:09 UTC):**
  `mysoc-cyfox-il` (finalized 14:04:22, `success=true`, `relay_listener_ok=true`,
  heartbeats through 14:09:03) and `siemcore-cyfox-il` (heartbeats through
  14:08:52) both upgraded `1.18.0.2 → 1.19.0.8` with zero pending self-updates;
  all three SiemCore children (`siemcore-cyfox-il`, `siemcore-danshar`,
  `siemcore-seetech`) and both SWF children behind `siemcore-cyfox-il` remained
  online. All 13 active Linux nodes fleet-wide (alpha 7, beta 2, stable 2,
  production 2) are on `1.19.0.8`.

**Follow-up findings from the 1.18.2.1 alpha soak (for the next updater build):**

1. `Relay.handleChildReport` (`pkg/updatersim/relay.go`) omits
   `UnavailableSeconds` and `AvailabilityViolation` when constructing
   `child.LastReport`, and `ChildReports()` prefers `child.LastReport` over
   `Heartbeat.LastUpdateAttempt` because its relay-side timestamp is newer —
   stripping downtime telemetry from every relayed child.
2. SiemCore's `health_command` is a target-version gate, not a liveness probe
   (inline retry loop on older hosts; target-version equality on one-line pod
   hosts). A separate direct HTTP liveness probe config (e.g.
   `availability_probe_url`, HTTP 200 on any version) is needed so the updater
   can measure SiemCore outages accurately.
3. Log `unavailable_seconds` on the `"update applied and verified"` line, and
   send the first heartbeat before the first self-update check after restart
   (avoids a one-off `403` on the first cycle after self-update).

### #9 Dual-alpha releases never reach a normal offer

With the channel filter gone, a legacy release parked on a
`dual-alpha-<product>` channel (today `swf 2.3.0.4`, published to all four
groups) became a candidate on the normal offer path, losing only on version.
The ring offer (`HighestReleaseForGroup`) and `/latest` (`GetLatestRelease`)
now skip any release whose channel starts with `dual-alpha-`, whatever its
target groups, alongside the existing artifact-kind filter. Paired
qualification releases live in that namespace too, so they are no longer
offered to anyone; SiemCore retired that model on 2026-09-29 and no product
qualified it. After deploying, withdraw `swf 2.3.0.4` with an empty
`target_groups` list so the record stops showing as live on every ring.

### #8 Dashboard review fixes

A full review of the dashboard found about 40 problems, from wrong numbers to
unsafe defaults. This build fixes them, plus the server changes they need.

**Release safety (server)**

- An upload with no target groups now lands on `alpha` only. It used to go to
  all four rings, production included.
- Ring edits must be cumulative from alpha (`alpha`, `alpha,beta`, …). A set
  that skips a ring, a duplicate or an unknown ring is a `400`. A skipped ring
  is how stable once ended up with no mySOC release. An explicit `[]` on
  `/target-groups` still withdraws a release; a body with no `target_groups`
  is now a `400`.
- The availability promotion gate (`409`, audited `?override=<reason>`) now
  also covers the notes-and-groups `PUT`. Before, that path could widen a
  release with no check.
- A dual-artifact upload matches the exact artifact kind when checking for a
  duplicate, so a qualification pair can sit beside a normal release of the
  same version.

**Fleet (server)**

- `failed=true` filter, and the failed count, skip a failure already
  superseded by a newer installed version, the same rule as the detail page.
- `customer=__none__` selects unassigned nodes; the customer directory lists
  them as "Platform / unassigned".
- Search also matches display name and IP, with `%` and `_` treated literally.
- List items carry `self_update_capable` and `relay_listener_ok`.
- Tree roots include orphans (parent not on record). A relay's rollup counts
  only the nodes below it, not itself.
- The security list and stats count only nodes that report security data.
- Empty lists are `[]`, not `null`. Sorts have an `id` tie-breaker so paging
  is stable.
- `GET /instances/{id}` and the cleanup endpoints return `404` for an unknown,
  malformed or deleted id (was `500`). Cleanup uses the derived status, so
  stale direct children shown as offline are now candidates.
- Malformed heartbeat values (a non-numeric version, `products` that is not an
  array, a non-boolean flag) can no longer fail a fleet-wide query.
- The operators list aggregates fleet counts in SQL instead of loading every
  instance and license.

**Auth and admin (server)**

- `POST /auth/mfa/disable` accepts a backup code, so a lost phone does not
  lock 2FA on.
- An admin can no longer change their own role or deactivate themselves (`400`).
- API keys: `product_tier` is accepted only with scope `update-group`, and must
  be a known tier.
- `GET /health` adds `database` and `storage` backend labels.

**Dashboard**

- Releases: ring-head pills per product, one cumulative "Rings" selector for
  upload and edit, the `409` gate shown inline with "Promote anyway (audited)",
  delete warnings for releases still on a ring.
- Home: ring-heads card with a warning when a ring has no release.
- Fleet: Degraded and Failed-updates cards, a "Failed updates" filter, badges
  for no self-update / listener down / update failed, orphan badge in the
  tree, controls shown only where they apply, inline retry for a failed page.
- Instance detail: listener-down and rejected-credential banners, a
  Self-update row, wrapped errors, "updating" in amber, "Last reported via".
- Sessions: a refresh that fails on the network or a 5xx keeps you signed in;
  only a refused refresh signs you out. Tabs share one session and adopt each
  other's rotated tokens. Sign-out always lands on `/login` and clears cached
  data.
- MFA: backup codes stay on screen until "I saved them"; login and disable
  accept a backup code. Password change signs you out.
- Profile: "Sign out all sessions".
- Admin: Operators moved under Admin; operator pages use the operator-scoped
  cascade tree; deactivate asks first; key reveals cannot be dismissed by
  accident and say so when copying fails; update-ring API keys with a tier.
- Users: you cannot lock yourself out; Active/Inactive status; delete errors
  shown. Legacy licenses: keys masked, no misleading "SOC Operator" scope.
- Settings shows the real database and storage backends.

**Relay compatibility.** No relay or updater change; nothing on the wire
between hops changed. Every server change is additive or a tighter check on an
admin endpoint. Two changes can break an existing caller and need a heads-up to
mySOC, SiemCore and SWF before deploy:

- An upload that omits `target_groups` and relied on reaching every ring will
  now reach only alpha. `scripts/publish-updater.sh` already requires groups.
- A ring edit that skips a ring (for example `production` alone) is now a
  `400`.

**Validation:** gofmt, `go vet`, and the full Go suite on Postgres 15; dashboard
lint, typecheck, 57 unit tests and `next build`. A local walk-through against a
seeded database covered releases (edit, `409` override audited in the server
log, delete, upload defaults), fleet list/tree/customers and the failed filter,
instance detail and ring change, security, operators (deactivate confirm, key
rotation), API keys, users (self-lock enforced by the server too), legacy
licenses, sign-out, and the full MFA cycle with a backup code.

### #7 Automatic release retention

Nothing ever retired old releases on the server; they piled up until someone
deleted them by hand. The server now runs a retention pass 10 minutes after
start and then daily. Per product:

- The newest 5 releases that reached the `production` ring are kept; older
  production releases are deleted (`old-production`).
- Releases that never reached production and are older than the production
  head are deleted (`never-reached-production`). Anything newer than the
  production head is still rolling out and is kept.
- A product with no production release loses nothing.

Always kept: the release each ring is currently offered (same rule as the
update check), any version a device reports (heartbeat products, including
relay-reported nodes; the updater's own version for `updater-*` products; last
update from/target versions for every product), uploads younger than 7 days,
versions that do not parse, and dual-artifact qualification releases
(`dual-alpha-*` channel, an artifact kind, or paired variants). A pass deletes
nothing when no device is known, and at most 100 releases. Deletion goes
through `DeleteRelease`, so files go too (#6). "Reached production" is the
current `target_groups`; the server keeps no ring history, so a release later
narrowed off production counts as never having reached it.

Config: `RELEASE_RETENTION_PRODUCTION_KEEP` (default 5, `0` turns it off),
`RELEASE_RETENTION_INTERVAL` (24h), `RELEASE_RETENTION_MIN_AGE` (168h).

Production preview (2026-11-18, read-only export, same selection code): 37
releases and 37 devices. The first pass would delete 10 releases, about 505 MB:
mysoc 1.3.51.7, 1.3.52.2, 1.3.53.3, 1.3.54.20 and 1.3.55.3; siemcore 3.3.183.1,
3.3.184.2, 3.3.184.3 and 3.3.191.1 (all `old-production`); and
updater-linux-arm64 1.17.1.1 (`never-reached-production`). The SWF products
and updater-windows-amd64 lose nothing.

Also corrected the device-side `keep_releases` wording: it keeps the current
and previous versions plus `keep_releases - 1` others (code unchanged).

**Relay compatibility (#7):** none; server-internal.

### #6 Deleting a release deletes its files

`DeleteRelease` removed only the database record and never the artifact files,
so every deleted release left its bytes on disk forever. On 2026-11-18 a
one-off cleanup on updates.mysoc.ai removed 522 such version directories
(22 GB, 37 live releases untouched), taking the root disk from 78% to 18%.

- The record is deleted first, then the files, so a failure can never leave a
  release pointing at missing bytes. File removal is best effort and logged; a
  leftover file is unreachable.
- Two releases of one version (bootstrap and update kinds) share a directory.
  Deleting one removes only its own files (artifact path, manifest artifacts and
  variants) and keeps anything the other still references.
- When the last release of a version is deleted, the whole version directory
  goes, including binaries uploaded separately per architecture.
- New `Storage.DeleteVersion`, with the same single-level path confinement as
  every other storage call. Tests: storage unit test (incl. traversal), and the
  DB-gated dual-publication test now checks the shared-directory and
  last-release cases against Postgres 15.

**Relay compatibility (#6):** none; server-internal.

### #5 Channel removed from publishing tools, dashboard, configs and docs

Finishes the follow-on to #4, so no one is ever asked to choose a channel.

- **`scripts/upload-release.sh`:** the `--channel` option is gone. Passing it is
  a clear error saying target groups are the only selector. Uploads no longer
  send `channel`; the server records the same `stable` default as before
  (deployed 1.18.0.2 and this build both default an omitted channel to
  `stable`), so nothing changes on the wire.
- **`scripts/publish-updater.sh`:** stops sending `channel` and drops it from
  the verification listing.
- **Dashboard:** the upload form's Channel dropdown (Stable / Beta / Alpha, next
  to Target Groups) is removed. On the deployed server a "Beta" choice there
  silently hid a release from every node. Normal uploads send no channel. The
  isolated-qualification upload still sets `dual-alpha-<product>`
  automatically. The release list drops the Channel column and labels
  isolated-qualification releases instead; the overview shows target groups.
- **Configs:** `channel: stable` removed from both kit templates, the example
  and deploy configs, and the config `install-siemcore-updater.sh` writes. The
  updater still defaults an empty channel to `stable` and still accepts the key,
  so existing host configs keep working unchanged.
- **Docs:** `UPDATER-GUIDELINES.md` §5 now defines rings as the only selector
  and records channels as retired. The SiemCore deployment guide (2.2.0)
  replaces "Channels vs Update Groups"; the SWF guide's §6 is the rings-only
  release process and states that SWF owns its own updater
  (`updater-windows-amd64`). Upload examples in the admin guide, API contract and
  `DEPLOYMENT.md` drop `channel` and start at `alpha` (two examples previously
  had no target groups and would have published to every ring).

**Relay compatibility (#5):** none. No wire field is added or removed; `channel`
keeps arriving as `stable`.

Features that let an operator stop hand-moving new nodes between rings — the
pain behind SiemCore's pod-a "new id landed in stable, move it to alpha" step —
and that collapse the confusing channel/ring duplication down to one concept.

### #4 Target groups are the only update selector (`channel` is now inert)

Per Rony's directive "we use only target groups, not channel." The normal update
offer is now chosen by **product + target group (the ring) + highest version**,
full stop. The `channel` column/field is no longer part of release selection.

- **Server selection:** `HighestReleaseForGroup` (the authoritative check-plane
  offer) and `GetLatestRelease` (the `/latest` endpoint + heartbeat hint) drop
  the `channel = $?` predicate — they now query by product (+ ring) only via the
  new `GetAllByProduct` / `GetAllByProductAndGroup`. The dead channel-only query
  helpers were removed.
- **`channel` kept inert on the wire.** The field still exists on the heartbeat,
  check, and release records and is echoed in the check response; the
  child-facing (`:18443`) golden contract is **unchanged** (byte-for-byte), so
  every deployed mysoc/siemcore/swf node keeps working. Deployed fleets may
  still send `channel`; the server simply ignores it for selection.
- **Exception (unchanged):** the alpha-gated independent-artifact qualification
  path (`IndependentOffer`, `dual-alpha-<product>`) still uses channel as an
  internal artifact *namespace* — that is a qualification scope, not a ring, and
  is excluded from the normal offer (#9). Removing it there is separate,
  out-of-scope work.
- **Backward compatible for normal releases:** every normal release is on
  `stable`, so collapsing the predicate changes no live offer for them; the ring
  (`target_groups`) was already doing the real gating. Legacy releases parked on
  a `dual-alpha-*` channel were the exception, closed by #9.

**Follow-on:** removing `channel` from new templates and docs is done in #5. A
later, cross-project phased removal of `channel` from the wire contract can
follow once no deployed fleet relies on it.

### #2 Enrolment can request a ring (server + updater + relay + contract)

A node may declare its desired ring in config (`instance.update_group`); the
server honors it **only when it first creates that node's instance row** (a
brand-new `instance_id`). Every later heartbeat ignores it, so a node can never
self-promote its ring after enrolment — thereafter the ring is server-owned (an
admin, or the new scoped key below, moves it). Empty/invalid → `stable` (the
unchanged default), so this is fully backward compatible.

- **Contract (additive):** `Heartbeat.RequestedUpdateGroup` and
  `ChildReport.RequestedUpdateGroup` (`requested_update_group`). Older servers
  ignore it; new instances still default to stable.
- **Server honours it on first-create in all three enrolment paths** —
  `UpsertFromHeartbeat` (direct nodes), `TouchFromCheck` (a check seen before
  the first heartbeat), and the cascade rollup (`UpsertReportedChildren` /
  `IngestDelta`). Each `ON CONFLICT` deliberately omits `update_group`, so the
  ring stays sticky after first enrolment. Proven against real Postgres:
  honored on first enrol, unchanged on a later request, empty→stable.
- **Updater** sends `instance.update_group` from config on every heartbeat.
- **Relay** forwards a child's requested ring up the cascade (`childReport`).
  **Cross-project:** for SiemCore's *cascade* pods the real relay is mysoc-ai's;
  it must run a build carrying this `ChildReport` field for the pod case to
  work end-to-end (coordinated on the hub). Direct nodes work with this build
  alone.

### #3 Scoped self-service ring key (`update-group` scope + tier binding)

A new least-privilege managed-key scope **`update-group`** authorizes ONLY
`PUT /instances/{id}/update-group` (move a node between rings) and nothing else.
A key may additionally be **bound to one product tier** (`product_tier`): it may
then set the ring of only that tier's instances. So SiemCore can hold one key
that moves only SiemCore nodes' rings, with no other admin power and no reach
into another tier's fleet. This is the "SiemCore has a key and that's it" shape
for ring control.

- Migration **020**: adds `api_keys.product_tier` and widens the scope CHECK to
  include `update-group`; the down migration refuses to roll back while any key
  still uses the new scope or a tier binding (validated up + guarded down).
- `requireScope` stashes the authenticating managed key in context;
  `handleSetUpdateGroup` enforces the tier binding (403 across tiers; 404 for a
  missing instance). The static master key and admin JWTs are unbound/unrestricted.
- `POST /admin/api-keys` accepts `{"scope":"update-group","product_tier":"siemcore"}`.
  The plaintext key is returned once — issuing SiemCore's key is a deploy-time
  secret step (never in a hub message), gated on Rony.

**Relay compatibility:** additive only. The child-facing (`:18443`) contract is
unchanged (the child-contract golden test is untouched); `RequestedUpdateGroup`
rides the upward rollup, which older servers ignore. No minimum hop version.

**Validation (clean once committed):** gofmt/vet/build; `go test ./...`; new unit
tests (scope allow/normalize incl. update-group; `normalizeRequestedUpdateGroup`;
relay `childReport` forwarding); DB-gated enrolment test extended for
request-a-ring across all three paths (run against Postgres 15); migration 020
up/down exercised against Postgres 15. For #4: the full `go test ./...` DB-gated
run (incl. the dual-publication integration test that exercises the normal vs.
independent-artifact offer paths) and the untouched child-contract golden test
both pass against Postgres 15, confirming selection is target-group-only and the
wire is unchanged.

**Follow-on (not in this build):** wiring the siemcore/mysoc enrol scripts to
render `instance.update_group` from `server.conf` so the one-liner itself can
target a ring (operators can set it in `config.yaml` by hand today); mysoc-ai
relay carrying the new `ChildReport` field for the cascade-pod case.

---

## 1.18.2 — Availability measurement reflects actual outage, not apply wall-clock

**Build:** 1.18.2.1 · commit `8a8bb96` · tag `v1.18.2.1` · branch `version/1.18.2`.
**Status:** **published to alpha** (`updater-linux-amd64` sha256 `049b6340…`,
`updater-linux-arm64` sha256 `9478cd3b…`) on Rony's explicit go, 2026-11-17.
**Alpha soak passed (2026-11-18..20):** all seven live Linux alpha nodes
(`drtest.mysoc.ai`, `mysoc-testing-mysoc-ai`, `testing-dr.mysoc.ai`,
`testing-dr-b.mysoc.ai`, `siemcore-testing-01`,
`siemcore-bezeq-pod-test-a-20261114`, `siemcore-bezeq-pod-test-b-20261114`)
held 1.18.2.1 for >60 hours with ~1-minute heartbeats, clean self-updates and
none stuck in `pending_self_update`; all four mySOC alpha nodes applied
`1.3.60.1`..`1.3.60.5` cleanly (`mysoc-testing-mysoc-ai` reported 10 s); all
three SiemCore alpha nodes applied `3.3.199.2` cleanly with zero warnings (the
135 s apply on testing no longer tripped the availability warning); relay cache
proven by `mysoc-testing-mysoc-ai` (one 70 MB fill in relay journal + one
download in server access log at 23:51:05 UTC, all three children updated).
Superseded on alpha by `1.19.0.8` on 2026-11-20. This is an
updater-only change (`pkg/updatersim`); the server is unchanged (`1.18.0.2`).

**Problem.** `unavailable_seconds` (invariant 7) was the restart command's full
wall-clock duration. For a cascade apply whose wrapper keeps running validation
long after the app is back up, that counted ~2 minutes of "up" time as downtime,
producing a false `availability_violation` and WARN "restart window exceeded
availability budget". SiemCore saw ~133s reported for a real outage of ~17–30s.

**Fix.** `FilesystemExecutor.restartAndMeasure` no longer times the command. While
the restart runs, it polls the configured `health_command` once per second and
counts only the contiguous window during which health did not pass (first
down→first up-after-down). Behaviour:

- Health that never drops (zero-downtime restart) → window `0`, no violation.
- No `health_command` configured → nothing to probe → window `0` (matches the
  `FilesystemConfig.max_unavailable_seconds` doc: a health command is required to
  measure anything).
- `first_install` is still never measured (whole-host provisioning).
- A real outage over `max_unavailable_seconds` still flags the violation, still
  **never aborts** the in-flight change.

**Relay compatibility.** None. `unavailable_seconds`/`availability_violation`
wire fields are unchanged; only the value computed for `unavailable_seconds`
changes (now smaller and accurate). Older/newer hops interoperate freely.

**Note for the promotion gate.** This must land before the server-side widening
gate is wired to actually *consume* violations — otherwise a legitimate ~2-min
cascade apply would falsely 409 at beta on today's overcounted signal.

## 1.18.1 — SiemCore one-command fresh install (kit + bootstrap only)

**Build:** 1.18.1.5 · commit _pending_ · tag `v1.18.1.5` · branch `version/1.18.1`.
**Status:** **published** to `https://updates.mysoc.ai/install/` (siemcore tier)
on Rony's explicit go. Build history: `.1` first
cut; `.2` folded in SiemCore's six branch corrections (placeholders); `.3` swaps
in SiemCore's four **authoritative** first_install-capable host files from
release/v3.3.198 (verified byte-for-byte against their handover SHA-256) and
reconciles the archive-credential key name to the singular form; `.4` fixes four
enrol-script findings SiemCore reported from pod a's first run (below) and
carries SiemCore's **3.3.198.2** apply wrapper; `.5` fixes a fifth finding —
`install.sh --clean` run a second time on a host that already has updater state
exited `1` silently (below).

**`.5` fix (finding 5 — second `--clean` run exits 1 silently in step 4):**

- **Root cause (same class as `.4`'s #4).** `install.sh` runs under
  `set -euo pipefail`. On a re-run, `reset_clean_state` calls `json_string` to
  read `instance_id`, `api_key`, `relay_token` out of the existing
  `/var/lib/siemcore-cascade-updater/state.json`. `json_string`'s pipeline ends
  in a `grep` for the key; when the key is **absent** the `grep` exits 1,
  `pipefail` propagates it through the command substitution, and `set -e`
  aborts the whole script with a bare `exit 1` at "creating service user and
  directories". SiemCore's pod a carried `relay_token` (and version/attempt
  fields) but **no** `instance_id`/`api_key`, so the very first `json_string`
  call killed the script. Fix: the `json_string` pipeline now ends in
  `|| true`, so a missing key yields an empty string (treated as absent)
  instead of a fatal error.
- **Don't rebuild an empty `instance_id`.** `reset_clean_state` previously wrote
  `"instance_id"` unconditionally, so an absent id became `"instance_id": ""`.
  It now rebuilds `state.json` from only the identity fields that were actually
  present (correct JSON for any subset), and the status line reports exactly
  which were preserved. A re-run on pod a's state now keeps `relay_token` and
  writes no empty id.
- **Scope:** `kits/siemcore/install.sh` only — no updater-binary, host-file, or
  releases-API change. (SiemCore's companion design proposal — when the app
  already answers on `/health/live`, keep the running version instead of
  resetting to `0.0.0` — is **not** in this build; tracked separately.)

**`.4` fixes (pod a first-run findings, enrol-script + host-file only):**

- **#4 — silent `exit 1` with no message (the blocker).** The step-5 wait loop
  ran `line="$(applied_line)"` under `set -euo pipefail`, where `applied_line`
  ends in a `grep` for `update applied and verified`. On no-match the `grep`
  returns 1, `pipefail` propagates it, and `set -e` aborts the whole script with
  a bare `exit 1` **before** the DONE line — exactly SiemCore's symptom.
  `applied_line`/`applied_version` now end in `|| true`. Proven against the real
  extracted functions on bash 3.2 (macOS) and bash 5.2 (the pod's shell): the
  fix survives no-match; the pre-fix form reproduces the silent `exit 1` on 5.2.
- **#3 — stale journal window.** The wait loop read `journalctl --since
  "-${WAIT_SECONDS}s"`, which could pick up a previous run's `backing off` (die
  at once) or an old success (false DONE). It now anchors `--since` to a
  wall-clock timestamp (`RUN_SINCE`) captured immediately before `systemctl
  restart`, so only the current run is considered.
- **#2 — DONE address.** Falls back to `SERVER_NAME` (SiemCore's `server.conf`
  key) when `DOMAIN` is unset, instead of printing a placeholder.
- **#1 — carry the current wrapper.** The kit now ships SiemCore's **3.3.198.2**
  apply wrapper (`siemcore-apply-update`, sha256 `125ec2224…`, from the
  `siemcore-universal-3.3.198.2` artifact) instead of the 3.3.198.1 bytes, so a
  fresh host starts on the same wrapper the alpha fleet runs. The other three
  host files (sudoers, `override.conf`, `executor-block.yaml`) are byte-for-byte
  identical between 3.3.198.1 and 3.3.198.2, so only the pin for the wrapper
  changed in `siemcore-host-files.sha256`.

**Scope:** kit and bootstrap only — **no updater-binary logic change and no
releases-API change.** Gives SiemCore the MySoc fresh-install model one tier
down (answer to siemcore hub task `20261113T194332Z-9b2cdc`, for their 3.3.198):

- New [`deploy/bootstrap/siemcore-enroll.sh`](../deploy/bootstrap/siemcore-enroll.sh),
  the twin of `mysoc-enroll.sh`: `--license-key … --config /root/server.conf`.
  Copies `server.conf` verbatim to `/etc/siemcore/install-seed.env`, reads only
  the enrolment keys, enrols the updater at `0.0.0` against the parent MySoc
  relay, then **waits for the updater's own `update applied and verified` line**
  (not the first `:8443/health/live` answer) and prints the address + version
  (correction #5). The updater identity key is **`UPDATER_INSTANCE_ID`**
  (correction #3). Persists a generated instance id to `/etc/siemcore/instance-id`
  so a re-run never makes a second identity (never derived from the domain — the
  DR-lab lesson). Copies any `ARCHIVE_CREDENTIAL_FILE` (GCS archiver credential,
  SiemCore's singular key name) out of `/root` to `/etc/siemcore/provisioning/`
  because the unit has `ProtectHome=yes` (correction #4).
- `kits/siemcore/install.sh` gains `--executor filesystem` (ported from the
  mysoc kit): installs the SiemCore host files and the `/opt/siemcore-cascade`
  staging root (the fleet path — **not** `/opt/siemcore`, which is the dashboard
  folder; correction #1). `first_install_timeout: 30m` (in `executor-block.yaml`).
- The **four** SiemCore host files — apply shim
  (`/usr/local/sbin/siemcore-apply-update`), sudoers entry
  (`/etc/sudoers.d/siemcore-cascade-updater`, NOPASSWD only for the lifecycle
  invocations), unit drop-in (`override.conf`:
  `NoNewPrivileges=false`/`ProtectSystem=no`/`ReadWritePaths=/opt/siemcore-cascade`),
  and the filesystem-executor block (`executor-block.yaml`, spliced into the
  `#BEGIN/#END-FS-EXECUTOR` region of `config.yaml`) — are **SiemCore-owned and
  installed/spliced byte-for-byte** from the kit, never hand-written
  (correction #2). The kit bundles them under
  [`deploy/siemcore-cascade/`](../deploy/siemcore-cascade/) with a **pinned
  `siemcore-host-files.sha256`**; `build-kits.sh` and `install.sh` both refuse
  bytes that do not match the pin. **As of `.3` these are SiemCore's
  authoritative bytes** — extracted from the `siemcore-universal-3.3.198.1`
  artifact's `cascade/` dir (committed `d64deb82` on release/v3.3.198) and
  verified byte-for-byte against the handover SHA-256. Their executor block
  also sets `first_install_phase`/`first_install_rollback=false`; the config
  loader ignores those two unknown keys and the updater's built-in fresh-install
  behaviour already matches them (first_install phase at `0.0.0`, no rollback,
  back off/resume). The sudoers grants the one wrapper bare +
  `apply`/`rollback`/`health`/`first_install`, no `SETENV`.
- `install.sh --clean` on siemcore now **preserves the relay identity**: it
  rewrites `state.json` to reset the product version to `0.0.0` while keeping
  `instance_id`, `api_key` and `relay_token` (a relay child that lost its token
  is rejected `401 relay_token_absent`), saving the prior state beside it. The
  fallback (clear the stale binding at the parent) is documented in the kit
  README.
- `scripts/publish-install.sh` takes a tier argument (`mysoc` default,
  `siemcore`); nginx `/install/` needs no change (prefix alias).

**Relay compatibility:** none affected — no child-facing protocol, config
schema, or releases-API change. The updater binary is identical to 1.18.0.3
(the self-update artifacts carry the new version only so kit and binary report
in lockstep).

**Ring at enrolment (correction #6):** a *new* instance id always enrols in
`stable` (the installer cannot set the ring). Reusing an existing id keeps that
row's ring — this is how the test pods (`bezeq-pod-test-a`/`-b`) keep their
**alpha** ring across a reinstall. A from-scratch wipe loses the `relay_token`,
so the parent must clear each pod's binding (`POST /api/v1/decommission` per id,
or restart the parent relay). Offering a group *at* enrolment remains a pending
design decision for Rony.

**Handover received (`.3`):** SiemCore's four authoritative first_install-capable
`deploy/cascade/` files + SHA-256 (hub `20261114T184220Z-dba925`), now in the
kit. Confirmed by SiemCore: seed path `/etc/siemcore/install-seed.env`; exit
codes 17 (hardware below strict target) and 18 (forbidden port open), not emitted
by 3.3.198.1 but by later 3.3.198 builds. 3.3.198.1 is live on alpha, so a fresh
host enrolled in alpha pulls the first_install-capable release. Registry/pull-
through and ready-made base images were declined as Updates scope (3.3.198.1
builds etcd and Patroni on the server from release build files; the signed-
artifact image path is a later infra call for Rony).

**Still held (separate, on Rony's word):** tearing down the two Bezeq pod servers
to customer state for the end-to-end one-line test — they are a live pod now.

## 1.18.0.3 — Updater self-install / deadlock / relay survivability hotfix

**Build:** 1.18.0.3 · commit `fbe2b3a` (VERSION bump `2fabc37`) · tag `v1.18.0.3`
(`main` fast-forwarded). Binary-only hotfix (`pkg/updatersim` +
`cmd/updater-simulator` + kit units/scripts); **no protocol or config-schema
change, child-facing relay contract frozen** (byte-for-byte child-contract
golden test recorded against the 1.18.0.2 relay). Fixes h1–h9: loopback relay
health exempt from the guard/drain gate; watchdog gates pings on cycle progress
AND the relay listener; exec hygiene (Setpgid + signal the group + WaitDelay +
bounded output); recovery under `WithoutCancel`; `unavailable_seconds` wired
into the heartbeat so the promotion gate measures with no server change;
self-update finalize deferred to first successful heartbeat + quarantine of a
failed target; per-target backoff; prune stale self-update releases +
`ensureFreeDisk` in Stage.

**Rings (self-update artifacts `updater-linux-amd64`/`-arm64`):**

| Ring | Action | When (UTC) | Approval |
|------|--------|------------|----------|
| alpha | published (`publish-updater.sh --groups alpha`) | 2026-11-02 06:26 | Rony: "go alpha" |
| beta / stable / production | **not widened** | — | awaiting soak sign-off + per-ring instruction |

**Alpha soak (read-only against `instances`, 2026-11-12/13):** all seven live
alpha cascade nodes self-upgraded to 1.18.0.3 and held it ≫ 3 cycles with
~1-min heartbeats, no failed update attempt, no failed self-update, none stuck
in `pending_self_update`: the testing relay `mysoc-testing-mysoc-ai`,
`drtest.mysoc.ai`, `testing-dr.mysoc.ai`, `testing-dr-b.mysoc.ai`,
`siemcore-testing-01`, and pods a/b (`siemcore-normal-db91d16e…`,
`siemcore-normal-b-90881d89…`). Relay path proven: the pods took SiemCore
`3.3.197.3` and the no-change `3.3.197.4` through the relay's cache during the
soak. Beta/stable/production remain on 1.18.0.2 pending their own soak and a
per-ring widening instruction.

**Server:** unchanged — `update-server` 1.18.0.2 (commit `8cab819`) remains live
(the 1.18.0.3 server-side gate note is inert until the measurement/countersign
fix; see 1.18.0 below). `git diff v1.18.0.2 main -- internal cmd/update-server`
is empty.

## 1.18.0 — Updater survivability: self-update always works, no deadlocks

**Build:** 1.18.0.2 · commit `8cab819` · tag `v1.18.0.2` (`main` fast-forwarded).
(`1.18.0.1` was the Phase 1 binary-only checkpoint; `1.18.0.2` adds the Phase 2
kit — the systemd watchdog pinger, `Type=notify`/`WatchdogSec`, and the root
restore backstop — so the binary changed and the build number was incremented
per the dev cycle.)

**Rings (self-update artifacts `updater-linux-amd64`/`-arm64`):**

| Ring | Action | When (UTC) | Approval |
|------|--------|------------|----------|
| alpha | published (`publish-updater.sh --groups alpha`) | 2026-10-31 ~10:45 | Rony: "go alpha" |
| beta | widened via `PUT …/target-groups` → `[alpha,beta]` | 2026-11-01 ~06:45 | Rony: "widen to beta" |
| stable | widened → `[alpha,beta,stable]` | 2026-11-01 19:19 | Rony: "widen to production, follow the release policy" (stable is the required intermediate ring) |
| production | widened → `[alpha,beta,stable,production]` | 2026-11-01 19:29 | same instruction, after stable soak passed |

**Beta soak (read-only, 2026-11-01 19:17Z, before widening to stable):**
`mysoc-cloud-mysoc-ai` and `siemcore-cloud-01` both self-upgraded 1.17.0.2 →
1.18.0.2 and held it ~12.5 h with ~1-min heartbeats (≫ 3 cycles); product
checks completing through the new binary (`mysoc=1.3.51.7`, `siemcore=3.3.175.8`
— SiemCore had reverted .10 to alpha-only, so a no-op check is the expected
product result); nothing stuck. PASS.

**Stable soak (read-only, 19:23Z and 19:28Z, before widening to production):**
all four live stable cascade nodes — `testing-dr.mysoc.ai`, `testing-dr-b.mysoc.ai`,
`siemcore-seetech`, `siemcore-danshar` — self-upgraded to 1.18.0.2 within 4 min
of widening and held it 6–9 min with ~1-min heartbeats (≥ 3 consecutive
cycles), product checks completing (`siemcore=3.3.175.8`, `mysoc=1.3.49.4`), no
rollback, nothing stuck. SWF nodes in stable run the Windows updater and do not
take `updater-linux-*`; Sept/Oct rows are dead instance IDs. PASS.

**Production result (19:33Z):** `mysoc-cyfox-il` and `siemcore-cyfox-il` both on
1.18.0.2 within 4 min, heartbeating. Whole cascade fleet is on 1.18.0.2 except
the wedged Bezeq observer (1.16.1.33, separate track).

**Alpha soak (checked read-only against `instances` before widening to beta,
2026-11-01 06:37Z):** every live cascade-updater node in alpha was on 1.18.0.2
and heartbeating — `siemcore-testing-01`, pod a (`siemcore-normal-db91d16e…`),
pod b (`siemcore-normal-b-90881d89…`), `drtest.mysoc.ai`, and the testing relay
`mysoc-testing-mysoc-ai` — ~20 h on the new binary (≫ 3 cycles), with SiemCore
3.3.175.10 applied through it on testing + both pods and the pods heartbeating
via the relay. Waived by Rony: the Bezeq witness/observer
(`bezeq-pod-test-observer-…`, still on 1.16.1.33), wedged on an unrelated
retained observer operation — see the observer investigation. Stale Sept/Oct-29
rows were dead or re-enrolled instance IDs, not live nodes.

**Why beta was widened:** the whole customer fleet ran updater 1.17.0.2, whose
unsigned `.updater-release.json` receipt is (correctly) refused by SiemCore's
fail-closed recovery runtime (`signed_release_not_provisioned`), so no SiemCore
release could be promoted past alpha until the fixed updater reached customer
rings. Beta nodes at widening: `mysoc-cloud-mysoc-ai`, `siemcore-cloud-01`
(both 1.17.0.2).

**Server:** `update-server` 1.18.0.2 (commit `8cab819`) deployed to
updates.mysoc.ai 2026-11-02 03:45Z via `scripts/deploy.sh` on Rony's instruction
("go production, release the deadlock"), replacing 1.16.2.3 (`7d8056fd`). Delta
since the running build was the three 1.18.0 server commits only (promotion
gate, availability-violation counting, stamped-version upload guard); **no new
migrations** (`schema at head, 19 in ledger`). Single graceful restart;
heartbeats resumed on all rings within a minute. Pre-deploy check: zero
`availability_violation` rows for any product/version, so the promotion gate
could not block anyone on day one. Note for 1.18.0.3: `unavailable_seconds` is
empty in every heartbeat's `last_update_attempt`, so the measurement is not
reaching the gate yet (gate is effectively inert until that and the
health-poll-derived definition from SiemCore's countersign are fixed).

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
