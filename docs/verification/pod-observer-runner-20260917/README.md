# Native Updates Observer runner qualification

2026-09-17, disposable local `siemcore-observer-bootstrap-fixture` container.
The actual verified host binary ran against the real systemd authority service
and TLS-authenticated three-member etcd. Updates invoked health verification only;
SiemCore's test harness performed the service restart.

| Request | Case | Exit |
|---|---|---|
| 1 | Initial management verification | 0 |
| 2 | Identical operation retry | 0 |
| 3 | After authority-service restart | 0 |
| 4 | Changed generation | 1 |
| 5 | Expired invitation | 1 |

See responses.json. The three successful bound receipts are identical,
observer-management-verified, installation_complete=false, processing_allowed=false.
Wrong generation was rejected by the adapter before execution; expired invitation
was rejected by the actual product command. Product reports final native harness
exit0 with maintenance barrier retained and active assignment absent.

Independent Updates checks: registry release-v1 Ed25519 signature against separate
protected fixture key; actual new.tar.gz SHA256 and exact regular archive members;
manifest product/version/architecture; archive binary digest matching host verifier;
protected root-owned executable before/after; immutable original-input hash and
registry fingerprint; protected drain/update/receipt input hashes before/after;
bounded process time and output; exact receipt binding and false completion flags.

This is a test-signed artifact (synthetic fixture metadata), not a published release.
No cloud host, production database, fleet assignment or kit delivery changed.
The fixture needed test-only python3-jsonschema installed locally; this is a test
harness dependency, not a product runtime prerequisite or installer download path.

76 local source tests pass. Remaining gates include verified runtime-module worker,
combined three-node bootstrap/schema/seed/readiness, production signed-artifact
qualification and explicitly qualified activation. Normal paths are unchanged.
