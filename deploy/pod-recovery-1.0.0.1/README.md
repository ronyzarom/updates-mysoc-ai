# Signed staged recovery runner — local review candidate

Not installed, signed or executed on the pod. Exact new signed product artifact
qualification is required before any live recovery execution.

## Scope and wire

The runner accepts a root-private intent at
`/etc/siemcore-pod-recovery/intent.json` with exactly: schema(integer1),
product(siemcore), operation_id, pod_id, node_id(a/b), machine_id, version,
artifact_sha256, maintenance:{operation_id,generation(positive integer)},
expected_owner(empty string), activation_allowed(false).

Root-private `release.json` has exactly product, version, sha256, signature,
public_key, source_commit(full40 hex), architecture(amd64/arm64), artifact(absolute
protected local archive path). The existing pinned Ed25519 verifier checks the
retained archive each time; signed MANIFEST product/version/architecture/build
commit must match, and runtime SHA is computed from the verified bundle.

The only product executable is the signed `updater/recover`, with stage or
verify-staged as its sole argument. Environment is freshly constructed:
PRODUCT, VERSION, CURRENT_DIR, VERIFIED_ARTIFACT_SHA256, RECOVERY_INTENT plus
fixed PATH/HOME/LANG. It never runs a caller-supplied command or downloads images.

Each successful phase must emit exactly one stdout line, max8192 bytes:
`MYSOC_RECOVERY_RESULT_V1:` followed by JSON mirroring the exact intent plus
status=staged-paused, runtime_sha256, prerequisites_verified=true,
unchanged_state_verified=true, application_health_verified=false.
Duplicate/extra keys, extra lines, malformed/nonfinite JSON, wrong types or
bindings fail closed. Logs go to root-private stderr files.

Stage always runs verify-staged afterward. The Updates outcome is
recovery-staged, never normal application health or update success. The normal
current-release pointer and original product update journal are untouched.

## Supervision and normal-update exclusion

Main holds updater cycle, normal boundary and dedicated runner locks. Worker
holds the normal boundary work lock; unit identity is persisted before launch.
A delayed worker must match the exact pending unit and action. Native systemd
KillMode=control-group, RuntimeMaxSec(600 stage/120 verification), SendSIGKILL and
TimeoutStopSec10 bound descendants, including separate process sessions.

Retry revokes the pending unit before stopping it and requires inactive/failed
(or absent) unit plus empty cgroup before continuing. Failed quiescence retains
the draining-unit identity; it cannot become staged success. Root retains the
intent, release bytes, journal and evidence. An operation with different intent
or release cannot replace an existing journal.

`normal_boundary.py` is a proposed boundary1.0.0.3, based on deployed1.0.0.2.
It refuses every ordinary apply/health/rollback when the dedicated recovery
journal exists—even if malformed—so parent death cannot reopen normal updates
while a supervised recovery worker remains. The runner requires that exact
installed guard. `reconcile.py` and `incident.json` accompany the normal boundary
for compatibility with B's preflight-refused receipt.

No handoff/removal of this recovery guard is implemented yet. That is
intentional fail-closed behavior: staged-paused must remain protected until a
separately reviewed product-owned activation/readiness and Updates handoff
contract is accepted. Do not delete the journal or lift holds to bypass it.

## Remaining qualification gates

- Review product draft and exact new version/commit/artifact/architecture.
- Package/install guard+runner through signed, exact-host OS Config prerequisites.
- Native signed-product fixture: successful staging/no starts, wrong identity,
  maintenance loss, signature/checksum/manifest mismatch, interruption/timeout,
  remaining descendants, retry and changed-state refusal.
- Validate post-activation handoff contract before returning to ordinary updates.

Local runner tests use verifier/systemd adapters; no native recovery success is
claimed. Product recovery tests are separate. No DB copies/restores, activation,
IP routing or product hold changes are performed by this local implementation.
