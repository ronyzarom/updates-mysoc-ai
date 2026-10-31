# Updater Kit Release Process

The cascade updater (all tiers) is developed, versioned, and released from
**this repository**. The `siemcore-ai` repo's `siemcore-updater/` directory is
the *retired v2 auto-updater* — a different lineage kept only as the migration
runner reference; nothing is built from it, and its old unit name is what v3
posture audits flag.

## Version anchor

There is exactly one version anchor: the repo root `VERSION` file
(`MAJOR.MINOR.PATCH.BUILD`, per the development cycle). The updater kits carry
the version of the build that produced them — the binary via ldflags
(`main.Version`, shown by `<updater> version`), the config
(`instance.updater_version`), the README title, and a stamp line in every
bundled doc all come from that single anchor at build time. There is no
separate updater version line, and no combined app×updater version: customer
or tier tailoring is a **build target**, never a version axis.

## Building kits

```bash
scripts/build-kits.sh            # both tiers
scripts/build-kits.sh siemcore   # one tier
```

Templates live in `kits/` (tracked); rendered kits land in
`dist/updater-kits/<kit>-<VERSION>/` (untracked artifacts). The script refuses
to rebuild into an existing versioned directory — bump `VERSION` first — and
stamps builds from a dirty tree `-dirty` (diagnostic only, never shippable or
tailorable).

## Tailoring for an operator/customer

Tailoring means pre-filling a rendered kit's `config.yaml` (instance id,
`parent_id`, pinned signing key, relay address) and adding a
`QUICKSTART-<target>.md`. Rules:

- Tailor a **copy** of a rendered kit; never edit templates with
  target-specific values.
- The kit version stays the version of the binary it wraps.
- Secrets (platform keys, enrollment credentials) never travel in a kit.
- After tailoring, regenerate `SHA256SUMS` and record the zip's SHA-256 in the
  handover message.
- The installer prints whichever `CHANGE-ME`/`PASTE-` placeholders remain, so
  a tailored kit automatically shows only what is left to fill.

## Unit naming

| Tier | Unit | Note |
| ---- | ---- | ---- |
| mysoc | `mysoc-updater` | |
| siemcore | `siemcore-cascade-updater` | Renamed in 1.8.1.1; `siemcore-updater` is the retired v2 daemon and is flagged by v3 posture audits. Never mask either unit to silence an audit. |

## Publishing an updater self-update

Since 1.8.3 the updater updates itself through the normal release channel.
`build-kits.sh` emits `dist/updater-kits/updater-artifacts-<VERSION>/` with
per-platform binaries. Publishing them is the deploy step of the development
cycle and needs an explicit instruction naming the ring.

### Where the authority lives

- The releases API (`POST /api/v1/releases`) requires the admin API key. It
  exists in exactly one place: `ADMIN_API_KEY` in
  `/home/bitnami/updates-mysoc-ai/config/.env` on the updates server
  (`EnvironmentFile` of `update-server.service`). It is never copied to a
  workstation, never pasted into a shell history, never put in a hub message.
- The server signs the release at upload with the fleet key. Nothing is
  signed on the workstation.
- Release records are immutable: a product/version pair can be uploaded once.
  A mistake is fixed by a new build number, never by re-upload.

### Procedure (per build)

1. Build from a clean committed tree (`scripts/build-kits.sh`); record the
   commit and the two artifact SHA-256s from
   `dist/updater-kits/updater-artifacts-<VERSION>/SHA256SUMS`.
2. If the updates server itself changed in this release, deploy it first
   (`scripts/deploy.sh`) and confirm `GET /health` on
   `https://updates.mysoc.ai` before publishing any updater artifact. An old
   server must never be asked to serve offers a new updater depends on.
3. Publish to the approved ring only:

   ```bash
   scripts/publish-updater.sh --version <VERSION> --groups alpha --yes
   ```

   The script copies both `updater-linux-*` artifacts to the server, reads the
   admin key from the server-side env file inside the SSH session, posts each
   as product `updater-linux-<arch>`, `channel=stable`, with the given
   `target_groups`, and then prints the public listing for the version so the
   result is verified from the outside. `--groups` is mandatory; there is no
   default. (Until the script exists on `main`, the same steps are performed
   by hand over SSH; `scripts/upload-release.sh` must not be used for updater
   artifacts because its `--groups` handling is broken on `main`.)
4. Verify from a workstation, with no credentials:

   ```bash
   curl -s https://updates.mysoc.ai/api/v1/releases | jq -r \
     '.[] | select(.product_name|startswith("updater-linux")) | select(.version=="<VERSION>")
      | "\(.product_name) \(.version) \(.channel) \(.target_groups) \(.manifest.artifacts[0].checksum)"'
   ```

   Both products listed, `channel=stable`, exactly the intended groups, and
   checksums equal to the build's `SHA256SUMS`.
5. Merge the version branch to `main` (fast-forward), tag `v<VERSION>` on the
   build commit, push both. Update the entry in `RELEASE-NOTES.md` to the ring
   it is on.
6. Notify the consuming projects on the hub (`info`, no wake): version, groups,
   both checksums, commit and tag, and what behaviour changes for them.
7. Soak (below) before any widening. Widening is
   `PUT /api/v1/releases/{product}/{version}/target-groups` with the complete
   new list, per product, per ring, each on its own explicit instruction.

### How a node takes it

Every cycle each updater checks its parent for its own platform product using
the running binary's stamped version; offers cascade through relays with the
same signature verification as any release. The updater stages the binary,
validates that the candidate executes and reports the offered version,
atomically retargets its `current` symlink, and exits for systemd to relaunch
it. The new binary finalizes the persisted marker; if the wrong version comes
up instead, the watchdog restores the previous binary and reports the failure
upstream.

Requirements and scope:

- Only installs using the kit layout from 1.8.3+ (`install.sh` versioned
  layout under `/var/lib/<unit>/self-update/`) can apply self-updates; older
  installs log a warning and continue — reinstall the kit once to re-arm.
- Self-update is independent of `simulation.mode` and of product installs;
  disable per node with `self_update: { disabled: true }`.
- The check carries the node's update group like any product check, so a ring
  is exactly the set of nodes that will take the binary.
- **Every fleet host is a relay** (`relay.enabled: true` in both kits). A
  self-update therefore restarts a relay, and its children see the parent
  unreachable for the restart window (observed: seconds). Children recover on
  their next heartbeat with no operator action. See [Relay](#relay).

### Updater soak (required before widening)

A build has passed soak on a ring when, for **every** node in that ring,
including the ring's relays:

- the node heartbeats on the new version for at least 3 consecutive cycles;
- its `self_update` report for this version is `success=true` and no
  `self_update` failure report exists for it;
- at least one product apply (or a no-op product check) has completed through
  the new binary without error;
- for a relay: its children heartbeat through it on the new version and at
  least one child download served from the relay's cache succeeded;
- no node in the ring is stuck in `pending_self_update`.

Record the soak result (ring, nodes, timestamps) in the `RELEASE-NOTES.md`
entry before asking to widen.

## Relay

The relay is the backbone of the cascade: below tier 1 a node has no
credential for `updates.mysoc.ai`, so its relay is its only path to product
updates and to its own self-update. A release that breaks a relay breaks every
node beneath it, including that node's ability to receive the fix.

Rules for every release:

- **Server first, then ring.** The updates server is deployed before the
  updater artifacts are published; the ring then self-updates in any order.
  There is no way to order nodes within a ring and none is assumed.
- **Protocol-additive only.** Any mix of versions across hops must work. A
  relay re-encodes the offers and heartbeats it forwards, so a new field
  added upstream is dropped by an older relay in between; a release may only
  add optional fields and must degrade to "hidden", never to "broken".
- **The self-update offer is frozen.** The fields a node needs to take a new
  updater (`update_available`, `latest_version`, `download_url`, `sha256`,
  `signature`, `update_group`) are the 1.8.3 set and are never renamed,
  removed, or made conditional on protocol or capability negotiation. Every
  relay version forwards them. This is what guarantees a host can always be
  rescued by a newer updater through an older relay.
- **Relay compatibility block.** Every `RELEASE-NOTES.md` entry carries a
  "Relay compatibility" block stating: what an older relay or server drops
  from this release's new fields; the minimum relay/server version, if any,
  for the new behaviour to be visible; and whether the release changes
  child-facing behaviour (listener, auth, download). "None" is a valid and
  common answer, but it is stated.
- **Operator deactivation is fleet-impacting.** A `401` from the parent
  starves the whole subtree of updates and of self-update within one
  heartbeat. Treat it as a fleet action with that consequence stated.

Wire contract index: [RELAY-CONTRACT.md](RELAY-CONTRACT.md). Operator guide:
[RELAY-DEPLOYMENT.md](RELAY-DEPLOYMENT.md).

## Survivability invariants

The updater is the component that must keep working when everything else on
the host has failed, because it is the only path to a fix. These are the
invariants it is held to. Status is per invariant: **holds** in the current
release, or the release that will make it hold.

| # | Invariant | Status |
|---|-----------|--------|
| 1 | Self-update is evaluated first in every cycle, in isolation, with its own deadline; its outcome never depends on heartbeat success or on any product hook. | 1.18.0.1 (code; Phase 1) |
| 2 | A candidate binary must prove it boots on this host (real config and state, read-only preflight) before the symlink swap. | 1.18.0.1 (code; `preflight` subcommand) |
| 3 | Every phase has a deadline, including downloads; a stalled cycle is cancelled, never waited on. | 1.18.0.1 (code; cycle/product/download/stall deadlines) |
| 4 | An out-of-process, root-owned backstop restores `.previous` if the activated binary keeps dying during the handoff. | 1.18.0.2 kit (Phase 2: `OnFailure` restore unit + root `-self-restore` script; pending host-rollout approval) |
| 5 | Several trusted signing keys; the self-update product is served on a frozen protocol by server and relays. | 1.18.0.1 (code; `public_keys` + frozen offer pass-through) |
| 6 | Compatibility with the previous release's `config.yaml` and `state.json` is a build-time test. | 1.18.0.1 (code; `testdata/compat/`) |
| 7 | An update never backs up or restores data and never takes the server down beyond a process restart window; the updater measures the window and the server refuses to promote a release that broke the budget. | 1.18.0.1 code + entrypoint contract v1.4 (proposed) |
| 8 | The relay is the backbone: it forwards every field a self-update offer needs regardless of its own version, drains children before it restarts, never reports itself alive with a dead child listener, and is the first thing the boot backstop protects. | 1.18.0.1 (code) + 1.18.0.2 (kit backstop, Phase 2) |
| 9 | Receipts and every other file the updater writes for a root-owned consumer (`.updater-release.json`, `state.json`) are pinned by tests on `main`. | holds since 1.17.1.1 |
| 10 | The updater is minimal: the leaf binary carries only check, verify, stage, apply, health, rollback, repair and self-update; relay, load generation and reconciliation tooling ship as separate commands. | planned (follow-up) |

> Status note: 1.18.0.1 was the Phase 1 checkpoint (updater binary). 1.18.0.2
> is the current build and adds the Phase 2 kit — the root `OnFailure` restore
> unit, the `-self-restore` backstop script, and `Type=notify`/`WatchdogSec`
> with a background watchdog pinger — so the binary changed and the build number
> was incremented. Neither is on any ring, merged, or tagged; Phase 2 host
> rollout (the systemd unit changes) is gated on a separate approval.

### Refreshing the golden compatibility snapshot (invariant 6)

`pkg/updatersim/testdata/compat/<version>/{config.yaml,state.json}` is a frozen
copy of a real node's config and state from the ring fleet at the time a build
was cut. `TestLoadsGoldenConfigFromPriorRelease` loads it under the current
binary so a change that adds a required field or breaks a default fails in CI,
not on a node. When you cut a new ring build:

1. Copy the `config.yaml` and `state.json` from a representative production node
   on the **current** fleet branch.
2. Replace secrets (license keys, API keys, relay tokens) with placeholders;
   keep the structure and all non-secret fields intact.
3. Save them under `testdata/compat/<new-version>/` and point the test's
   `snapshot` constant at the new directory (keep older snapshots).
4. Never edit a snapshot to make the test pass — a failure means the new build
   dropped backward compatibility; fix the code or document the break.

## Known issue: binaries labeled 1.8.0.1 may not roll up children

The first tailored kits (testing.mysoc.ai pilot) shipped binaries built
mid-development of 1.8.0 and stamped `1.8.0.1` before the relay rollup fixes
were committed. Symptom: children enroll and heartbeat to the relay, but never
appear on the dashboard — the relay's upward heartbeats silently omit them.
Fix: upgrade the relay binary; **minimum relay version for rollup is
1.8.1.1**. This is exactly the unanchored-artifact failure `build-kits.sh`
now prevents (clean-tree requirement + `-dirty` stamping + no rebuilds under
an existing version).

## Release checklist

1. Confirm the fleet's current branch is contained in `main`
   (`git log --oneline main..<fleet branch>` is empty). Develop on
   `version/MAJOR.MINOR.PATCH`; bump `VERSION`.
2. Run the local gate (fmt, vet, tests, builds), including the compatibility
   golden tests against the previous release's `config.yaml` and `state.json`.
3. `scripts/build-kits.sh` from the clean committed tree; record the commit,
   version and artifact SHA-256s.
4. Write the `RELEASE-NOTES.md` entry, including the Relay compatibility block.
5. Stop; report. Wait for the explicit deploy instruction naming the ring.
6. Server first if it changed; then `scripts/publish-updater.sh --groups <ring>`;
   verify the public listing.
7. Merge to `main` (fast-forward), tag `v<VERSION>`, push; update the entry's
   status; notify consumers on the hub.
8. Soak per the definition above; stop; wait for the per-ring widening
   instruction. Snapshot this version's `config.yaml` and `state.json` into
   `pkg/updatersim/testdata/compat/<VERSION>/` for the next release's gate.
9. Tailor per target if needed; zip; record SHA-256. Hand over via the
   approved channel; keys travel separately.
