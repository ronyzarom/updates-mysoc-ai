# Revised Normal manifest compatibility review

Read-only review of fresh A VM1028971160940844512. No installation, enrollment,
fleet/ring mutation, database action, security approval, or old-image restoration.

Protected manifest SHA256 `e1c6a4e64771e1e9d155c8443dfbe7e8345f6837a36dcc1529920005f9e3b0df`;
application candidate SHA256 `86cb07ef3780694451e7d66fb00583d559acfe86596cebc7bf8560078951bf12`.
Both root-owned0600 and match SiemCore's prerequisite cleanup receipt. The receipt
has `prerequisite_checks_passed=false`, solely blocked on missing approved security
qualification; `full_bootstrap_qualified=false`. Completed scans are not CVE approval.

| Component | Revised selection | Current Normal product path | Result |
|---|---|---|---|
| etcd | private3.5.33-security2, runtime3cbced26… | hardcodes quay.io/coreos/etcd:v3.5.12 | Incompatible selection |
| nginx |1.30.5-alpine, runtime89956b83… | installer sets1.27-alpine | Incompatible selection |
| Patroni |normal-security-20260919, runtime0234553e… | prebuilt accepted only via dual metadata; otherwise builds | Handoff unconsumed |
| Redis |7.2-alpine, runtime84bab713… | same tag | Tag matches; immutable verification still required |
| TLS |SSL.com protected fullchain/key paths | Normal greenfield does not read management_tls | Serving certificate integration unproven |

The current kit **1.16.1.33-r1** accepts schema3/topology single, including GCS
configuration. Application candidate identity, URLs, archive shape,730-day retention,
STANDARD existing bucket and auto_provision=false align with that structural contract.
Credentials were not returned/read back or remotely authenticated by this review.

The candidate JSON is the application object alone. `--greenfield-input` requires
exactly `application` + `release`; release must contain the approved signed product
version, sha256, public_key, signature and channel. `normal-a-20260919` is supported
by the current20-character channel limit. No such qualified signed release exists
in this handoff. Do not pass either the standalone manifest or bare application
candidate directly as --greenfield-input.

Supported future path: verified signed kit → --clean --server-type normal → complete
protected envelope → standard cascade/root hook → corrected product executor.
No pod/node/Observer flags. Unique updater identity remains
siemcore-normal-db91d16e-97a5-452d-ae54-6db5c6d8f3bf; preserve logical application
siemcore-bezeq-pod-test. Updater self-channel stable, central alpha/automatic settings
must be verified after authorized enrollment; this review changes none.

Required before activation:
1. Explicit security disposition for residual findings; preserve blocked status now.
2. Product-owned protected handoff consumption with schema/version, exact runtime
   selection, separately typed manifest/config digests, missing/mismatch refusal
   before mutation, and no build/pull fallback. Never substitute old tags/pins.
3. Product-owned management/syslog TLS wiring; kit relay cert/key flags can supply
   the approved18443 keypair, but do not configure app443 or syslog6514. Verify actual
   served chains and hostname on each enabled listener.
4. Qualified immutable signed product artifact and complete release envelope.
   If new schema/hook behavior requires kit changes, publish a new kit revision;
   never silently claim current candidate supports an unimplemented field.
5. Clean native Normal success, missing/mismatch, interrupted retry, exact identity,
   full UI/MySoc/GCS/health, no Observer dependency, and cascade reporting tests.

Existing .44 testing qualified Updates installer/type/signed-delivery/retry behavior,
not full Normal bootstrap. Current SiemCore source retains legacy dependency choices;
no compatibility claim follows merely from successful host preparation.
