# Combined native bootstrap operation

2026-09-17 disposable local Docker fixture. Registry fingerprint
`5a4ace02c4e2929fb44d9575646d7342b3090febcf446c2ccd89c395d2082ccc`, generation2.
The preceding non-internal-network attempt is recorded separately; this is its
replacement fixture with a new registry/installation, not recovery of that attempt.

| Request | Stage | Result |
|---|---|---|
| 1 | Node1 runtime via actual authenticated module | Pass |
| 2 | Node2 runtime via actual authenticated module | Pass |
| 3 | Node1 schema and initial progress marker | Pass |
| 4 | Node2 schema | Pass |
| 5 | Node2 seed on temporary runner IP | Rejected by unchanged PostgreSQL HBA |
| 5-retry | Same operation/config seed, verified target network namespace | Pass |
| 6 | Node1 runtime retry | Pass; container/data retained |

No initial failure receipt was overwritten. Original response5 is retained beside
response-5-retry. The first seed process was also retried once unchanged solely to
capture private diagnostics (initial stderr had been discarded); it again failed
HBA before the reviewed namespace fix. Source rejected temporary runner addresses
172.30.97.3/.2, because only standby peer172.30.97.12 is allowed.

The fixture-only namespace option verifies exact immutable target container ID,
pinned PostgreSQL image ID, running state, Internal=true bridge ID and expected
fixed peer IP before using `--network container:<ID>`. Read-only mounts/root,
cap-dropALL, no-new-privileges, output/time/resource limits remain. HBA was not
broadened. The original operation, config, input hashes, generation, databases,
slots and authority state were preserved. Initial combined consumer now applies
this reviewed namespace path, so a fresh run need not repeat the known mismatch.

Actual runtime workers verified the test-signed retained bundle and module
manifest/hash/size/interpreter, loaded authenticated bytes in isolated workers and
used fresh real pinned-mTLS invitation authorization on every callback. Release
and authorization keys were distinct. Data commands used the manifest-bound common
image with no pulls. No-op verifier/authorization callbacks were used nowhere in
this combined run.

Product's saved resume log (product-resume.txt) independently confirmed disabled
connector mirroring, excluded raw logs, exact progress identity, unchanged A
container and retained local row, original barrier and no processing assignment.
Updates also read back target disabled=true/rawlogs0 and source rawlogs1; sanitized
results are in target-readback.txt/source-readback.txt. See responses.json and
seed-container.txt for bound partial receipts and actual execution configuration.

Updates removed only its four stopped one-shot command containers after collecting
evidence. Product owns retained dependency fixture cleanup. No cloud resources,
production data, live kit delivery, publication or activation were changed.

This qualifies local combined runtime/schema/selected-seed/retry integration under
test-only trust on arm64. It does NOT qualify production artifact trust, amd64,
cloud routing, full operational readiness/RPO/archive availability, activation,
role-IP switching, clean cloud VMs or a released installer. All receipts remain
installation_complete=false and processing_allowed/processing_enabled=false.
85 local tests discovered:84 passed,1 Linux-only worker test skipped on macOS;
that worker test previously passed in the supported Linux environment.
