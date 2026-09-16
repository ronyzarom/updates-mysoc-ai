# Boundary 1.0.0.2 — local review candidate

Signed boundary deployed to B only through GCP OS Config on 2026-09-16.
Native replay, signature/tamper and filesystem/systemd qualification passed.
B's exact failed transaction is now preflight-refused with original evidence
preserved. A/witness were not changed. No product release was published.

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

The log pins were derived from captured diagnostics and subsequently matched
against full files on B by read-only SHA-256 readback. The operation
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
   replay qualification has passed. Envelope qualification is tracked separately.
   Do not reuse 1.0.0.1 installer (it rejects existing journals).
4. Independently read back terminal journal, preserved evidence and package
   hashes. Keep B product hold until product-owned recovery and next candidate
   approval. Never clear a journal or fabricate a paused receipt.

Local tests:
`python3 -m unittest discover -s deploy/pod-boundary-1.0.0.2 -v`

Current local result: 31 tests pass, including exact evidence checks, extra
execution/environment injection refusal, populated cgroup refusal, installation
drift, and interruption after reconciliation before wrapper switch. Signature
and systemd adapters are mocked in the unit suite; the separate native result
below verifies real signatures and systemd behavior. No pod deployment is claimed.

## Native qualification and signing (2026-09-16)

OS Config boundary-2-native-20260916 passed on disposable VM
6826523492612546857. Real .33/.30 signatures and checksums were verified.
Historical incident logs were replayed under fixture identities/paths; native
systemd active-unit refusal, empty stopped cgroup, exact terminal evidence and
versioned installer repeat passed. No signed product apply, DB or pod activation
was run. A first fixture packaging attempt failed on tar-preserved non-root
ownership; extraction was corrected to root ownership without weakening checks.

Signed package SHA-256:
`ba508630db07192ceda951957755c0fed076798823c61547af06943b0be2aa2e`
Existing key/domain, `mysoc-pod-boundary-v1\n1.0.0.2\n<sha256>`.
Signature independently verified locally. Signed package and receipts are in
`/tmp/pod-boundary-2-package/`; durable evidence is in the primary repository's
`docs/verification/pod-boundary-2-20260916/` directory.

`provision_osconfig.py` is the management-channel envelope (not part of the
signed package payload). It verifies exact GCP/machine/product identity and
configuration hashes, validates package signature and safe members, stops only
the updater after cycle-lock acquisition, invokes the versioned installer under
all three locks, then restores the updater's prior active state. No pod service,
VM, DB, ownership, routing or ring changes are performed. The product hold must
be independently verified and preserved by the deployment coordinator.

## B-only deployment result

OS Config `boundary-2-b-recovery-20260916` reported COMPLIANT at12:16:12Z.
Independent host readback verified terminal preflight-refused, no pending unit,
preserved original journal/evidence, installed manifest hashes, unchanged
configuration/policy/sudo hashes, and updater active after restart12:15:44Z.
Wrapper SHA256: `64eaa766f846aea9cc9e7864c8d947b016f247aaad328b1fe3dab63725e2dfa3`.
Current application remains3.3.152.30, product transaction absent, B standby
with customer processing/DB/Redis stopped. No availability/rollback success is
claimed. B/witness product holds and A's stopped state were preserved.

The first attempt refused a busy updater cycle lock before mutation. The
reviewed envelope now waits at most45s by retrying the same nonblocking exclusive
lock; timeout still refuses. It never stops the updater before acquiring that
lock. Delivery regular-file/symlink and retry tests bring the unit suite to36.
Only the updater service restarted. No product services, VM, DB, routing,
maintenance or release-target changes were made by delivery.
