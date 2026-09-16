# Proposed signed staged recovery contract

Status: product/Updates alignment, not implemented by boundary 1.0.0.2.
Normal updater apply/health must not accept a paused/non-serving result as healthy.
B's exact preflight-refused reconciliation closes that incident only; no product
transaction is synthesized and no application retry is authorized.

A separate reviewed recovery runner must verify/retain the signed candidate and
execute its `updater/recover stage` and `updater/recover verify-staged` entrypoints
under bounded systemd supervision and the existing execution locks. No public
registry or unsigned/manual binary substitution is implied by this contract.

The protected root intent must bind schema version, unique operation ID, product,
version, source commit, artifact SHA-256, pod ID, node ID, immutable host identity,
durable maintenance operation and generation, expected owner absence, and
activation_allowed=false. Exact field names require joint review before shipping.
No caller-supplied shell command or arbitrary executable path is permitted.

The product stage action may install the signed runtime and required static
configuration only while it independently verifies the protected intent and
exclusive durable maintenance. It must preserve application/database data and
keep processing and autonomous controller activation disabled. Any prerequisite
or barrier failure fails closed and retains recovery evidence. Updates does not
implement product provisioning or infer readiness from file/directory existence.

Verify-staged must emit a strictly bounded, exact receipt with status
staged-paused and matching intent/host/artifact/maintenance identities, actual
runtime version and required prerequisite results. The status is recovery-staged,
not application healthy, rollback success, or resumed availability. A malformed,
missing, stale or mismatched receipt cannot authorize resume.

Activation/data-service readiness and explicit resume remain separate product
operations after both nodes qualify. Existing product holds remain until that
coordinated acceptance. A stopped node with product auto-update still enabled
requires explicit handling before startup; do not rely on its offline state as
a durable update hold.
