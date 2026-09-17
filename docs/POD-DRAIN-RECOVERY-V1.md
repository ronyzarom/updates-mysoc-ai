# Interrupted-drain recovery v1 — canonical contract, execution disabled

Capability/protocol: `pod-maintenance-drain-recovery-v1`. This is separate from
maintenance-v1 and recovery-v2. Private updater 1.16.1.25 does not implement or
advertise it. These fixtures authorize no real operation.

## Read-only status

Action `status` takes protocol, exact original v1 binding and maintenance
generation. Generation zero is permitted ONLY for authenticated read-only lookup
after a lost begin response: observer resolves the exact full binding under the
pinned mTLS registry identity and derives its already-persisted positive original
generation. It never allocates a generation or creates a barrier. Response repeats
that exact binding and returns the positive generation. Foreign identity/binding
or absent durable operation is refused. Updater may persist the discovered value
only after authenticated proof; a status timestamp or echoed request is not proof.
Every authorize/resume mutation and signed grant still requires the positive exact
original generation; zero never authorizes mutation. No v1 state widening. It remains read-only after the original deadline and after a recovery
authorization expires or is revoked. Authentication/registered identity is still
required. Status may return phase `draining`, `paused`, or `blocked`; it includes
captured_owner, original_deadline_expired, observed_at, valid_until and current
node evidence. Status validity is at most 60 seconds and only describes freshness
of that response. **status.valid_until never authorizes mutation.** Unknown host,
watchdog or lease state is reported blocked/unknown; it is not converted to paused.

Only `paused` is a successful terminal state for this extension. A completed
product maintenance operation is outside its scope and cannot be reopened.

## Explicit authorization

Envelope: standard-base64 `payload_base64` and `signature`. Ed25519 verification
uses the independently provisioned observer authorization key, over UTF-8
`mysoc-pod-drain-recovery-v1\n` followed by the exact decoded payload bytes.
No reserialization for signature verification. Decode strict bounded JSON after
verification: reject unknown/duplicate fields, trailing JSON, non-integer
unsigned generation values and payloads over 32 KiB.

Exact claims and signed vectors: `fixtures/pod-maintenance-drain-recovery-v1/canonical.json`.
Fields: protocol, authorization_id, binding (unchanged v1 binding), generation
(original maintenance generation), captured_owner {node_id, owner_generation,
watchdog_generation}, action=`resume-drain`, issued_at and expires_at (RFC3339,
maximum 15 minutes). Captured owner/watchdog generations come from durable state
recorded when the original drain began; neither requester nor updater may infer
or replace them. Missing capture evidence refuses mutation. Original operation
ID, identity, binding, deadline and maintenance generation remain unchanged.

Actions permitted: status, authorize-drain-recovery, resume-drain. The grant does
not authorize product apply/recover/rollback/complete, assignment change, new
maintenance, barrier clearance, traffic activation, or database operations. A
separate recovery-v2 authorization is required for later product recovery.
V1 or recovery-v2 grants cannot be accepted under this domain/action.

Authorization and resume request/response envelopes are literal fixtures in the
canonical file, including paused success and blocked unknown termination.
`permission_expires` is bounded by the signed grant; read-only `valid_until`
remains a snapshot freshness field. Transport/adapter replies are authenticated,
not authorized merely by their booleans.

## Checks before every mutating step

Each resume-drain invocation and each watchdog/lease mutation independently
checks authenticated registered identity, current quorum/barrier, immutable
original binding, captured owner/watchdog generations, grant scope/expiry and
revocation. Independently collected node evidence must be no older than five
seconds, not future-dated, and must bind the same host and generations. A stale
status response or requester-supplied booleans cannot substitute for this probe.

Stop/drain processing and ingress while preserving DB/Redis. Before retiring a
watchdog, prove processing and ingress stopped and quiescent. Never retire a
watchdog protecting active processing or a different/newer generation. Persist
the generation-bound retirement receipt and independently verify actual watchdog
exit before conditionally releasing only the captured owner lease. Never release
a different/newer owner. Recheck the barrier/identity around probes and use exact
CAS conditions for authority changes. Emergency isolation remains armed if proof
is absent. Expiry, timeout, unknown termination or inconsistent evidence retains
the barrier and reports blocked; no automatic release or guessed success.

## Durable retry and replay

Before mutation persist authorization ID, SHA-256 of exact decoded signed payload,
signature, original binding/generation/capture, phase and revocation/supersession
history. Reusing an ID with different bytes is rejected even if newly signed.
Retries of the same unrevoked/unexpired authorization may resume only the same
operation, after all fresh checks; recorded completed substeps are reconciled,
not blindly repeated. Revoked/superseded IDs remain rejected across restart.
A replacement authorization has a new immutable ID and exactly the same scope;
it does not reset evidence or the original deadline. Replay history cannot be
cleared by retry or an updater restart. Revocation is checked online at every
mutation, not just once at authorization acceptance.

Crash recovery may read status with expired credentials only insofar as the
transport identity is still valid; an expired recovery grant itself cannot
authorize any mutation. Unknown termination retains the barrier until independently
resolved. Paused acknowledgement must attest verified processing/ingress stop,
captured watchdog retirement/exit and exact lease release with fresh quorum proof.

## Next-operation clarification

An accepted ACTIVE node may legitimately retain its current processing lease and
watchdog. `authorize-next-operation` does not require absence of that valid lease.
It requires unambiguous accepted state and no unresolved prior maintenance work.
The next maintenance operation then captures and drains that owner using the
normal protocol. Archival never disarms a watchdog, releases ownership, or stops
services by itself.

## Implementation boundary

These are canonical protocol/signing fixtures plus validation vectors. No drain
executor, live grant, capability advertisement or deployment is supplied here.
Native product/observer/updater integration and crash, expiry, revocation,
newer-owner/watchdog and emergency-isolation tests remain necessary.

## Updates coordinator implementation (P1, opt-in source only)

`pkg/podmaintenance/drain.go` provides `DrainCoordinator.Discover` and `Run`.
The caller must supply the protected pinned observer transport. The production
SiemCore adapter argv is `/usr/local/libexec/siemcore pod-drain-adapter --config
/etc/siemcore-pod-controller/drain-adapter.json`, followed by the action. Nonzero
exit (including the service's generic HTTP 409 refusal) always fails closed.
The observer owns the authoritative persistent revocation ledger and rechecks it
on every mutation; the updater does not infer a revocation reason from HTTP 409.

Discovery first records `drain-v1.json`, blocking ordinary v1 fallback. Authenticated
exact-binding evidence is persisted there before the original journal's generation
is filled in. Binding, original deadline, and original phase remain unchanged.
A retry uses the retained positive generation; it never begins a new operation.
Signed grants are retained by ID with exact decoded-payload hash, signature, and
supersession history. Recovery only authorizes/resumes drain; paused is its only
successful terminal state. Paused status reconciliation works after grant expiry.
No artifact application, barrier clearing, completion, activation, or promotion
can be invoked by this coordinator.

The isolated qualification driver accepts `drain-discovery` and `drain-recovery`
modes with the same adapter argv and canonical authorization envelope. Process
tests in `drain_test.go` exercise actual coordinator/adapter subprocesses, SIGKILL
at durable checkpoints and lost responses, retries, revocation refusal, expiry,
invalid discovery, and zero-generation mutation rejection. These use a synthetic
adapter and are component evidence, not real-host qualification.

Capability advertisement, updater startup integration, and deployment remain OFF.
A discovered lost-begin journal retains phase `intent`: a subsequent separately
signed v2 handoff and drain-evidence archival integration are still required before
that operation can apply a product artifact. The private `.25` binary is unchanged.
