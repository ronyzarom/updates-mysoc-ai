# Independent POD-node updater execution qualification

2026-09-18, local isolated root hosts only. No release publication, deployment,
fleet mutation, production database access, backup, or restore.

Both node1 and node2 passed first installation by the **actual Linux ARM64 updater
CLI**, running as an unprivileged service user. Product setup used the reviewed
signed fixture driver's `--prepare-only` path; no product apply or data journal
existed before the updater ran. Complete envelopes passed Updates `--validate-input`.

The CLI contacted a loopback HTTPS fixture relay with a trusted disposable CA,
verified the fixture Ed25519 signature/checksum, downloaded the artifact and
invoked the real protected root hook via sudo. Local PostgreSQL/Redis, shipped
schema and paused management were installed. The success report binds the exact
artifact digest. A second CLI process loaded the saved version and immutable
`pod-node` identity and reported it in its heartbeat.

Independent original-hook health and trusted HTTPS reads subsequently returned
`installed-unlinked`, `installation_complete=true`, `data_ready=true`,
`management_ready=true`, and false processing/authority/POD/link readiness on
both nodes. See the per-node JSON records.

The compiled native updater-core fixture additionally passed retained replay,
reporting and persistence on both prior completed roots; bad signatures and
corrupted downloaded checksums were rejected. Normal/Observer/legacy POD Go
regression suites passed. The native test uses original protected root commands,
not a mocked product executor.

Artifacts are test-only product version `3.3.152.99`, signed with disposable keys.
The actual updater CLI is a `dev` build, SHA256
`b2516294f25fd9b7652d766bb1c58ad455513878d39b1c3877f19e1c0079c475`.
Native test binary SHA256:
`6a81788753cb7b09c640a818197f6293e92aa1e2407aab380cf15c11ff40e503`.
These are not released kit versions or fleet signing artifacts.

## Remaining release boundary

`simulation.filesystem.independent_node_bootstrap` is default-off. The explicit
fixture opt-in requires immutable node identity, no POD/Observer authority, and
the exact protected install root/apply/health commands. It is rejected on Normal
and other server types. The user-facing installer/type advertisement remains
disabled until a matching versioned kit is assembled and qualified. No existing
fleet config was changed. Live relay enrollment, central rollup, updater
self-update, kit installation and rollout are not established by these local
loopback-server tests.

Original superseded failed fixtures were removed only after private evidence
capture and hash verification, to recover capacity. Final CLI fixtures are
`updates-node-root-updater-a` and `updates-node-root-updater-b` in explicit Docker
context `colima-updates-node-root-20260918`; default context remains unchanged.
