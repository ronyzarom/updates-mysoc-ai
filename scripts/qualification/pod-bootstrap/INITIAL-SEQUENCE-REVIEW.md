# Fresh prerequisite and initial sequence review — 2026-09-17

Reviewed product `pod_data_runtime.py` subscription-state SQL helper,
`podreplication/initial_sequences.go` and restricted sequence grants in reader.go.
No Updates contract conflict for a fresh signed candidate.

The helper returns boolean equality only, fixes search_path to pg_catalog,
revokes PUBLIC access, and checks the named subscription in the current database,
session-user ownership, exact connection/slot/enabled/publication/runasowner.
Restricted SELECT/UPDATE sequence grants are derived from approved table-owned
sequences rather than all sequences in the schema.

The sequence stage persists pause intent before disabling its bound subscription,
waits for worker exit, and reauthorizes sequence preparation and resume. Original
seed fingerprint and subscription OID are retained; this grants no processing
permission or alternative operation identity.

Independently ran11 product Python runtime tests: PASS. Reviewed product-provided
native PG16 full-schema/owner evidence at initial-sequences-native.log; this review
did not rerun that database fixture. The fresh matching artifact/kit must qualify
actual helper installation, restricted readiness and interrupted sequence-stage
reconciliation. Existing v1 fixture/evidence is unchanged and cannot qualify new
helper bytes. No live SQL, retrofit, deployment or teardown was performed.
