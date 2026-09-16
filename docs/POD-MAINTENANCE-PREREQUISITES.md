# Pod updater prerequisites — private candidate 1.16.1.25

Not published, enabled or deployed. Existing updater 1.16.1.24 does not contain
these changes. Normal stable updater channel / alpha testing group remain the
intended rollout path, after joint qualification. No separate pod updater binary.

## Release capability gate

Optional multipart `updater_requirements` is stored in existing manifest JSON:

```json
{"scope":"pod-node","capabilities":["pod-maintenance-v1","pod-maintenance-recovery-v2"],"min_updater_version":"1.16.1.25"}
```

Update checks add explicit `deployment_role`: normal, pod-node or witness. An
unknown role or witness cannot receive a release with pod-node requirements.
Explicit Normal can receive the same product artifact without pod capability;
POD-NODE must meet the version and every capability. Releases omitting requirements
retain existing behavior. Legacy latest endpoints exclude requiring releases.
Origin checks requirements after final variant selection and before issuing a
URL; relay preserves role and requirements; updater checks again before download
and execution. Group/channel/automatic-update gates remain unchanged. Metadata
is routing policy, not a substitute for signed product entrypoint enforcement.
SiemCore must bind its required lifecycle protocol to the signed artifact and
reject an obsolete invocation before mutation. No new signing root or DB migration.

Configure `products[].deployment_role` explicitly on Normal/witness installations;
POD-NODE's explicit maintenance config binds pod role. A conflicting role is an
error. Updater capability advertisement defaults OFF, controlled by
`simulation.filesystem.pod_maintenance.advertise_capabilities`.

## Readiness contract

Action `readiness`, protocol `pod-maintenance-readiness-v1`.
Exact fixture: `fixtures/pod-maintenance-readiness-v1/ready.json`.
Request contains protocol, pod_id, node_id (string 1/2), updater_id.
Response repeats exact identity, capabilities, ready, observer_verified,
credentials_ready, lifecycle_ready, issued_at and valid_until. All four booleans
must be true, time must be current, and validity may not exceed 60 seconds.
Unknown/duplicate JSON is rejected. This response comes from the provisioned
trusted local adapter, which must establish fresh authenticated observer proof.
Merely reachable observer/matching binary is insufficient. Missing scoped
credentials or any concrete apply/recover/health/acceptance path means not ready.
V2 advertisement additionally requires configured observer public key/recovery-v2.

## Explicit terminal outcome to next operation

Action `authorize-next-operation`, protocol `pod-maintenance-next-operation-v1`.
Exact fixture: `fixtures/pod-maintenance-next-operation-v1/transition.json`.
Authorization uses the separate pinned observer key, over
`mysoc-pod-next-operation-v1\n` plus decoded strict payload bytes. Envelope is
payload_base64/signature. Claims bind authorization_id, previous_binding,
previous_generation, previous_outcome, exact next_binding, issued_at/expires_at
(maximum 15 minutes). Original and next IDs differ; identity/product are unchanged;
next from_version and previous digest must equal the accepted installed outcome.
A new signed next-artifact receipt is independently verified before authorization.

Observer acknowledges exact claims with allowed=true and current valid_until no
later than authorization expiry. It must check prior operation terminal acceptance,
no in-flight work/lease/watchdog ambiguity and authority to begin the specified
next operation. This authorization only changes local operation bookkeeping; the
new intent still needs a fresh observer begin/paused acknowledgement to mutate.

Under the same exclusive lock, original operation.json and recovery-v2.json are
archived **byte-for-byte**, fsynced and verified. A prepared receipt records their
hashes plus exact authorization/acknowledgement before live journals change. Only
then is the old recovery journal removed from the live slot and next intent
activated. Archived evidence remains intact. Crash finalization verifies archived
and live identities; no apply/rollback/service command is replayed. Same receipt
is idempotent; mismatched/replayed input or changed evidence refuses. The original
operation binding/generation/deadline is preserved in the archive.

Opt-in file: `pod_maintenance.recovery.next_operation_file`. A protected signed
request is necessary; no automatic journal deletion, generic reset or repeated
failed-target retry. Same target retry needs its own explicitly approved new
operation. Missing/invalid input blocks the transition. This does not permit
expired draining-state recovery; recovery-v2 remains paused-only.

## Qualification boundary

Local race/unit tests cover requirements, readiness identity/expiry, old client
separation, archive evidence preservation, interrupted preparation, denied or
expired authorization, tampered artifact/archive and idempotent transition.
Still required from SiemCore: signed artifact protocol enforcement; authenticated
readiness and next-operation endpoints; protected role/identity provisioning;
privileged adapter lifecycle and native joint crash/drain/recovery tests.
No capability, fleet membership, product hold or live grant changed here.
