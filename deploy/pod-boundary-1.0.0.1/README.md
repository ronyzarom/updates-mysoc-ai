# Retained-candidate pod boundary 1.0.0.1 — LOCAL CANDIDATE, NOT QUALIFIED FOR DEPLOYMENT

Updates owns this privileged boundary. It wraps the pinned existing root hook,
without editing SiemCore's hook or extending updater sudo permissions. A/B only;
witness, standalone and incomplete first bootstrap retain the existing path.

Before invoking an A/B candidate, retain both signature-verified candidate and
predecessor archives privately under /var/lib/siemcore-pod-boundary. Require the
candidate's signed executable updater/compensate. Journal every supervised unit
before invocation. Pass SIEMCORE_POD_RECOVERY_PROTOCOL=1 only from this boundary.

Apply and separate updater health calls use the retained candidate. Rollback
first revokes/stops any interrupted unit, then executes the retained candidate's
`updater/compensate rollback`, THEN the retained predecessor's `updater/apply rollback`.
The filesystem executor may already have switched current; never use current to
find the failed candidate. Validate current's signed predecessor identity before
rollback. Signature/digest verification is repeated on privately retained bytes.

Compensate must read its identity from its own verified bundle manifest and emit
exactly one stdout line:
MYSOC_COMPENSATION_RESULT_V1:{"status":"paused","product":"siemcore","version":"<candidate>"}
All other diagnostics go to stderr. It must idempotently retain/reacquire quorum
maintenance, abort resume, and prove drained processing. Missing transaction,
wrong identity, bad/missing receipt or command failure blocks previous-role apply.
Paused is not restored availability. No unchanged/success substitute is allowed.

All entrypoints execute in transient systemd control groups (KillMode=control-group,
RuntimeMaxSec, bounded stop), not only process groups. A root-owned pending-unit
journal and worker gate reject delayed launches after rollback revoked a phase.
Interrupted quiescence retains its unit ID and blocks all later rollback retries
until inactivity is confirmed. Apply/health and predecessor rollback have 600s
budgets; compensation 90s. Product subprocess deadlines must fit these limits.
Cgroup supervision covers processes, not asynchronous Docker-daemon side effects:
product compensation must reconcile those effects and prove stopped state.

## Qualification and provisioning

Local unit tests use fake artifact verification/systemd adapters: they cover
apply and health failure ordering, missing/bad compensation receipts, failed
compensation, retained bytes versus mutable cache, corrupted retained bytes,
interrupted unit quiescence/retry, delayed workers, and repeated rollback.
They do NOT establish native systemd timeout, SIGKILL/cgroup, signed full-artifact,
retained .30 rollback, Docker, database or availability qualification.

Before live installation, qualify on a disposable Linux/systemd GCP host with
real signed candidate/prior artifacts; inject outer wrapper termination, timeout,
late unit launch, post-apply health failure, and compensation failure. Confirm no
remaining phase processes and no prior rollback before proven compensation.
Protect root journal and archive permissions, verify installer repeat/refusal paths.

install.py requires exact machine ID/updater identity, known legacy hook digest,
known wrapper, private root package directory and updater already inactive. It
never holds/stops/restarts services, changes sudo permissions, edits DBs or reboots.
Use an explicitly approved non-SSH root provisioning route only AFTER qualification.
Do not use a startup-script reboot of active pod nodes. OS Config route still needs
agent, IAM/API and targeted-assignment verification; availability is not authorization.
Verify signed package externally; files.json provides internal integrity only.
There is no automatic publishing, scheduling, installation or ring promotion here.
