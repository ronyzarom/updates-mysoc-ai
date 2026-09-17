# Fresh eight-stage combined qualification

2026-09-17 disposable local fixture. All eight stages passed on the initial run:
node1 runtime, node2 runtime, node1 schema, node2 schema, node2 seed, node1 runtime
retry, node1 read-only observation, node2 read-only observation.

Local arm64 common image:
`sha256:e1d333c091225e8b8473dbc2e5e14a0b1d5ca9af68f7b487cabea4e78c0d07f1`.
Internal fixture bridge:
`238a6131a06512c0dbc4a2def4ad2e869031e30e85c1426cb0bd6408902a8619`.
Separate test release/authorization keys, authenticated retained module, real mTLS
callbacks, immutable per-node inputs and verified peer namespace were used.

The exact read-only command appended --verify-readiness; no schema-command fallback.
Decoded returned receipt sizes: node1 4517bytes, node2 12566bytes. Readiness alone
has a32768 combined stdout/stderr bound; normal data stages retain8192. Updates
accepted observations only within5seconds, required exact33 sorted/distinct policy2
tables, matching content/table hashes and incarnation/generation, effective marker
age<=5minutes and all installation/processing/activation flags false.

Product native log copied as product-native.txt confirms initial bootstrap/retry,
source progress marker unchanged by observation, matching target mirror/incarnation,
retained Observer barrier and no assignment. Responses.json preserves exact returned
receipts and bytecounts. Stored observations are historical evidence, not reusable
current readiness: they expire after5seconds.

Limitation: this test binary was built before the final product-side internal
5second observation deadline. Updates enforced receipt freshness; the newer bounded
binary still needs qualification before any release candidate. This does not prove
full operational readiness, management/sequence/archive/routing readiness, active
permission, cloud networking, amd64 or a released clean-install kit.

Updates removed only its five stopped one-shot command containers after collecting
evidence. The orchestrator exited --rm. SiemCore retains dependency fixture resources.
No cloud VMs, production databases, publication, live delivery or activation changed.
