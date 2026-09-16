# Maintenance recovery v2 — local implementation, not enabled

Capability and outer `protocol`: `pod-maintenance-recovery-v2`.
This extends one existing v1 operation; it does not create another maintenance
owner or rewrite its binding. Inner binding retains `pod-maintenance-v1`, the
original operation ID, identities, deadline, digests and generation. V1 continues
to refuse expired mutation and predecessor success. Presence of a v2 recovery
journal prevents fallback to the v1 coordinator.

Exact request/response vectors:
`fixtures/pod-maintenance-recovery-v2/predecessor-restored.json` and
`fixtures/pod-maintenance-recovery-v2/target-installed.json`.
Types: `pkg/podmaintenance/recovery_v2.go`. Fixture keys are synthetic and MUST NOT
be provisioned on any host. Fixture artifact is synthetic bytes, not a package.

## Scoped recovery authorization

`authorization` contains `payload_base64` and `signature` (standard base64).
Verify Ed25519 over the exact decoded payload bytes prefixed by UTF-8
`mysoc-pod-recovery-authorization-v2\n` using an independently provisioned observer
public key. Do not reserialize the payload to verify its signature. Then strictly
decode JSON: reject unknown/duplicate fields, trailing data, invalid types, and
payloads above 32 KiB. This does not change the release-signing trust root.

Payload fields are `protocol`, `authorization_id` (nonempty immutable string),
`binding` (exact original v1 binding), `generation` (positive uint64), `action`
(`resume-target` or `restore-predecessor`), `issued_at`, and `expires_at` (RFC3339).
Validity is at most 15 minutes, with no grace for not-yet-valid or expired grants.
The action authorizes only the same-operation recovery/health/completion sequence
for the chosen outcome, not new apply, role reassignment, or unrelated host work.
No deadline is extended. This flow also supports explicitly authorized rollback
before original expiry. No authorization is created by the updater.

Observer mTLS identity/authorization, certificate registry, revocation, durable
barrier, verified paused processing/ingress, watchdog retirement, fencing,
readiness and replication policy MUST be rechecked by the adapter on **every
mutation**, including idempotent retries. A signed document alone is insufficient.
`authorize-recovery` acknowledges current authorization and returns fresh paused
permission bounded by the signed expiry. The updater requests it before recover
and again before complete. Product calls must independently enforce expiry and
scope during execution, drain descendants on timeout, and reconcile unknown
outcomes. Updater cancellation alone does not prove termination.

## Outcome and artifact identity

`outcome` is `target-installed` for resume-target or `predecessor-restored` for
restore-predecessor. `artifact` contains product/version/sha256/signature/path.
For predecessor, version and digest must equal original `from_version` and
`previous_artifact_sha256`; for target they must equal target_version and
artifact_sha256. Updater independently verifies the existing product release
signature with the pinned release key and hashes retained local bytes. Product
adapter additionally verifies signed manifest identity/architecture/commit and
its lifecycle applicability. Nothing downloads a replacement or restores a DB.

Actions: capabilities, status, authorize-recovery, recover, health, complete,
acceptance. Every reply binds protocol, exact binding, generation, outcome,
authorization_id, phase (paused/completed), permission_expires. Health before
complete proves the **selected outcome's** exact version/digest and paused role.
Post-complete acceptance separately proves fresh ACTIVE service/IP/processing
authority or STBY management/service health with processing/traffic disabled and
no active IP. Restoring a predecessor is never recorded as target upgrade success.

## Durable retry and completion

`recovery-v2.json` is fsynced under the same exclusive lock as v1's journal.
It retains original binding, scoped authorization, immutable authorization ID
hash history, outcome, verified artifact, phase, health and acceptance. Original
operation.json binding/generation remains unchanged. Mutation phases are persisted
before invocation. Interrupted recover repeats idempotent recover, never apply.
Completion loss uses status and acceptance without repeating product work.

Authorization replacement uses a new ID and may renew the SAME operation/outcome
only. Reusing an ID with different signed bytes is rejected. Changing outcome or
artifact mid-recovery is rejected. If completion is uncertain, status with the
previous authorization is reconciled first: completed uses the old receipt for
read-only acceptance; paused may receive the new bounded authorization.
An expired signed receipt may be used to identify completed work, never authorize
mutation. Fresh post-completion authority is still mandatory.

Accepted target marks original operation accepted. Accepted predecessor marks it
rolled-back and reports failed target upgrade with the verified predecessor as
installed. That terminal record does not automatically retry the failed target,
clear the barrier, or start a new maintenance operation. A subsequent operation
requires an explicit reviewed journal archival/next-operation transition; this
extension does not supply a generic journal deletion or reset command.

## Opt-in configuration

Under `simulation.filesystem.pod_maintenance`:

```yaml
recovery:
  protocol: pod-maintenance-recovery-v2
  observer_public_key: <independently-provisioned-observer-public-key>
  authorization_file: /protected/recovery-authorization.json
  artifact_receipt_file: /protected/retained-artifact.json
```

Both files must be private, regular, bounded files. Once a recovery journal is
written it retains the authorization and receipt for restart reconciliation even
if the input files disappear. Configure no test keys. Absence of this opt-in
cannot activate v2. Central advertisement, privileged adapter provisioning,
publication, grants and host changes remain out of scope until integrated native
qualification. Normal installations and the v1 wire shape remain unchanged.


## Validation recorded for this local change

`go test -race ./pkg/podmaintenance ./pkg/updatersim` passes, including both signed
fixture vectors, scope/key/digest/expiry failures, immutable and superseded ID
replay rejection, recovery interruption, bounded renewal, lost completion after
expiry, failed acceptance, v1 refusal and predecessor reporting. Linux amd64
updater cross-build passes. These are unit/process-fixture results, not proof of
observer mTLS, real watchdog/drain behavior, production DB safety or live role
switching. No artifact was signed for release or deployed for this change.
