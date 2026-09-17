# External Updates data-stage runner qualification

2026-09-17, disposable local Docker fixture `siemcore-data-runtime-53447`.
No cloud VMs, production data, fleet settings, release publication or kit delivery
were changed.

Actual common image:
`sha256:95a767c304c981d3412adee04fc7144223367e86e1f23d99a66dfcfe9202e3db`
Linux arm64, synthetic/unpublished version3.3.152.99, entrypoint `/app/siemcore`.
The fixture manifest binds that image/product/version/architecture; its hash is
Ed25519-signed in the fixture registry, verified against a separate protected
fixture trust file. This is test-only trust, not a production signed release.

External handoff results (see responses.json):

| Sequence | Input | Exit | Result |
|---|---|---|---|
| 1 | Changed config generation | 1 | Rejected by adapter before execution |
| 2 | Original operation | 0 | Actual image returned schema-prepared |
| 3 | Same operation/config retry | 0 | Actual image returned schema-prepared |
| 4 | Expired invitation | 1 | Rejected by verifier before execution |

Both success receipts preserve original operation, generation, node, input hash,
registry hash, artifact hash and version, with installation_complete=false and
processing_allowed=false. The two actual command containers exited0 and remained
stopped for evidence:

- updates-pod-stage-03e8a4885f4e43d8bdeb83aeb3c4cc9c
- updates-pod-stage-7c87b82b8f16426196dbf2b4fc2c0542

The runner independently checked immutable image/architecture, fixture signature,
original input bytes/registry, pinned healthy PostgreSQL runtime image and UID999,
protected mounts and socket ownership/mode, TLS copy bytes equal to PostgreSQL's
mounted TLS, and current invitation through real pinned mTLS Observer authorization.
The command ran on the descriptor's internal bridge with read-only mounts/root,
capabilities dropped, no-new-privileges, bounded output/time, memory/CPU/PID limits,
and no Docker socket. An outer qualification orchestrator held the Docker socket;
it was never exposed to the product command.

The native product harness owns the inserted-row preservation and retained-barrier
assertions; its final test result must be recorded separately. This Updates run
proves real process/receipt integration, not full A/B/Observer bootstrap, selected
synchronization readiness, continuing RPO, active permission, role IP switching,
amd64 qualification, production release trust, or clean-VM acceptance.

58 distinct local tests also pass with Python3.13, cryptography44.0.3 and
jsonschema4.26.0. `native_handoff.py` is explicitly fixture-only; no schema4 kit
entrypoint has been enabled.

Product native test completion independently read from its saved log:
`TestNativeDataBootstrapCommand` PASS99.13s (copied as product-native.txt).
SiemCore confirmed the test's inserted-row preservation, expired-invitation refusal
and retained original barrier/generation assertions passed. After receipt/log
collection, Updates removed only the two named stopped command containers above.
SiemCore owns cleanup of fixture53447's disposable dependency resources.
