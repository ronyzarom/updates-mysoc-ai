# Narrow independent-node bootstrap health repair

Implemented and qualified on node A on 2026-09-18. See
[the signed native repair evidence](verification/node-a-bootstrap-repair-20260918/README.md).
Scope: recover original incomplete node A bootstrap `.40` after Docker29 returned
`CAP_DAC_OVERRIDE`/`CAP_NET_BIND_SERVICE` instead of their unprefixed spellings.
Actual image, binary, identities and effective capability set match. No bypass
of capability checks, artifact replacement or ordinary node upgrade is allowed.

## Existing updater route

Updater1.16.1.28 already retains and automatically retries the exact `.40` signed
staging through `/usr/local/sbin/siemcore-apply-update apply`, then health. Keep
that route and both original archive/module bytes unchanged. A signed, narrowly
bound root-maintenance component supplies the compatible health verifier and
journal reconciliation. It is installed as updater host preparation; product
repair executes only on the updater's next normal retry, never manual apply.

## Signed authorization

Protocol `pod-node-bootstrap-health-repair-v1`, action `docker-cap-prefix-v1`,
unique repair_id. Bind exact product/version, original archive SHA/signature and
trusted key fingerprint, machine/install/updater/node IDs, policy/config hashes,
runtime image/binary hashes, retained PG/Redis/management IDs, original root hook
hash and reviewed repair module hash. Sign component and authorization through
the existing fleet-key signed kit manifest; no new trust root or unsigned override.

Root verifies kit provenance and original signed `.40` archive independently.
Require node-unlinked, original installing journal, retained original inputs,
matching live container identities, no processing/authority/POD/link readiness.
Product-owned readiness uses all original checks with only optional `CAP_`
canonicalization before comparing the exact allowed set. Reject extra capabilities,
identity drift, TLS/auth failure, schema/data failure and any unrelated operation.

## Reconciliation

Use the existing root hook lock; no nested acquisition. Retain an original control
journal snapshot/hash and dedicated operation receipt under
`/var/lib/siemcore-bootstrap-repair/<repair_id>/`. Atomic phases:
prepared → verified → journal_committed. Only the original completion fields may
change: status=complete, installation_state=installed-unlinked, management_id.
Artifact/policy/config bindings remain immutable. No Docker up/down/recreation,
DB initialization/migration, configuration rewrite, management replacement, or
new active assignment. A retry rechecks bindings and resumes the receipt safely;
a crash after journal completion must reconcile without repeating mutation.

Subsequent root health for this exact operation uses the pinned repair checker
and validates its durable receipt. Other roles/versions retain existing behavior.
Updater success follows only after root apply+health; version/report reconciliation
continues through the ordinary updater. Branding changes are deferred.

## Qualification required

Product owns root adapter, repair logic and tests: exact admission, CAP aliases,
extra-cap rejection, linked/active rejection, changed IDs/config/artifact/key,
wrong machine/node, completed unrelated journal, concurrent lock, interruption at
each atomic boundary, repeated repair with unchanged containers/data, and retained
health validation. Updates owns signed-kit packaging, immutable predecessor checks,
component installation with updater-only restart, and final cascade/state/health
report. Normal and Observer paths must remain unaffected.
