# Independent Observer kit qualification — 2026-09-18

**Latest result: live independent Observer bootstrap and five-minute stability
acceptance passed.** See [live result](LIVE-RESULT.md), signed kit details below,
and `final-rollup.json`. The earlier “pending” entries below are historical
pre-enrollment observations; only the fresh Observer was subsequently assigned
alpha and installed through the cascade. Updater-wide publication did not occur.

Implemented in Updates commit 849a21f321944f85a66bff898bb3174e46a5dba3.
Kit **1.16.1.26-r1**, Linux amd64, built from a clean detached checkout.
Product provisioning hook/recovery source: d138647aea0cc6ab4cfb8044b75280b2cd042b62.

Archive: `siemcore-updater-kit-1.16.1.26-r1-linux-amd64.tar.gz`

SHA256: `69395592058a45157d062e45f844c1c7b273baef8649652f59ea8efaaa692add`

Signing identity (existing fleet Ed25519 public key):
`1f1aa11a80d6ac549a26bb25daac4798c42dd469138680e68f89832bd32e7f57`

The updater binary has a verified `mysoc-release-v1` signature. The package is
covered by the signed `mysoc-installation-repository-v1` manifest. No private key
left the origin. No candidate was published, no release target changed, and no
service was replaced or restarted. Candidate CLI/checksum checks on Linux amd64
ran as an unprivileged process in a temporary directory, not as a deployment.

## Results

- 17 kit/identity Python tests passed, including protected input/machine binding,
  schema rejection, old-binary refusal, persistence and Normal isolation.
- Go updater, CLI, types, licensing and API packages passed. Database-dependent
  tests retain their existing skip when no disposable test database is configured.
- New updater tests cover retained apply failure and health failure, exact retry,
  changed digest/version rejection, immutable installation identity, protected
  executor requirements and explicit no-rollback failure reporting.
- Product clean-source tests independently rerun: 4 independent Observer tests and
  5 outer-hook tests passed. These include initial health failure, retained journal,
  retry, read-only completed replay, later health failure/recovery, changed identity,
  and refusal of incomplete/complete independent Observer rollback.
- Signed package and every packaged SHA256 verified. Actual Linux amd64 binary
  reports 1.16.1.26 / 849a21f and advertises observer-unlinked.
- Systemd and HTTPS product fixture operations are injected. This is NOT native
  clean-host or live signed-cascade application qualification. Product candidate
  remains unpublished; do not bootstrap working-tree product source.

## Alpha verification and next installation

Read-only observations at approximately 2026-09-18T03:38Z:

- GCP osherad-graylog / me-west1-c / bezeq-pod-test-witness is RUNNING,
  immutable VM ID 8079452056575878166.
- Fresh updater identity agreed with SiemCore:
  `bezeq-pod-test-observer-8079452056575878166`.
- Fresh application installation identity:
  `observer-8079452056575878166-v1`.
- No registration containing this VM ID exists in central Updates yet. Therefore
  this host's effective alpha assignment, installed updater and heartbeat are
  **pending**, not verified.
- Parent `mysoc-testing-mysoc-ai` is online, group alpha, auto_update_enabled=true.
- Latest published Linux amd64 updater remains 1.16.1.24, channel stable. Candidate
  1.16.1.26 is not a published release and has no central target groups yet.
- Kit invocation explicitly uses local self-update channel stable. After enrollment,
  assign/verify the exact fresh child as alpha with automatic updates; do not rely
  on parent inheritance. Verify effective local config, actual version, restart,
  cascade heartbeat and management-only product health before marking deployed.

Input/CLI contract is in `docs/SIEMCORE-CLEAN-INSTALL-PARAMETERS.md`, section
“Independent Observer bootstrap — kit 1.16.1.26”. Host preparation installs ONLY
this updater and protected inputs. Application installation is performed solely
by the enrolled updater receiving the approved signed common SiemCore artifact.
No POD/node identity, A/B, quorum or active permission is fabricated. Normal
installation/update remains on the existing executor path. Observer update and
rollback beyond exact bootstrap replay remain explicitly unsupported.

## Exact deployed-origin compatibility follow-up

Origin `/health` reports 1.16.1.19; latest startup log records source **2a35e76**.
This source predates InstallationIdentity and ignores the optional `installation`
field rather than validating its kind. An isolated checkout at 2a35e76 passed
`TestObserverUnlinkedOptionalFieldCompatibility`: the real HTTP decoder accepts
the new optional field on heartbeat and nested child report, preserves the child
instance ID, and does not persist the unknown installation field. No live fixture
heartbeat was sent and no origin restart/deployment is required for this protocol
compatibility. The origin UI will lack the new optional identity until a later
server upgrade. Newer servers with installation-kind validation must include the
additive `observer-unlinked` support from 849a21f before being deployed.

Parent relay HTTPS `/health` returned 200 from the existing testing host with its
pinned CA. Unknown-source health requests return 403 as designed; first heartbeat
is the guarded enrollment route. A fresh target-generated child credential is
used; no parent secret or another child's relay token is reused.

Agreed candidate product channel is `obs-test-20260918` (17 characters).
`observer-test-20260918` exceeds the fixed kit's 20-character maximum. This applies
only to the SiemCore product; updater self-update remains stable. Single-artifact
publication accepts this custom channel and explicit alpha target. No enrollment
until the signed product receipt exists and SiemCore signals readiness.
