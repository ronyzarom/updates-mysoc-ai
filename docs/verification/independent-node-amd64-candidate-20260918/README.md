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

## Actual isolated publication and native bootstrap

Common SiemCore3.3.152.40 archive SHA
1d79b0c3cbdc54cb36fbcd337c6a5e9583659f5765ea17064034ea3b752dd7fc,
65,359,811bytes, published node-a-20260918/alpha only at12:31:37Z. Metadata,
Ed25519 signature and full parent-license-authenticated download verified.
The first verification script received401 from its unauthenticated download
step AFTER successful publication. This was initially misattributed to upload
authentication; catalog reconciliation corrected that diagnosis. A redundant
file transfer was stopped before any second POST; the release was never overwritten.

Actual kit installer completed on A; UUIDf00a55b8-bd5a-4e44-a3f1-0e2b00b0f463
assigned alpha/automatictrue via supported API. All other fleet controls and
benchmark hold unchanged. Updater1.16.1.28, stable self-update, SSL.com relay TLS.
At12:37:32Z the updater accepted .40;12:37:38Z verified the artifact;12:37:39Z
invoked the protected root apply boundary. No manual product apply occurred.

At12:38:15Z the root final readiness verifier failed closed with ValueError.
PG/Redis/management containers and HTTPS health are running, but original root
journal remains installing; this is NOT accepted updater bootstrap success.
Cause confirmed: Docker29 returns CAP_DAC_OVERRIDE/CAP_NET_BIND_SERVICE while
product verification expects unprefixed spellings. Image/binary/identities and
actual allowed capability set match. No data/container recreation or artifact
rewrite was performed. Exact original transaction is retained for retry.

`node-a-failed-bootstrap-binding.json` records nonsecret original journal,
policy/config/artifact hashes and retained container IDs. The narrow signed
health-only repair contract is `docs/POD-NODE-BOOTSTRAP-REPAIR-CONTRACT.md`.
Product normalization fix is source-only pending reviewed signed root repair.
