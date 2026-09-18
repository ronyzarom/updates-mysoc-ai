# Independent node update protocol

Candidate updater 1.16.1.29 adds `pod-node-update-v1`, disabled unless the protected
host configuration enables `simulation.filesystem.independent_node_update`.
It applies only to immutable `server_type: pod-node`, node slot 1 or 2, with no
POD assignment or maintenance executor. Normal, linked POD and Observer paths
retain their existing behavior. Existing bootstrap remains available unchanged.

## Root boundary

Fixed `/usr/local/sbin/siemcore-node-update` accepts readiness/apply/status/recover.
Requests contain protocol, operation_id and target version/SHA/signature; no
caller-controlled command, path or service. Readiness also binds node_id.
The root component verifies original bootstrap, machine/install/updater identities,
both retained signed archives and signed target `pod_node_capabilities`.
Its first deployment policy permits only an exact predecessor/target transition.
The component is separately delivered in a fleet-signed maintenance kit.

Updater persists operation identity before invoking root. An uncertain result
reconciles the same operation before checking the network or accepting another
offer. Root workers retain the existing lifecycle lock across parent death and
are deadline/output bounded. Accepted replay measures health without reapplying.
Filesystem pointers are published without product commands only after root
acceptance. No Normal executor or generic rollback fallback is allowed.

SiemCore's signed worker owns management-only switching and recovery. It compares
schema/migration contents, preserves original database initialization binding,
retains PostgreSQL/Redis identities and credentials, and checks TLS, login and
all unlinked/paused flags. The original bootstrap journal stays immutable.
Only management schema version/image may change. Database copies, initialization,
migration, role activation and public dependency fallback are absent.

## Deployment gates

Local transaction/transport/parser and existing Observer tests; native AMD64
contract/worker switch/recovery tests; signed updater standard stable/alpha
self-update with installed-version/restart/heartbeat verification; signed component
readiness; then isolated node-A product offer. Product channel isolation is
separate from the updater's standard self-update channel. Benchmark hold remains.
This document records candidate implementation, not deployment success.
