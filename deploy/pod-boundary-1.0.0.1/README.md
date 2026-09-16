# Retained-candidate pod boundary 1.0.0.1 — A/B prerequisite installed

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

## Native qualification result — 2026-09-16

Passed via OS Config on disposable Ubuntu24.04 VM updater-boundary-qual-20260916,
instance6826523492612546857, me-west1-b. No SSH writes, production changes or
currentpod restarts. Tests: installer wrongidentity/activeupdater refusal and
first/repeated provisioning; real systemd command; runtime timeout kills child
in a new session; killed outer supervisor recovered through durable unit stop;
delayed revoked worker refusal. Ten adapter boundary tests also passed onLinux.
Actual OSConfig compliance report and serial result retained in the main workspace
at docs/verification/pod-boundary-native-20260916.

This upgrades native supervision/installer qualification only. It does not
qualify live product compensation, signed .32→.30 artifact rollback, database
recovery or availability. The exact .32 archive has a detached existing-key
qualification signature; no release row or ring offer was created.

OSConfig minimal prerequisites verified: enabled API and Google serviceagent,
installed OSConfig agent, attached VM serviceaccount (no projectroles required
for that identity), instance enable-osconfig=TRUE, and GoogleAPI connectivity.
Currentpod already has serviceaccounts; projectmetadata is PER-VM and individual
enablement was absent. Do not change currentpod metadata or deploy this boundary
until remaining signed-artifact gates pass.

## Exact negative test and scoped installation — 2026-09-16

The unchanged signed .32 and .30 archives passed the actual pinned legacy
Ed25519/checksum verifier on the disposable VM. The actual .32 compensation
entrypoint rejected the missing transaction with the expected error. Native
systemd execution auditing proved no .30 rollback entrypoint ran. Synthetic
product-approved inputs supplied no working credentials. This test does not
establish successful compensation, full retained rollback or DB recovery.

Under the existing one-time non-SSH prerequisite approval, installed on exact
A/B IDs in PROVISIONING-PLAN.md through per-instance OS Config. Package SHA256:
79b0abc14ce642cb1db8092fbadaba4d1af2cd259ab34eff7caca1239d247485.
The package uses the existing Ed25519 key, domain mysoc-pod-boundary-v1.
The updater's own state.json.cycle-lock was acquired nonblocking before stopping
the updater; a running cycle would have refused installation. Root policy,
sudo and updater config hashes were preserved. Updaters resumed and reported
heartbeats; installed hashes were independently read back. Application remained
.30, public health healthy, A owner generation3, B standby, PG streaming and Redis
replication up. Witness was not modified. Product .32 was not published/offered.

Evidence and signed provisioning archive: main workspace
docs/verification/pod-boundary-native-20260916. The earlier native-only caveats
remain relevant; this prerequisite installation is not product rollout acceptance.
