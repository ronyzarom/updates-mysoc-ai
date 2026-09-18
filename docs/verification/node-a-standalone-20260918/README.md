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

## Current rollout hold

The isolated .43 target list is temporarily empty while component repair completes. GCP authentication expired after the approved A stop but before start; SiemCore is renewing login to start the same VM and finish the repair. No application transition has executed. Restore alpha targeting only after root admission passes, then verify cascade application, retained data identities, application/archive health and reporting before routing the stable customer IP.

Routine later standalone upgrades and linking remain separate qualified workflows; this operation does not silently enable them. No Normal or Observer installation is reclassified.
