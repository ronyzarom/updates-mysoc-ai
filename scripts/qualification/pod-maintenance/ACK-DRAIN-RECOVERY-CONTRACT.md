# ACK-v2 explicit drain recovery (source-only contract)

New opt-in protocol/capability `pod-maintenance-ack-drain-recovery-v1`; signature domain `mysoc-pod-ack-drain-recovery-v1\n`. Existing drain-v1 and artifact recovery-v2 wire behavior remain separate. This grant authorizes drain only, never artifact execution, clearing barriers, changing assignment, or extending the original deadline.

Original binding must use `pod-maintenance-ack-v2`. Updates requires a locally durable nonzero generation and phase barrier-acknowledged, draining, or acknowledged. An intent with a lost prepare reply is NOT eligible: discovery cannot manufacture the missing durable ACK.

Use existing DrainRequest/Response/Claims fields and actions status, authorize-drain-recovery, resume-drain. Add optional JSON `ack_proof` to all three, REQUIRED for this protocol, with exact positive integer `prepared_revision` and `assignment_revision`. prepared_revision must equal original generation. Proof is pinned on first authenticated status and must match on later responses and the signed grant. Existing captured_owner requirements remain: exact captured node/owner/watchdog generations; ownerless recovery is unsupported and must fail explicitly.

Product proof required: root operation and update-prepared have no TTL; exact binding/generation; prepared revision equals operation generation; assignment revision unchanged; no conflicting takeover/cloud/resume authority; captured owner/watchdog identity unchanged. Product must check these atomically on mutation, not merely echo requested epochs. Status is authenticated read-only discovery with no authorization fields and may return prepared, draining, blocked or paused. Existing freshness bounds apply (<=60 seconds); paused requires <=5-second complete node proof.

Before resume, Updates durably stores discovered proof and grant ledger, obtains fresh authorize-drain-recovery for exact epochs and checks grant expiry/revocation. Product authorization must not itself drain. Only resume-drain may create drain pointers from prepared, under the new scoped grant, preserving operation and original deadline. A separately bounded grant window controls recovery; it never rewrites the original deadline. Existing lease/watchdog permission must not be revoked until explicit resume.

Terminal paused proof requires processing_stopped, ingress_stopped, quiescent, watchdog_retired, watchdog_exited, lease_released and node_evidence_observed_at <= observed_at, both no older than five seconds. No owner adoption or implicit takeover. Lost replies retry same binding/generation/epochs and retain signed grant history. Prepared/assignment ABA, expired/revoked/superseded grant, incomplete proof or protocol fallback fail closed.

After terminal paused, a SEPARATE existing artifact recovery-v2 grant may authorize target/predecessor execution. Updates can accept the retained early ACK-v2 phase only alongside this protocol's complete durable paused proof. The original phase/binding/deadline is not rewritten to manufacture eligibility. This is explicit entry validation, not use of artifact recovery authorization to permit drain.

No advertisement/readiness enablement, release, host changes or deployment is included. Product must implement matching protocol/proof and explicit opt-in before joint tests.

## Implemented Updates integration

Set existing `pod_maintenance.drain.protocol` explicitly to the new protocol; existing pinned observer key, protected adapter command and authorization file configuration still apply. Omission or old protocol never enables ACK-v2 recovery. Merely configuring the transport does not preempt an unexpired normal ACK handshake when no recovery authorization file or retained drain journal exists.

The existing private `drain-v1.json` sidecar stores an explicit protocol discriminator plus the new proof and grant ledger; its filename is not protocol negotiation. Old rows are not reinterpreted. Both original operation and sidecar remain available for recovery/archival. All mutation requests carry the pinned ack_proof. A protocol-specific status response from the protected adapter is required; unsupported protocol errors cannot be treated as legacy permission.

Artifact recovery-v2 early-phase entry additionally requires this terminal proof to be fresh at first entry, plus its own separately signed grant. Later restart uses the retained recovery journal and fresh observer authorization, rather than treating old drain evidence as continuing permission. Next-operation archival retains and validates the protocol-specific proof.

Qualification driver supports explicit JSON `drain_protocol`; default remains old drain protocol. No new release number is allocated. Canonical public test-only vector: `ack-drain-recovery-v1.fixture.json`, using 3.3.152.35 -> 3.3.152.36 and fixed 2030 timestamps. It is not an installable archive or a production authorization.

Updater tests cover separate signature domain, expired/future/revoked grants, wrong or changed epochs, missing durable ACK, legacy fallback refusal, incomplete/stale paused evidence, actual subprocess crashes at status/authorize/resume and durable checkpoints, unchanged original deadline/phase, and separately authorized artifact-recovery handoff. Product implementation and joint real-adapter qualification remain required.

## Verification

- Full `go test` suites for podmaintenance, updatersim and qualification driver passed.
- Full `go test -race` suites passed: podmaintenance 117.101s, updatersim 14.378s, qualification driver 15.684s.
- Canonical fixture/retained epoch focused race rerun passed (4.523s).
- Tests use disposable local subprocess/adapter fixtures. Product matching implementation and native/joint execution evidence remain separate; no live qualification claimed.
