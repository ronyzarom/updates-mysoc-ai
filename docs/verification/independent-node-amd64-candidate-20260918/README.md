# Independent node AMD64 candidate — 2026-09-18

Candidate kit **1.16.1.28-r2**, Linux AMD64, source `73f8cad`, provisioning hook
`c5ac122`. Revision r2 adds an explicit relay certificate/key requirement before
node installation. Missing material fails before host mutation, including when
node type is inferred from the protected envelope. Normal installation is unchanged.
SSL.com chain/SAN/expiry/live-listener validation remains a separate deployment gate.

- Archive SHA256: `4df964cd74b0ba391f5475615d970596f4056e0ad8a6ccb60814639a7b89a2ee`.
- Binary SHA256: `77d44297f4acdccce6905665bd08ab1f940839967a6992bf125bf5a5b5a42267`.
- Binary receipt and outer repository manifest signed in place on the origin,
  using the existing fleet Ed25519 key; no private key export.
- Both signatures verified locally; public key independently matches the origin
  HTTPS `/api/v1/signing-key` response. ELF architecture checked by packager.
- 27 installer/identity tests, signing/architecture package test, and targeted
  Go updater/licensing/CLI tests pass. Shell syntax and diff checks pass.

No release row, download repository, fleet assignment or host installation was
changed by candidate assembly. These are candidate pins, not deployment evidence.
Native execution and full signed bootstrap must be recorded separately. Prior
ARM64 r1 fixture qualification does not qualify AMD64 r2 execution.

Node A provisioning and SSL.com certificate staging belong to the coordinated
SiemCore task. Remaining full-bootstrap inputs include the common signed AMD64
product receipt/capability, immutable prerequisite digests, complete protected
local settings with SSL.com-compatible database TLS, and actual machine binding.

## Native node A CLI check

Using the authorized IAP route to newly provisioned `bezeq-pod-test-a`, copied
only the signed candidate binary into a unique temporary directory. Verified
host `x86_64`, exact binary SHA256, executable `version` output `1.16.1.28` /
`73f8cad`, and `installation-types` output including `pod-node`. The response
correctly retains `lifecycle_ready=false`: capability admission is not a claim
of linked-POD readiness or qualified node upgrades. See `native-node-a.json`.
Temporary binary/directory removed afterward. No installer, service, enrollment,
product apply, certificate change or fleet mutation was executed. Full native
installation remains pending the coordinated product inputs and authorization.

## Coordinated product candidate selection

Read-only origin catalog check on 2026-09-18 found latest `3.3.152.39` and no
`3.3.152.40`. Coordinated `.40` with SiemCore for node A; this is not a database
reservation and must be rechecked immediately before publication. Proposed
product channel `node-a-20260918`, target group alpha only; zero existing releases
or reported SiemCore instances used that channel at the check. Only A may be
configured for this product channel. This is channel/ring isolation, not a
per-instance ACL. Updater self-update channel remains stable. No publication or
host assignment was performed by this check.

Prior dependency runtime evidence uses ARM64 PostgreSQL and Redis with UID999.
Those fixture digests/UIDs are not AMD64 qualification. Product owner must verify
authorized AMD64 image digests and users before creating A's protected settings.
