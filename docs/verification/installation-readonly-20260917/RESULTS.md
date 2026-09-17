# Read-only installation identity

Installation identity is emitted from persisted updater state as Normal/POD, with
immutable pod/node identity where applicable. Active/standby labels are separate.
Full and delta relay projections preserve the identity. Direct heartbeat updates,
upserts, and cascaded upserts retain the first recorded installation object using
existing heartbeat JSON. Later omitted/conflicting identity cannot rewrite it.
This is administrative immutability; it is not remote hardware attestation.

Dashboard: lock icon and read-only Normal/POD badge, no edit input. Legacy clients
without recorded identity show Not reported, not an inferred Normal. Identity
validation rejects malformed class/pod/node combinations; no schema migration.

Passed:
- Go race suites for updater, licensing and shared types.
- Disposable local PostgreSQL integration tests: both Normal/POD, direct update,
  direct upsert, relay rollup, with/without update attempts, omitted/conflicting
  identity, while continuing to accept fresh heartbeat telemetry.
- Explicit active/standby transition preserves class/pod/node identity.
- Dashboard typecheck and 9 component/API tests. A pre-existing API fixture's
  artifact-kind array was annotated as literal types to unblock typecheck.

No deployment or fleet changes. Test database contained synthetic records only.
