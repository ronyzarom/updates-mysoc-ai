# Independent POD node kit qualification — 2026-09-18

Tested candidate: **1.16.1.28-r1, Linux ARM64**, disposable fixture signature.
This is not a published fleet release. No live alpha targeting was changed or
verified by this local run; live AMD64 qualification remains outstanding.

## Exact package

- Updates template/binary source: `4f7753d5f2677adb7135773e4017431a3a7f8947`.
- Bundled SiemCore root hook source: `c5ac122b4be3a8dbff5df2e861916467bfba9411`.
- Product fixture driver: `36185d5` (`--inputs-only`, no installed product state).
- Kit SHA256: `1f416198389ef23b2fc6a8f5a879560e60abeb8b9355ed3409ed8484a313ff50`.
- Updater binary SHA256: `7ad2fa4c1c624a0d7b5598dca61581c37f91497a8740ea25cb94f7ec6c85339e`.
- Product version `3.3.152.99` is a fixture identifier, not a release candidate.

Both independent node slots passed from separate fresh privileged containers
with systemd, isolated nested Docker, no outer network, and no host mounts or
host Docker socket. Cached image bytes were imported into a loopback-only
registry with immutable fixture digests. No public dependency pulls occurred.
No production data was read, copied, backed up, restored, or migrated.

## Passing checks on nodes 1 and 2

- Actual packaged `install.sh --clean --server-type pod-node --node-id 1|2`
  with a protected `/etc/node-qualification/envelope.json`.
- Kit hook/commit binding, binary capability admission and signature-required
  product download; real product bootstrap via installed protected sudo hook.
- Service runs as `siemcore-cascade-updater`, with `ProtectHome=yes` unchanged.
- Successful update report from `0.0.0` to the fixture version.
- Exact installer retry preserves every running application container ID.
- Systemd restart succeeds; installed version and immutable node identity
  persist and are sent in the next heartbeat.
- Actual updater self-update requests use **stable**, version **1.16.1.28**;
  automatic self-update remains enabled. The parent is a local test server
  returning alpha policy, not the live fleet controller.
- Installed root hook health, trusted HTTPS management, password login/logout,
  unauthorized page rejection and activation rejection (409).
- `installed-unlinked`; installation/data/management readiness true;
  processing/authority/POD/link readiness false. No Observer or peer required.

Receipts are `node-a.json`, `node-b.json` and their management result files.
They contain no credentials. Application settings, private keys and passwords
remain only inside disposable fixtures.

The initial run successfully installed and restarted but stopped at an incorrect
fixture assertion requiring explicit `disabled: false`. The production default
already enables self-update. The assertion was corrected and both complete runs
were repeated on fresh hosts. No runtime relaxation was needed.

## Compatibility and limits

26 installer/envelope/type Python tests, the signing/architecture package test,
and Go suites for updater, types, licensing and CLI pass. Normal and Observer
configuration generation remains unchanged; only qualified node-unlinked input
gets the explicit independent bootstrap switch.

These tests qualify first independent-node installation and exact retry. They
do not qualify later node linking, ACTIVE/STBY processing, independent-node
version upgrades, AMD64 execution, live ring targeting or fleet publication.
