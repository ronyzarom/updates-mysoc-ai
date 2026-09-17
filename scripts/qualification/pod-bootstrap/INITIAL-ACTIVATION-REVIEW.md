# Initial activation proposal review — 2026-09-17

Conceptually compatible with Updates' staged installation boundary. This is a
proposal review, not acceptance of an implemented API or enablement of activation.
Normal installation/update paths remain unchanged.

The Observer owns initial assignment and authorization; the controller owns
fenced lease-driven role/routing transitions. Updates never clears a barrier or
infers completion from data, installer, or paused-management receipts.

Required explicit contract details:

- Atomic compare-and-swap of the original registry hash, operation, generation,
  expected absence of assignment and barrier ownership into a unique pending
  intent. Recording node1 assignment is not processing permission. Release only
  that intent's bootstrap admission barrier while retaining independent
  maintenance/no-automatic-takeover protection.
- Domain-separated initial-activation signatures, bound to action, registry,
  node and artifact, with short expiry and durable replay/idempotency rules.
  Retries must not reset generations or renew authority implicitly.
- Define reconciliation after authorization expiry for committed pending work;
  fresh processing permission still requires valid authority and lease.
- Arm watchdog before processing. Peer pause observations do not replace fencing
  and permission checks. Endpoint/routing verification precedes processing.
- Persist cloud operation IDs. A failure after role-IP transfer remains incomplete
  and must reconcile actual ownership; no implicit failback or new assignment.
- Completion is a distinct typed receipt for the same intent, generation and
  artifact with fresh runtime, routing, MySoc and SSO evidence. Neither staged
  status, activation authorization nor observed processing alone is completion.

Next review input: concrete endpoints, request/ACK/error/receipt schemas and
freshness limits, followed by integrated failure/retry tests. All new capability
remains disabled until qualification; the existing local fixture is retained
paused and invitations expire normally.
