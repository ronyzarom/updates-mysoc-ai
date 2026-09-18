# Native node A bootstrap completion — 2026-09-18

Signed maintenance kit **1.16.1.28-r3** completed the retained SiemCore
**3.3.152.40** independent bootstrap on `bezeq-pod-test-a`.
Installed updater remains **1.16.1.28**, self channel **stable**, central **alpha**
with automatic updates enabled. Product isolation channel remains `node-a-20260918`.

The original signed .40 archive is unchanged. Docker29 returned CAP_-prefixed
capability names, which the original verifier rejected. Product commit
`29165eced177bf9fa286ebe403edad9f9ae3630f` canonicalizes that spelling while retaining
all original readiness checks. Updates installer commit `650b14d` installs the
signed, exact-host-bound adapter without applying the product itself.

## Verification

- Local: 11 product repair tests, 8 existing hook tests, 3 Updates installer tests passed.
- Native AMD64: 11 repair and 8 existing hook tests passed. Initial test staging
  omitted the AST comparison's reference source; adding that fixture resolved
  the test setup error without changing implementation or signed package.
- Existing fleet Ed25519 key verified outer manifest, archive, every delivered
  file and exact authorization. No new trust root.
- Installer verified original machine/install/updater/journal/release/container
  bindings; installed adapter last under existing root hook lock; restarted only
  the updater at 12:54:25 UTC. No manual product apply.
- Automatic updater retry downloaded and verified original .40, invoked fixed
  root apply and health, then reported success at **12:54:34 UTC**.
- Root journal is `complete`, `installed-unlinked`; repair receipt is
  `journal_committed`. Updater records .40 and success with no pending retry.
- PostgreSQL, Redis and management container IDs/images stayed identical.
  Policy/configuration hashes, application binary and identities stayed identical.
- Health: management/data/installation ready; processing/authority/POD/link false.
- Fresh cascade heartbeat at **12:55:39 UTC** reports .40 running, updater
  1.16.1.28, alpha/automatic, last update successful and empty error.
- Initial updater hash is an admission check only. A committed receipt permits
  later signed updater self-updates; it is not a permanent updater pin.

## Scope

This is narrow .40 bootstrap recovery, not general linked-POD upgrade qualification.
No data initialization, migration, restore, DB copy, container replacement, role
activation or SiemCore artifact replacement occurred. Normal and Observer product
installations were not modified by this repair. Benchmark hold remains in effect.

Host retains signed package under `/root/node-a-bootstrap-health-repair-1.16.1.28-r3`.
Original root hook is retained root-only. Original control journal and repair
receipt are under `/var/lib/siemcore-bootstrap-repair/04705744-f59d-4771-9188-b6a8c929f497`.
These are operation metadata, not database backups.
