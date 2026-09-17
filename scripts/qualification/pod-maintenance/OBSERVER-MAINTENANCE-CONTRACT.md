# Observer self-maintenance v1 — source-only contract

Separate protocol/capability: `pod-observer-maintenance-v1`. Observer is node_id `witness`, product `siemcore`, immutable POD identity. It does not use data-node Binding validation or bootstrap initialization. Default disabled; no advertisement, publication or deployment until joint qualification.

Local protected adapter uses the existing absolute-argv/single trailing action/strict bounded JSON transport. Request: `binding` (same artifact/operation identity fields as data-node binding, but this protocol and node_id witness), `generation` (0 only before prepare), optional `authority_snapshot_sha256` after prepare. Response: exact binding, generation, phase, permission_expires; optional capabilities, lifecycle_ready; authority_snapshot_sha256, authority_preserved, serialized, quorum_ready, authentication_ready, installed_version, installed_sha256 as applicable. No credentials or authority-state contents in responses.

Actions:
1. capabilities: requires exact protocol capability and lifecycle_ready=true. This is product qualification, not mere command existence.
2. prepare: before any restart, atomically acquire POD-wide serialization against data-node maintenance, takeover and uncertain cloud operations; persist no-TTL observer maintenance protection and original operation. Persist a protected baseline receipt covering current assignment, all authority/token generations and pre-existing barriers. Return phase prepared, nonzero generation, snapshot digest, authority_preserved=true, serialized=true, fresh permission. Repeated prepare returns the same operation and snapshot; no host mutation.
3. Updates fsyncs its prepared acknowledgment, then persists applying before apply. apply runs the independently verified signed update and restarts the authority service under the persisted guard. No bootstrap, replacement assignment, generation reset, or deleting unrelated barriers. Its privileged supervisor must survive/reconcile observer-service unavailability.
4. If apply reply is lost or Updates restarts in applying, call reconcile for the SAME binding/generation/snapshot. This dedicated local path reads protected product execution receipts and actual installed state; it does not require a continuously available observer endpoint. No blind replay or replacement operation. If completion is uncertain, return an error and retain protection. No automatic rollback through the normal executor.
5. health: return phase ready only after exact target version/digest, quorum and authenticated service access are independently verified, serialized=true and authority_preserved=true against the retained baseline. Permission must be fresh. Updates persists completing before complete.
6. complete: atomically complete only the matching operation, release only its own protection after verification, preserve original assignment/generations/unrelated barriers; return completed. Lost completion is reconciled by status without reapplying. status is read-only and supports completed reconciliation even after original deadline.
7. acceptance: completed, exact installed target, quorum/authentication ready, authority preserved and fresh permission. serialized is not required after completion. Active nodes obtain their own new short-lived processing permission; this response grants none.

`authority_snapshot_sha256` identifies immutable retained baseline evidence, NOT a hash of live leases. Product must compare preserved/restored authority state against that evidence. Expiring processing leases during downtime are allowed; resetting authority generations or assignments is not. Removal of this operation's own maintenance protection at completion is allowed; removal of unrelated barriers is not.

All mutations require the original deadline and the local protected execution guard; expiry retains state and requires future explicit recovery, never deadline extension or bootstrap fallback. Failure to renew active processing permission causes controlled pause at expiry. Neither the updater nor observer maintenance extends permission indefinitely.

Updates journal phases: intent, acknowledged, applying, applied, completing, completed, accepted. Pending operation cannot be replaced by another release. Source coordinator covers updater ordering; product owns atomic cross-node serialization, authority preservation, durable execution receipts, process supervision and actual service reconciliation. Normal and existing data-node execution remain separate.

## Updates source integration

Implemented coordinator and exact wire types: `pkg/podmaintenance/observer.go`.
Explicit local config is `simulation.filesystem.observer_maintenance`, with enabled
(default false), protocol, pod_id, node_id=witness, journal_directory and adapter_command.
It requires persisted pod-observer installation identity and rejects Normal/data-node
use. Startup resumes observer-operation.json before offers; errors never invoke the
normal filesystem rollback executor. Existing signature/checksum verification runs
before calling the observer coordinator, including restart reconciliation. Product
must still independently verify the protected signed artifact and prerequisites.

The journal stores the prepared generation/snapshot, exact original operation and
latest evidence. Lost apply uses reconcile; lost complete reads status and does not
reapply. Completed read-back can run after the original deadline but still requires
fresh final service acceptance. Expired incomplete operations stay blocked pending
a future explicitly scoped observer recovery contract; none is invented here.

Capabilities.lifecycle_ready is a statement that the complete protected executor
is qualified, not that the observer endpoint happens to be reachable at that instant.
Local reconciliation must remain possible while that endpoint restarts. No capability
advertisement or release-target changes are enabled by this implementation.

Qualification driver adds observer-maintenance mode and records observer journal
hashes. Fixture cases must include actual authority-service outage, cross-node
serialization, no perpetual permission, signed apply and exact preservation checks.
Updates subprocess tests cover lost replies/process exits and retention; they do
not establish product runtime qualification.

Historical completion must remain readable after later legitimate pod activity:
preservation is attested atomically at completion, not by freezing live assignment
history forever. Final acceptance separately measures current service health.

Focused Observer tests and race tests pass, including actual process exits after
prepare/apply/complete and journal checkpoints, exact-operation reconciliation,
authority/generation/snapshot mismatch, failed quorum/artifact checks, expiry,
lost completion after expiry, disabled configuration, and Normal/data-node isolation.
