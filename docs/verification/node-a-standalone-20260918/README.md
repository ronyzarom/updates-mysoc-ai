# Independent node A standalone enablement — 2026-09-18

Updater **1.16.1.31**, built from `140ac89587be179b4e14ea9909d9ec9b2c8ec40c`, was published on the normal `stable` updater channel for `alpha`. Actual installed versions, active services and cascade heartbeats were verified on MySoc, Normal SiemCore, Observer and node A. MySoc remained healthy on 1.3.19.2; Normal and Observer remained healthy on 3.3.152.39. The benchmark automatic-update hold remains false/disabled.

The independent-node application transition uses signed **3.3.152.43**, digest `605b41c1903c1c1eb5d2423d0873cb2af6a147988e9f05bb5c3de627f7a6010f`, and the isolated product channel `node-a-standalone-20260918`. Product execution must come through cascade. A retains its immutable POD node identity and original bootstrap receipts; its standalone effective mode is separate from installation type.

## Verification completed

- 31 Python tests cover admission, configuration inventory, signatures, encrypted input binding, interrupted prerequisite replay, semantic data identity, and exact component repair.
- Go updater tests and release/database tests pass. Native Linux binary version, `run` and `relay` commands verified for the published digest.
- `native-product.json` records disposable nested Docker tests: full application/archiver health, synthetic event ingestion/archive/retrieval/checksum, exact MySoc retry and conflict refusal, paid-AI refusal, and interrupted recovery. No live database was copied.
- Signed host-bound prerequisite **1.16.1.31-r1** enrolled A and installed successfully through GCP startup. Existing startup metadata was restored. It installs root components/configuration only, not the product.
- Encrypted configuration used A-generated recipient material, existing fleet signatures and two approved private files. No plaintext secrets entered metadata or public downloads.

## Corrections discovered in live integration

1. Python generated `__pycache__` during prerequisite enrollment. The installer treated the directory as a component file. Retrying the identical signed package from a clean verified extraction using `python -I -B` succeeded without changing the operation or credentials.
2. The agreed isolated channel exceeded the catalog's VARCHAR(20) limit. Migration 018 widens only release channel metadata to 64 characters, preserving existing values/targets. PostgreSQL temporary-table validation and repository tests passed. The Updates service restarted to refresh cached statement plans.
3. Docker returns retained mount records in varying order. Five read-only inspections confirmed identical PostgreSQL/Redis IDs, images and mount sets. Signed component repair **1.16.1.31-r2** compares records independently of order while rejecting duplicate destinations and any changed/missing/extra property. It preserves the original operation and refuses repair if product execution has started.

## Current rollout — 2026-09-18 19:59 UTC

Updater **1.16.1.33** is published on stable/alpha. MySoc, Normal testing SiemCore, and the independent Observer automatically downloaded, verified, restarted, and reported fresh heartbeats. MySoc 1.3.19.2 and SiemCore/Observer 3.3.152.39 remain healthy. Effective local self-update channel is stable; central alpha and automatic updates are enabled. Benchmark automatic updates remain disabled.

A's .43 attempt failed on absent archiver configuration. Its exact operation recovered .41 management successfully, with processing stopped and original data/bootstrap retained (`live-43-recovery.json`). The .43 target groups are empty. Product .44 is signed but **unpublished**.

The immutable .44 artifact passed disposable native tests with the real approved GCS backend: synthetic event ingestion, upload, download/checksum, configuration retry/conflict, paid-AI guard, preserved bootstrap/data IDs, interrupted recovery and exact retry. See `native-44-gcs.json` and `native-44-recovery.json`. SiemCore additionally passed three runtime marker adoption interruption/refusal tests. Fixture containers and their synthetic volume were removed; no live database was copied.

A remains on updater1.16.1.31 because terminal restored-operation reconciliation returns before checking self-updates. The .33 regression fix keeps that product hold while permitting signed self-updates. Full Go updater tests pass, plus 39 Python prerequisite/repair tests. Normal/Observer execution is unchanged.

A requires an explicit one-time delivery exception under AGENTS.md. Prepared, signed, host-bound updater repair **1.16.1.33-r2** validates VM/machine/installation identity, exact configuration, terminal operation, old binary and target signature. It preserves updater state and rejects conflicting receipts/unsafe destination paths. The successor root kit **1.16.1.33-r1** then authorizes new operation `098b3762-9ff2-40e3-b397-f1a05a83a7d3`, retaining the restored prior operation. Neither kit has been applied. SiemCore has the concrete startup scripts for review; user exception is pending.

After repair authorization and root readiness, .44 can be offered only on `node-a-standalone-20260918`/alpha through cascade. Live acceptance and public routing remain pending. B and production databases are untouched. Later linking and routine standalone upgrades remain separately qualified workflows.
