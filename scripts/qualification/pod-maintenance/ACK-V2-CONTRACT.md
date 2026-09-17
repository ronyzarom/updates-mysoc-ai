# ACK v2 contract (source implementation; not deployed)

Capability and binding.protocol: `pod-maintenance-ack-v2`. Existing `pod-maintenance-v1` stays unchanged. Explicit local protocol opt-in is required. A release requiring ACK v2 must declare it in updater requirements; absence of local opt-in or adapter capability rejects the offer/execution, never falls back.

Same Request/Response binding, generation and permission_expires fields as v1. No new trust root. Product adapter uses pinned authenticated observer communication.

1. Persist original intent before I/O; require exact ACK v2 capability.
2. `prepare`: returns `prepared`, exact original binding, nonzero generation, unexpired permission. Observer has durably persisted the barrier, suspended takeover, preserved assignment; **no drain/stop/restart occurs**. Existing owner lease/watchdog permission remains intact. Product uses a separate non-expiring prepared barrier, not the pointer that triggers controller drain. Retry same operation is idempotent.
3. Updater durably journals generation and phase `barrier-acknowledged` using file fsync, atomic rename and directory fsync. Only after successful persistence may it call `start-drain` with exact original binding/generation.
4. `start-drain`: idempotent, returns `draining` or `paused` with same identity/generation and unexpired permission. Lost reply retains barrier-acknowledged; retry sends the same start-drain. No new operation/deadline is generated.
5. A draining response persists local `draining`; subsequent runs use `status` until `paused`. No apply in prepared/draining. Paused saves existing local `acknowledged`, then the existing status/apply/recover/health/complete/acceptance flow applies.
6. Failure/expiry preserves journal and observer barrier. No automatic clear, reassignment or legacy fallback. Recovery requiring separate authorization is not inferred from a prepared acknowledgment.

Product must accept the new binding protocol only with explicit support and guard controller drain on explicit start-drain, never merely barrier existence. Returning prepared after start-drain or regressing a draining journal to prepared fails closed. Existing v1 pending journals stay v1; never rewrite original bindings.

Tests cover missing capability, bad binding/generation/expiry, durable ACK ordering, lost prepare/start replies, restart from ACK/draining, and unchanged v1 behavior. This file defines an in-progress contract, not deployment readiness.

## Updates integration

Explicit local opt-in: `simulation.filesystem.pod_maintenance.protocol: pod-maintenance-ack-v2` (within the existing configuration hierarchy). Omitted protocol keeps v1. `advertise_capabilities` remains off by default, and fresh product readiness proof is still required. ACK-v2 is filtered out of offer capability advertisement unless locally selected. Releases requiring strict ordering declare `pod-maintenance-ack-v2` in `updater_requirements.capabilities`; legacy-only clients are ineligible. No release version is allocated by this change.

The immutable operation binding retains the selected protocol through process restart. A pending ACK-v2 binding cannot be downgraded to v1 by changing configuration. Automatic legacy drain discovery is skipped for ACK-v2, so it cannot bypass the prepared acknowledgment gate. Separate expired-operation recovery remains a qualification requirement; no recovery grant is fabricated.

Product has confirmed protocol/action alignment. Its current start-drain may wait for paused and return an error rather than draining; Updates safely retries the same action on that error. Supporting a draining response also permits bounded polling in future.

`pkg/podmaintenance/ack_v2_test.go` includes actual subprocess exits after prepare persistence and on start-drain entry, same-operation restart, lost-response retries, invalid acknowledgments, journal persistence failure before drain, waiting for paused, deadline retention, and downgrade refusal. Existing v1 tests remain in the same suite. These fixture tests establish updater ordering; product native tests and joint real-adapter acceptance are separate evidence.

## Verification (2026-09-17)

- `go test ./pkg/podmaintenance ./pkg/updatersim ./pkg/updatecapability`: PASS after final changes.
- `go test -race ./pkg/podmaintenance ./pkg/updatecapability ./pkg/updatersim`: PASS.
- Final focused rerun after persistence-failure test and startup guard: `go test -race ./pkg/podmaintenance -run ^TestAckV2`: PASS (2.300s).
- `git diff --check`: PASS. No live qualification or deployment claimed.
