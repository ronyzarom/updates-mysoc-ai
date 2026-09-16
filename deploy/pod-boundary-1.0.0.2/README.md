# Boundary 1.0.0.2 — local review candidate

Not signed, installed, published or qualified on a native host. No live journal
has been edited. 1.0.0.1 remains the deployed implementation.

## Exact incident reconciliation

`reconcile.py` is a root-only operation, intended for the already authorized
GCP OS Config management route after review and disposable-host qualification.
It takes no caller-selected identity, artifact, log path or reason. It accepts
only B / bezeq-pod-test / transaction e7d0fe81a55941b1a3ba99567f1582cf,
3.3.152.33 candidate and 3.3.152.30 predecessor with their pinned digests and
existing signing key. It checks completed bootstrap, protected policy/machine
identity, current signed predecessor, both retained signed bundles, exact
historical apply/compensation requests and reviewed log hashes. It never runs
an artifact during reconciliation.

All three normal cycle/boundary/worker locks must be acquired nonblocking.
Both historical units must be inactive/absent with empty cgroups. No unit is
stopped to manufacture this evidence. Existing or dangling product transaction
paths fail closed. Unknown additional candidate invocations fail closed.

The original journal and evidence hashes are preserved privately before the
terminal write. Logs and retained archives remain intact. The terminal outcome
is `preflight-refused`, with compensation_accepted=false,
rollback_dispatched=false and health_verified=false. It cannot grant health or
rollback success. A later normal apply must still have the exact signed .30
predecessor and intact reconciliation record; .33 will encounter its normal
compatibility check again. This does not solve the current availability outage
or authorize a retry, service start, maintenance change, DB action or IP move.

## Evidence limits and mandatory review

The log pins were derived from the captured diagnostic tails. The operation
compares full local files against those pins; if any tail was truncated or the
file changed it MUST refuse. Never loosen this to substring matching. These
root-owned logs and requests are historical evidence, not a cryptographic
execution attestation. Product review must establish that the exact signed
.33 path to line 369—including sourced shell helpers, greenfield --requested,
and status/peer/health calls—performs no application/DB mutation. An absent
transaction alone is never sufficient proof. The operator must review this
claim and its evidence before packaging; this local candidate is not approval.

## Bounded reporting

Boundary phase errors persist at most three typed records with transaction,
executed artifact, unit, phase and fixed reason code; raw exception text, output,
environment and arguments are excluded. Reconciliation emits the two explicit
historical failure reasons and false rollback/health flags as bounded JSON.
This is a local reporting interface only: authenticated cascade transport,
server persistence and UI display remain the separate diagnostic proposal.
No claim that these fields are already visible in fleet reporting.

## Review and qualification path

1. SiemCore reviews no-mutation path and Updates reviews evidence checks.
2. Test signed .33/.30 retained archives plus exact requests/logs on a disposable
   native systemd host. Include populated descendant cgroup, lock contention,
   stale/different host or transaction, tampered signature, extra invocation,
   mismatched log, product transaction present, and crash between evidence and
   terminal persistence. Current unit tests use signature/systemd adapters.
3. Build a versioned package with these files and sign using the existing
   prerequisite signing domain. Use exact-host GCP OS Config; keep product
   auto-update held and acquire its actual cycle lock. Stage files before
   reconciliation; atomically switch the wrapper only after verified terminal
   evidence. A crash before wrapper switch leaves old boundary rejecting new
   apply, which is fail closed. The new `install.py` stages the versioned files and supports retry after a
   terminal-write/wrapper-switch interruption. Its local tests pass; native
   qualification and the signed OS Config delivery envelope are still pending.
   Do not reuse 1.0.0.1 installer (it rejects existing journals).
4. Independently read back terminal journal, preserved evidence and package
   hashes. Keep B product hold until product-owned recovery and next candidate
   approval. Never clear a journal or fabricate a paused receipt.

Local tests:
`python3 -m unittest discover -s deploy/pod-boundary-1.0.0.2 -v`

Current local result: 31 tests pass, including exact evidence checks, extra
execution/environment injection refusal, populated cgroup refusal, installation
drift, and interruption after reconciliation before wrapper switch. Signature
and systemd adapters are still mocked in these tests; no native qualification
or deployment is claimed. No package has been signed.
