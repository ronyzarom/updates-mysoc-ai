# Pod maintenance v1 — implementation contract (not deployed)

Capability: `pod-maintenance-v1`. Normal installations omit it and keep their
existing executor. POD-NODE explicitly configures a trusted local adapter;
there is no automatic detection from a directory or fallback to normal apply.
The adapter bridges authenticated observer APIs and the verified product entrypoint.
It does not create a second maintenance operation.

Every request binds protocol, operation_id, pod_id, node_id (1/2), updater_id,
product, from_version, target_version, artifact_sha256, previous_artifact_sha256,
and deadline (UTC RFC3339). Updater persists this binding before begin-or-resume.
A different artifact cannot reuse an unfinished operation.

Adapter stdin/stdout is a bounded JSON request/response, invoked with one action:
capabilities, begin-or-resume, status, apply, recover, health, complete.
Observer actions return the exact binding, positive generation, phase
`paused` or `completed`, and permission expiry. Capability response names
`pod-maintenance-v1`. Product apply/recover receives that same acknowledged
binding and generation. Recover is idempotent reconciliation of an interrupted
apply, never an instruction to restore a database. Product rollback remains
under the same barrier; unsuccessful recovery never completes maintenance.

Health returns exact binding/generation and installed version/digest, role
ACTIVE or STBY, management_ready, processing_disabled, traffic_disabled,
authority_valid, replication_status. Before complete, both roles must be paused
(processing_disabled and traffic_disabled true). ACTIVE may start serving only
when observer completes the operation and issues fresh permission. STBY remains
paused. Replication status is reported independently (ready/degraded/unknown),
not inferred from HTTP 200. Observer enforces readiness/fencing and whether
completion is allowed; updater never assigns the active node.

The durable journal precedes each side effect. Interruption during apply uses
recover; interruption during complete uses status and an idempotent complete.
Timeouts, mismatches, malformed replies, and recovery failures retain the journal
and observer barrier. No auto-clear, nested begin, or automatic rollback outside
the contract. Completed journal may be replaced for a distinct next release only.
Single-process serialization is supplemented by exclusive on-disk operation lock.

The trusted adapter must authenticate observer calls, validate signed product
entrypoints, supervise/drain all child processes, and enforce the binding at the
privileged boundary. Updater timeout alone is not proof that product work ended.
No capability is advertised by default. Old updaters must be refused by the
product entrypoint before mutation when this protocol is required. Signed
artifact requirement enforcement and native restart/crash qualification remain
mandatory before deployment.

## Exact wire fixture and post-completion acceptance

See `docs/fixtures/pod-maintenance-v1/standby.json`. Binding `protocol` is the
literal `pod-maintenance-v1`; node_id is a string, generation an unsigned integer
(zero only before acknowledgement), deadline and permission_expires are RFC3339
strings. Booleans are JSON booleans. Unknown fields, duplicate keys, trailing
JSON and output over 64 KiB are rejected. artifact_path is a local absolute path;
the privileged product adapter must reverify its content against artifact_sha256
and existing signature provenance before execution. No new trust root.

After observer completion, `acceptance` independently verifies role health.
ACTIVE requires fresh processing authority, active IP ownership and service
health, with processing/traffic enabled. STBY requires service/management health,
fresh authority, processing/traffic disabled and no active IP ownership.
Failed acceptance retains phase `completed`; retry checks status/acceptance and
never repeats apply or clears another operation. Deployment succeeds only at
phase `accepted`. Completion after a lost reply can be reconciled past the
operation deadline; expired paused operations cannot execute or complete.

Implementation configuration (opt-in, **do not enable on live hosts yet**):

```yaml
simulation:
  filesystem:
    pod_maintenance:
      pod_id: qualification-pod
      node_id: "1"
      journal_directory: /var/lib/siemcore-cascade-updater/pod-maintenance
      adapter_command: [/usr/local/lib/siemcore-pod/maintenance-adapter]
```

The same updater binary preserves Normal behavior when this block is absent.
The adapter owns product staging/application and retained-artifact recovery;
the generic filesystem executor does not perform a second symlink swap,
restart, health check or rollback. Retained predecessor metadata is required for
this upgrade protocol; fresh bootstrap remains separate. Product integration
must refuse obsolete updater entrypoints before mutation. Native adapter
process supervision, authentication, and signed product capability enforcement need joint qualification before
rollout; unit tests are not live pod recovery acceptance.

Freshness is checked at each acknowledgement; adapters must obtain freshly
validated authority, not echo an old expiry. Complete requires a fresh expiry;
read-only status may report an already completed operation after expiry, but
acceptance must refresh authority before success. Witness maintenance is outside
node IDs 1/2: configuring a witness under this contract fails validation, with
no Normal fallback. Artifact from/target identities must match signed provenance
at the privileged adapter, not merely the request strings. Central capability
advertisement remains disabled pending this product enforcement and joint tests.

Startup and every real-mode cycle first reconcile unfinished journal operations,
without requiring a new offer or origin reachability. `artifact_signature` in
binding retains the existing release signature (no new signature format). Updater
rechecks this against its pinned key and rehashes retained artifact bytes before
reconciliation; it never downloads or substitutes a newer artifact for recovery.
An accepted journal with persisted product version is not replayed. Observe and
download modes never resume mutation. A pending operation blocks new product work
and self-update for that cycle. Product deadlines and observer policy continue to
gate any mutation, including after origin connectivity loss.
