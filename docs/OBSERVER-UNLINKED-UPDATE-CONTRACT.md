# Independent Observer update contract — review proposal

Status: proposed, 2026-09-18. Not enabled, not a release authorization. Existing
updater, root-hook and product guards remain intact until both owners accept this
contract and fixtures pass. Applies only to installed, unlinked Observers; Normal,
linked POD nodes, legacy witnesses and authority services retain their own flows.

## Ownership and eligibility

Updates owns signed delivery, retained operation binding, the protected root
adapter, reporting and capability advertisement. SiemCore owns its transactional
service update, measured health and recovery implementation. Credential
provisioning owns root-private bcrypt `ui.htpasswd`; neither updater nor product
update rewrites, removes, generates or logs that credential file.

Capability and execution protocol: `observer-unlinked-update-v1`.
Deployment role and persisted server type: `observer-unlinked`.

Advertise the capability only after the new binary AND a verified protected root
adapter are installed and its read-only readiness check succeeds. Version alone
is insufficient. Capability disappears if adapter integrity/readiness is unknown.
The tested minimum updater build will be recorded after qualification; do not
claim 1.16.1.26 supports this upgrade. Its existing guard is intentional.

Proposed release requirements reuse the optional requirements envelope:

```json
{
  "scope": "observer-unlinked",
  "capabilities": ["observer-unlinked-update-v1"],
  "min_updater_version": "<qualified four-part updater version>"
}
```

Current `pkg/updatecapability` accepts only POD-node requirements, so this is a
coordinated extension, not a field supported today. New scope applies to this
role only; it must not impose Observer requirements or a new executor on Normal.
Unknown/unclassified installations must not infer an Observer role. Matching
Observer updates require the capability locally even if an older origin omits
requirements. Server filtering is advisory; the root boundary enforces identity,
artifact trust and lifecycle. Publish only on an isolated product channel/alpha
for initial qualification. Updater self-update remains stable/alpha/enabled.

## Operation binding

Root-private durable journal directory:
`/var/lib/siemcore-observer-update/operations/<operation_id>/`.
The existing root lock `/var/lib/siemcore-greenfield/hook.lock` serializes
readiness, apply, reconciliation and recovery; never have separate concurrent bootstrap/update/recovery writers.
The root adapter owns this lock; the invoked product worker must not acquire it
again and deadlock. Unknown fields, duplicate JSON keys, symlinks and invalid
enum/types are rejected.

Immutable operation object (UTF-8, RFC8785 canonical JSON for its SHA256):

```json
{
  "protocol": "observer-unlinked-update-v1",
  "operation_id": "<UUID>",
  "machine_id": "<exact local machine ID>",
  "installation_id": "observer-8079452056575878166-v1",
  "updater_instance_id": "bezeq-pod-test-observer-8079452056575878166",
  "server_type": "observer-unlinked",
  "bootstrap_policy_sha256": "<retained original application policy digest>",
  "predecessor": {
    "product": "siemcore",
    "version": "3.3.152.37",
    "architecture": "linux/amd64",
    "artifact_sha256": "<64 lowercase hex>",
    "artifact_signature": "<base64 Ed25519 release signature>",
    "binary_sha256": "<verified packaged executable digest>"
  },
  "target": {
    "product": "siemcore",
    "version": "<target version>",
    "architecture": "linux/amd64",
    "artifact_sha256": "<64 lowercase hex>",
    "artifact_signature": "<base64 Ed25519 release signature>",
    "binary_sha256": "<verified packaged executable digest>"
  },
  "signing_public_key_sha256": "<SHA256 of pinned raw 32-byte public key>",
  "ui_protection_required": true
}
```

No updater-supplied executable path, shell string, trust key or health URL is
accepted in this object. Root resolves cache/staging paths under fixed protected
roots and the management URL from retained protected application policy.
The original bootstrap policy and signed bootstrap receipt are never rewritten.
The original bootstrap journal remains historical proof, not current version.

Current version is measured from installed executable/service and identity health,
then reconciled with the last accepted update receipt. The first predecessor is
bound to the original signed bootstrap. Later predecessors must chain to an
accepted receipt. Never reconstruct a missing predecessor from an offered target.
The archive signature uses existing `mysoc-release-v1` product/version/digest
signing. Product/architecture/binary identity are additionally checked against the
verified archive contents. Both archives are retained privately before mutation.

## Readiness, command boundary and health

Root adapter exposes exact `readiness`, `apply`, `status` and `recover` operations.
Request/response use bounded JSON, not environment-controlled shell execution.
`apply`/`status`/`recover` require protocol, operation_id and operation_sha256;
full binding is supplied only to create an operation and thereafter read from
its protected journal. A changed request cannot replace a retained transaction.
Product-owned executor receives the same immutable binding and root-derived
verified staging locations over a private descriptor/file. Product output cannot
impersonate a root completion receipt.

Readiness (before an operation exists) returns protocol, capabilities,
adapter_manifest_sha256, observed_at and measured health; it contains no operation
ID. Accept its measurement for at most 10 seconds, then remeasure under the lock.
Readiness/status calls have a 30-second maximum; apply/recover have a configured
bounded deadline (initially 15 minutes) and supervised worker process group.

Operation responses contain exactly protocol, operation_id, operation_sha256, phase,
observed_at (UTC), and measured health when available. Failures add error_code
and mutation (`none`, `possible`, `confirmed`); no exception text containing
credentials. Execution deadline expiry is an uncertain result, never success or
proof of rollback. Readiness responses are valid only for the immediate attempt;
re-measure under lock directly before mutation.

Required fresh health:

- installation_state=installed-unlinked; installation_id and updater_id match;
- version and binary_sha256 match the actually selected executable;
- management_ready=true;
- pod_ready=false, authority_enabled=false, processing_enabled=false;
- no linked-profile/active-authority evidence or conflicting service is present.

Use normal HTTPS certificate/hostname verification. No cached report substitutes
for a direct probe. Unavailable/mismatched health blocks a new operation. Status
and recovery remain available for an already retained uncertain operation.

Missing `ui.htpasswd` must yield a closed UI (no anonymous fallback), while machine
health and this recovery interface remain available. Acceptance tests separately
check unauthenticated UI rejection, valid authentication and health access; do not
include passwords or hashes in transaction/telemetry output.

## State machine and recovery

Durably write and fsync intent before any mutation. Phases:

`prepared -> staged -> switching -> verifying -> accepted`

Failure after possible mutation:

`switching|verifying -> recovery_required -> restoring -> restored`

Any unverifiable/ambiguous state becomes `blocked`, retaining the original phase,
binding, error and any measured evidence. A blocked operation can only reconcile
or recover with that exact binding; it cannot be overwritten by a new offer.

1. Validate immutable installation and fresh unlinked health; verify both signed
   archives and predecessor availability; snapshot only protected application
   configuration metadata/content digests and executable/service target identity.
   Do not back up/export/restore databases or include secret contents in evidence.
2. Stage target privately. Preserve configuration, TLS, credentials, installation
   IDs and bootstrap journals byte-for-byte. No dependency/public-registry fallback.
3. Record switching intent; atomically switch the service executable under lock.
   SiemCore owns the exact service transition. No bootstrap initialization,
   linking, authority enrollment, role assignment or token-generation reset.
4. Restart only the independent management service and measure exact target
   health. Write accepted receipt durably before Updates reports success.
5. On target-health failure, restore only the verified retained predecessor and
   its service target. Preserve the failed operation journal. Exact predecessor
   health is required before phase restored. Report update failure with recovery
   result; do not label the failed target successful.
6. After timeout/crash, `status` measures executable/service and journal before
   choosing continue-verification or recovery. Never infer success from process
   existence, a directory or a prior command exit. An exact retry may continue an
   interrupted nonterminal operation; an already accepted replay is read-only.
   A restored failed operation remains terminal failure; a fresh operator retry
   needs a new operation ID and fresh predecessor readiness, retaining history.

No generic filesystem rollback is allowed to claim success for this type.
Updater current/previous pointers are reconciled only after the root transaction
proves accepted/restored state. Interrupted pointer reconciliation is idempotent;
it must not start a second application transaction or delete retained artifacts.

## Agreed first-hop security recovery: blocked with management stopped

SiemCore accepted this rule on 2026-09-18. The retained .37 predecessor does not
enforce UI passwords. For the first .37-to-security-build transition, never start
.37 as a recovery action. On failed target acceptance:

1. Durably record recovery_required, the original binding and failure evidence.
2. Stop the failed target management service and its complete process group.
   Verify durable restart inhibition survives updater restart and host reboot
   (including service-manager restart policy), no
   descendants remain, and the UI listener is closed. An unverified stop remains
   blocked with stop_unconfirmed; it is not proof of a safe stopped state.
3. Record blocked with reason `predecessor_ui_unprotected`, preserving all
   operation/artifact/credential/identity evidence. Keep management stopped.
4. Keep the updater and its independently authenticated status/recovery boundary
   available. No .37 startup, no generic rollback, no fabricated restored receipt
   and no automatic transaction replacement. A later resolution requires the
   original operation binding and a separately qualified explicit recovery path.

This is controlled failure, not successful restoration or qualified unattended
rollback. New password-capable predecessors may reach restored only after exact
identity/version/binary health AND unauthenticated UI rejection have been measured.
Trust signed product capability evidence plus actual behavior, not a version
number alone. `ui_protection_required` is immutable across retries/recovery.

Readiness of currently installed .37 may be `eligible_for_security_upgrade=true`
while `ui_security_compliant=false`: that is the point of this isolated upgrade.
It still requires exact installed-unlinked management health and no authority.
Readiness and measured health responses expose independent booleans `ui_closed`
and `operator_login_usable`, plus readiness-only `eligible_for_security_upgrade`
and `ui_security_compliant`. These explicitly extend the response fields above.
A missing credential file producing HTTP503 can establish ui_closed=true, but
operator_login_usable=false. It must never be reported as successful operator
login. Target acceptance/restoration requires UI closed to unauthenticated users;
if operator login usability is an acceptance requirement for the operation, an
explicit authenticated login probe must also pass. Probe credentials remain
outside the operation payload and logs. For the requested password-protection
rollout, provision a valid credential and require that probe before reporting the
operator-facing feature usable.

Machine-readable first-hop conformance cases are in
`scripts/qualification/observer-unlinked-update/first-hop-cases.json`. They are
integration requirements, not evidence that an executor has been implemented or
qualified. Both teams must execute them against the real adapter/product worker
before removing existing guards.

## Protected adapter delivery — separate from product execution

Existing binary self-update cannot replace root-owned hooks. Deliver adapter and
new updater in a versioned, signed Updates installation kit using the existing
`mysoc-installation-repository-v1` manifest and existing pinned Ed25519 key.
No new signing key or public registry is introduced.

The Updates-owned root maintenance installer is run via the already approved
host-preparation route. It performs only updater/adapter maintenance, never a
product install/apply hook. Root admin verifies the signed kit with the pinned key
before running its maintenance code. Package records adapter protocol, module
hashes, updater minimum version and allowed predecessor adapter hashes.

Maintenance sequence:

1. Verify exact VM/machine and persisted observer-unlinked installation; verify
   current adapter against an allowed predecessor hash. Reject Normal/linked POD
   and any unknown or unfinished product transaction. Preserve all input pins,
   identity, credentials, updater state and completed bootstrap receipt.
2. Quiesce only updater execution; acquire the shared root lock and recheck that
   no application operation is in flight. Preserve existing root adapter locally.
3. Install adapter modules atomically under a root-owned version directory; keep
   the stable sudo boundary with an exact operation allowlist. No arbitrary path,
   user-controlled code, broad sudo or insecure ownership exceptions.
4. Verify installed hashes and read-only adapter readiness against current .37
   health. Activate the new signed updater through its managed layout, preserving
   stable self-update channel, alpha group and automatic updates.
5. Restart updater; verify readiness advertisement, persisted identity, heartbeat
   and unchanged product process/health. If adapter activation fails before any
   product transaction, restore the verified previous adapter and updater. Do not
   downgrade the adapter across a nonterminal application operation.

Only then may the separately signed SiemCore security artifact be offered through
the existing cascade. No SSH product installation and no host rebuild are needed.
The 1.16.1.26-r1 kit and original .37 receipt remain immutable historical artifacts.

## Required fixtures before removing guards

Updates: old/missing capability; wrong type/identity; adapter tamper; predecessor
mismatch; signed target/pinned key verification; no Normal behavior change; retained
pointer reconciliation; report failure vs accepted/restored; restart capability
readiness; adapter migration failure/rollback without product execution.

Joint product/root fixtures: clean .37-to-target upgrade; missing/valid/wrong UI
password behavior; exact accepted replay; crash before/after intent/staging/switch/
acceptance/restore; target health failure and verified predecessor recovery;
failed recovery blocks; repeated reconciliation; changed operation rejected;
config/TLS/password/identities preserved; linking/authority evidence rejected;
no bootstrap replay, database operations or public dependency fallback.

After fixture acceptance: one signed isolated alpha upgrade on the current
Observer; verify application health, UI protection, journal, restart/heartbeat and
five-minute stability. No other fleet assignment, hold or release target changes.


## Accepted update chaining

After accepted health, a subsequent newer signed release may create a new operation.
The root adapter retains the original bootstrap policy and every prior operation.
Its predecessor must exactly match the target of the previous root-journal accepted
operation, with identical installation, machine and pinned signing identity. New
admission independently checks predecessor health and both signed archives. Neither
a version directory nor updater-owned state can establish this chain. A nonterminal
or blocked root operation cannot be superseded. Installed CLI fixtures exercise two
consecutive updates (.37 -> .38 -> .39 synthetic signed services).

Current product recovery remains conservative: failed targets isolate management
rather than attempt an unqualified predecessor restoration. Successful routine
updates require no additional operator approval after maintenance activation.
