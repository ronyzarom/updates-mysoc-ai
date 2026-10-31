# Relay Wire Contract — Index

The relay protocol is the contract between an updater and its parent (a relay
or the updates server). It is defined across several documents written per
release. This page is the index: what is current, what each addendum added,
and the compatibility floor a release must respect.

The relay is the backbone of the cascade. Below tier 1 a node holds no
credential for `updates.mysoc.ai`; its relay is its only path to product
updates and to its own self-update. Every change here is additive and must
work with any mix of versions across hops.

## Documents, in order

| Document | Scope | Status |
|---|---|---|
| [API-CONTRACT.md §9 — Cascade Distribution](API-CONTRACT.md#9-cascade-distribution-added-in-180) | Base relay protocol (1.8.0): agent auth with `X-License-Key`, ed25519 release signing and `GET /api/v1/signing-key`, heartbeat `children` rollup, child-facing endpoint subset (`heartbeat`, `updates/{p}/check`, `updates/{p}/report`, `releases/{p}/{v}/download`), relay token issuance. | Live |
| [UPDATER-GUIDELINES.md §14 — Cascaded Distribution](UPDATER-GUIDELINES.md#14-cascaded-distribution-180) | Behavioural requirements on relays and children (verify at every hop, pull-through cache, no internet below tier 1). | Live |
| [RELAY-1.11.0-CONTRACT.md](RELAY-1.11.0-CONTRACT.md) | Identity adoption (`identity` object on heartbeat responses) and `POST /api/v1/decommission`. | Countersigned, live |
| [RELAY-1.12.0-CONTRACT-ADDENDUM.md](RELAY-1.12.0-CONTRACT-ADDENDUM.md) | Heartbeat interval hint (`child_heartbeat_interval`), delta reporting, child caps. Advisory, opt-in. | Live |
| [RELAY-1.14.1-CONTRACT-ADDENDUM.md](RELAY-1.14.1-CONTRACT-ADDENDUM.md) | Reject reason `code` on child-facing `401`. Operator diagnostic. | Live |
| [RELAY-1.15.0-CONTRACT-ADDENDUM.md](RELAY-1.15.0-CONTRACT-ADDENDUM.md) | `products[].telemetry` delivery counters, carried through the rollup. | Countersigned 2026-09-05, live |
| [RELAY-1.16.0-CONTRACT-ADDENDUM.md](RELAY-1.16.0-CONTRACT-ADDENDUM.md) | Five destination fields inside `products[].telemetry`. | Carrier side live in 1.16.0; producer countersign proposed |
| [INDEPENDENT-ARTIFACTS.md](INDEPENDENT-ARTIFACTS.md) | `protocol_version` / `capabilities` negotiation for bootstrap vs update artifacts (`dual-artifact-v1`). Alpha-only, gated by server config. | Retired path; see note below |
| [RELAY-DEPLOYMENT.md](RELAY-DEPLOYMENT.md) | Operator guide: configuring the tiers, port protection, scaling, operations. | Live |

## Compatibility floor

These hold for every release and every hop. A release that cannot satisfy
them is not protocol-additive and is not shipped.

1. **Self-update offer, frozen.** The fields a node needs to take a new
   updater binary are the 1.8.3 set and are forwarded unchanged by every relay
   version: `update_available`, `latest_version`, `download_url`, `sha256`,
   `signature`, `update_group`. They are never renamed, removed, typed
   differently, or made conditional on `protocol_version` or `capabilities`.
   The `updater-*` products are never subject to the dual-artifact path or the
   per-instance auto-update gate.
2. **Additive only.** New fields are optional and omitted when unset. An older
   relay that re-encodes a message may drop a field it does not know; the
   result must be "not visible", never "broken". Nothing new is ever required
   by a child or a parent to keep updating.
3. **Signature passes through unchanged.** The release `signature` minted by
   the updates server is forwarded byte-for-byte; the relay rewrites only
   `download_url`. Every hop and every leaf verifies against the pinned fleet
   key before use.
4. **Minimum relay version for rollup is 1.8.1.1.** Older relays omit children
   from their upward heartbeat (see UPDATER-RELEASE.md, known issue).
5. **Any version mix works.** The updates server is deployed before updater
   artifacts are published; within a ring nodes self-update in any order; a
   newer child behind an older relay and an older child behind a newer relay
   are both supported configurations.

## Known gap (to be closed in 1.18.0)

`Relay.handleChildCheck` currently re-encodes a fixed, typed field set rather
than passing unknown fields through. Floor item 1 holds today because the
self-update fields are in that set; floor item 2 holds only by discipline. The
1.18.0 line adds pass-through of unknown offer fields and a test that replays
the oldest self-update check shape through a relay. See the "Relay" section of
[UPDATER-RELEASE.md](UPDATER-RELEASE.md).

## Note on `dual-artifact-v1`

The bootstrap/update dual-artifact negotiation (`protocol_version`,
`capabilities`) was an alpha-only qualification path, enabled on the server by
`DUAL_ARTIFACT_ALPHA` and the `dual-alpha-` channel prefix. SiemCore retired
the dual-artifact and special-channel model on 2026-09-29 ("one channel, pod
included"). The negotiation code remains in the relay and server and is inert
for the `updater-*` products by floor item 1. Removal is tracked in the
minimal-updater follow-up.

## How to add to this contract

1. Write the change as a new `RELAY-<version>-CONTRACT-ADDENDUM.md` with
   status, goal, exact field names, types, omission rules, and the
   degradation behaviour at an older hop.
2. Add a row above and update the compatibility floor if it changes (it should
   not).
3. Carry the "Relay compatibility" block in the `RELEASE-NOTES.md` entry for
   the release that ships it.
4. Obtain countersign from the producing or consuming team before the
   child-facing side ships.
