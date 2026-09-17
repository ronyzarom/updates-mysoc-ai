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

## Four-endpoint draft review

Reviewed SiemCore's `pod-initial-activation-v1.md` proposing authenticated
prepare/status/authorize/complete endpoints. The flow is compatible, but exact
wire schemas are still needed before implementing a caller:

- Specify strict request/grant/status/ACK/completion/error fields, types, phase
  enum, response/body limits, evidence freshness and exact signature encoding.
- Distinguish bootstrap generation, assignment generation and intent ID.
- Clarify grant expiry versus renewable runtime authority after completion.
  The draft's statement that expired grants never permit processing must not
  accidentally impose a ten-minute lifetime on a successfully completed POD.
  Incomplete expired activation must not silently renew permission.
- Resolve expired identical prepare retries: status remains read-only; mutation
  must refuse expired grants unless returning an explicitly read-only persisted
  acknowledgment. No retry extends expiry.
- Provide a typed, identity-bound absent-intent status. A generic404 must not be
  treated as proof that a lost prepare never committed.
- Define idempotent authorize/complete responses so lost responses reconcile the
  original committed result rather than repeat role-IP operations or assignments.

These are contract clarification items, not additional user approval requests.
No endpoint, production orchestration or kit capability was enabled by this review.

## Revised wire draft

The revision resolves the earlier caller blockers: exact domain-separated signed
payload bytes, distinct bootstrap/preparation/assignment revisions, verified
HTTP200 absent status, refusal of expired mutations, and atomic completed handoff
to ordinary renewable authority. Accepted as a basis for source-only caller work;
this does not enable endpoints or delivery.

Before freezing fixtures, specify exact infrastructure operation record fields
and provider values, plus phase-specific null/zero/positive/receipt invariants.
Updates treats any `processing_allowed` observation as informational, never an
instruction or permission to enable processing. A completion receipt proves
historical verified completion, not present application health. Actual HTTP,
authorize/controller/completion and race qualification remains outstanding;
durable prepare-only tests do not establish those capabilities.

## Canonical quorum principal packaging gate

Updates' kit does not independently author quorum RBAC; it delegates application
installation to the signed SiemCore artifact. Before schema4 delivery, qualify
that artifact's `pod/quorum/configure-auth.sh` with canonical controller
principals `1` and `2` and matching issued certificate identities. Adding grant
calls alone is insufficient if permission-selection branches still recognize
only legacy `a/b` names. Verify the required owner/cloud-operation and bootstrap
reads using actual authenticated principals, including watchdog intent reads.
Controllers/updaters must remain unable to write Observer-owned assignments or
activation authority. Preserve legacy compatibility explicitly where required.

The existing source review found legacy-only grant calls/permission branches;
SiemCore owns their correction. Updates will verify the complete signed helper
bytes in the matching candidate/kit, not introduce a separate RBAC implementation.
Schema4 remains disabled and no live quorum permissions were modified.

Fresh independent-data updater quorum credentials are accepted as a separate
identity namespace: node1 -> CN `updater-1`, node2 -> `updater-2`, witness ->
`updater-observer`. They do not replace registered updater/machine IDs or pinned
bootstrap-listener client certificates. Provision distinct scoped quorum client
credentials; do not fall back to controller or Observer authority credentials.
Updated artifact source requests read-only grants for these new principals,
including owner-resume/cloud-operation keys. Native tests must verify effective
permissions and repeat-run behavior because etcd grants/role memberships are
additive: successful scoped reads, rejected protected put/delete/transactions,
and rejected access to other PODs. This source review does not claim RBAC test
completion or enable schema4 delivery.

## Activation HTTP implementation review

Reviewed activation_server.go/activation_snapshot.go. Positive boundaries:
disabled by default, both independent readers required, original node1 mTLS,
authoritative post-mutation reread, immutable completion evidence for historical
routing, and unavailable response instead of invented provider-operation success.

Caller conformance findings sent to SiemCore:

1. Preserve typed errors: expired grant is409/grant_expired; quorum/observation
   failure503; do not collapse backend/readiness failures into retryable409
   phase_conflict or expired signatures into generic403.
2. Enforce endpoint-specific field presence. Pointer nil checks accept forbidden
   explicit null fields, contrary to the exact wire field sets.
3. Include registration comparisons in the final absent-intent proof transaction;
   the observed implementation drops those comparisons between initial registration
   verification and its second barrier/absence transaction.
4. Reconcile completed replay after expiry with the contract: either status-only
   reconciliation or an explicit no-effect historical replay exception. Current
   code checks the completed grant at issue time while the text refuses expired
   mutations.

These are source review findings, not claims of exploit reproduction or completed
HTTP qualification. Unknown provider IDs remain unavailable, never verified absence.
