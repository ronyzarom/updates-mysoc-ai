# Update Entrypoint Contract (v1.4)

How the updater invokes application-owned install logic. This is the interface
app teams (mysoc, siemcore, swf) code against; the updater guarantees it from
platform version 1.9.1.

v1.1 amends v1 with clarifications raised during the three-team countersign
(no semantic change for any conformant implementation): per-host
`command_timeout`, secrets vs. version-pin fields in host config, a third
compliant migrations mode, and the fresh-install rollback refinement.
v1.2 fixes the shim ordering rule: delegation precedes the shim's own
fallback preconditions, and an entrypoint owns the whole apply.
v1.3 adds the `first_install` phase: a fresh host (version `0.0.0`) is
self-provisioned by the artifact through the same cascade path, gated by an
own-phase read-only preflight, with its own timeout and a no-rollback retry
model.
v1.4 adds requirement 6: an update never backs up or restores data and never
takes the product down beyond the process restart window. The changelog is at
the bottom.

> **Status:** v1.4 is **proposed** (drafted 2026-10-31) and awaiting countersign
> from mysoc, siemcore, and swf. Requirements 1–5 are the countersigned v1.3
> contract and are in force. Requirement 6 states the updater's existing
> no-backup/no-outage expectation explicitly so it can be countersigned; the
> updater measures the restart window (`unavailable_seconds`) and the server
> gates ring promotion on a budget breach as of platform 1.18.0.

## Who runs what

```
updater (unprivileged)
  └─ sudo host-shim <phase>          root-owned script, pinned in sudoers;
       │                             enforces host policy (e.g. migration guard)
       └─ <release>/updater/apply <phase>
                                     shipped INSIDE the artifact, owned by
                                     the app team; falls back to the shim's
                                     built-in logic when absent
```

- The **application binary is never the orchestrator** — it is the payload
  being replaced. Install logic lives in a script (any language) shipped in
  the artifact.
- The **host shim is the privilege boundary**: it is root-owned, installed by
  the host operator, referenced by a single `NOPASSWD` sudoers entry, and free
  to veto an install (non-zero exit) before delegating.
- **Delegation ordering (v1.2):** the shim's veto covers *universal* host
  policy only (release-tree containment, ownership, integrity of the staged
  bundle). Preconditions of the shim's own fallback apply — expected file
  layout, the unapplied-migration guard — must be checked **after** the
  delegation point, so they never gate an entrypoint-bearing release.
  Otherwise a release that ships migrations together with the installer
  meant to run them is refused before that installer executes.
- **An entrypoint owns the whole apply.** When `updater/apply` exists, it is
  responsible for the full lifecycle — DB migrate (applied and recorded),
  code sync, service restart, and health verification — for both phases.
  There is no split mode in which the shim runs part of the apply around an
  entrypoint; two owners of restart/health is how rollbacks get ambiguous.

## Entrypoint location

An artifact opts in by shipping an executable at:

```
updater/apply        required for opt-in (invoked for apply, first_install,
                     and rollback)
updater/health       optional (invoked for health when the operator's
                     health_command delegates to it)
```

Artifacts without `updater/apply` install via the host shim's built-in logic —
nothing breaks; adoption is per-release. The one exception is `first_install`:
the shim refuses that phase for an entrypoint-less release (the fallback cannot
provision a bare host), so a fresh-host install requires `updater/apply`.

## Fresh-host install (`first_install`)

A bare VM enrols at `current_version: 0.0.0` (see `deploy/bootstrap/mysoc-enroll.sh`
and the kit's `install.sh --clean --executor filesystem`). For such a host the
updater invokes `updater/apply first_install` instead of `apply`, and keeps
sending `first_install` on every retry while the installed version is still
`0.0.0` — so a partially provisioned host resumes rather than being
re-detected. The artifact's `first_install` owns the entire host bring-up:
install/start dependencies, create the service user, database and app role,
seed `.env` (reading `/etc/mysoc/install-seed.env`, generating its own secrets),
place TLS, render units and nginx, bootstrap the baseline DB, sync code, and
health-gate.

Two rules make this safe:

- **Read-only preflight head.** `first_install` MUST begin with a read-only
  check and mutate nothing until it passes: the seed is complete for the chosen
  TLS mode, the OS/arch is supported, the host can reach apt and the updates
  server, the domain resolves (and ports are free for certbot), TLS material is
  valid, PostgreSQL is absent-or-clean, and there is enough disk. On failure it
  exits non-zero with a one-line diagnostic (and the fix) on stderr. Classified
  exit codes let the updater and operators triage (see the table below).
- **No rollback; back off and resume.** A `first_install` failure is NOT rolled
  back (there is nothing to roll back to, and a per-cycle stage/fail/rollback
  loop is exactly the churn to avoid). The updater clears the staged release,
  reports the error, and backs off (5m → 15m → 60m, capped). Because
  `first_install` is idempotent/resumable, a later attempt continues once the
  admin fixes the host.

## Invocation

Every lifecycle command receives the phase **twice**:

1. As the **trailing positional argument**: `updater/apply apply`,
   `updater/apply rollback`, `updater/health health`.
2. As the environment variable `UPDATER_PHASE`.

Existing wrappers that predate this contract ignore the extra argument and
keep working unchanged.

### Environment

| Variable | Meaning |
| -------- | ------- |
| `UPDATER_PHASE` | `apply`, `first_install`, `rollback`, or `health` |
| `PRODUCT` | product name, e.g. `mysoc` |
| `VERSION` | version the updater is applying (on rollback: the version that FAILED) |
| `FROM_VERSION` | version installed before this update |
| `CURRENT_DIR` | the `current` symlink path; resolves to the ACTIVE release directory (on rollback: the restored previous release) |
| `INSTALL_ROOT` | base staging directory, e.g. `/opt/mysoc-cascade` |

Rollback nuance: during `rollback`, `CURRENT_DIR` points at the restored
previous release, so re-reading that directory's own `VERSION` file — not the
`VERSION` env var — is the correct way to know what to bring live. On a
fresh-install rollback the symlink is removed; the entrypoint should detect
the missing directory and exit non-zero. Refinement: the exit code must
report the actual host state — if an application predating the updater's
tree is verified alive by a health probe, exiting zero is correct; a blanket
non-zero would page an operator for a host that needs nothing, and a blanket
zero would mask a host left down by a failed first apply.

Sudoers note: sudo strips the environment by default. The host's sudoers entry
must keep these variables:

```
Defaults!/usr/local/sbin/<shim> env_keep += "UPDATER_PHASE CURRENT_DIR VERSION FROM_VERSION PRODUCT INSTALL_ROOT"
<updater-user> ALL=(root) NOPASSWD: SETENV: /usr/local/sbin/<shim>
```

## Exit codes and flow

| Phase | Exit 0 | Non-zero exit |
| ----- | ------ | ------------- |
| `apply` | proceed to health validation | automatic rollback: symlink restored, entrypoint re-invoked with `rollback` |
| `first_install` | proceed to health validation | NO rollback: staged release cleared, error reported, updater backs off and retries (resumable) |
| `health` | update reported successful | automatic rollback |
| `rollback` | previous version restored | logged; host may need manual attention |

### `first_install` classified exit codes

Non-zero still means "refuse / not ready"; these let the updater and operators
triage the cause. (Values below are the mysoc convention; a `1` is any other
failure.)

| Code | Cause |
| ---- | ----- |
| `10` | seed incomplete for the chosen TLS mode |
| `11` | unsupported OS / arch / init |
| `12` | network or apt mirror / updates server unreachable |
| `13` | DNS / domain / port (80,443) problem |
| `14` | TLS material invalid (missing, mismatched, expired) |
| `15` | PostgreSQL in an unexpected state (not clean, no peer auth) |
| `16` | insufficient disk |
| `1`  | any other failure |

## Requirements on the entrypoint

1. **Idempotent** — re-running `apply` for the same release must be safe.
   `first_install` must be idempotent AND **resumable**: every sub-step is
   guarded (user/DB/role exist → ensure, not recreate; `.env` never
   overwritten; valid cert → skip; baseline is a no-op when a ledger exists),
   so a retry after a partial failure continues rather than re-breaking.
2. **Bounded** — `apply`/`rollback`/`health` must finish within the operator's
   `command_timeout`; `first_install` has its own, larger budget,
   `first_install_timeout` (fresh-host provisioning: apt deps, baseline DB,
   certbot, health gate — default 30m), including any health polling you do
   internally. These are per-host operator settings, not fleet constants
   (current fleet: mysoc hosts `command_timeout` 300s / `first_install_timeout`
   30m; siemcore hosts 900s). Do not normalize one product's budget onto
   another.
3. **Never touch host-side secrets** — credentials in `.env` and equivalent
   host configuration are outside the artifact's authority. Non-secret
   operational fields that the app's own deploy contract requires — version
   pins (e.g. `SIEMCORE_VERSION`) and provenance fields — may be updated by
   the install logic that owns them. Write deploy provenance to your own
   files (e.g. `DEPLOY_INFO.json`).
4. **Database migrations are explicit** — one of three compliant modes:
   the entrypoint applies and records them; or the entrypoint exits non-zero
   when unapplied migrations are detected; or the application
   self-migrates-and-records at startup (migration ledger plus advisory
   lock), provided the health gate is version-matching so a failed migration
   means no healthy app and therefore automatic rollback. Silently running
   code against a mismatched schema is the one unrecoverable failure mode.
5. **Output is captured** — stdout/stderr go to the updater log and to the
   error report on failure; print decisions, not spam.
6. **No data backup/restore, no outage beyond the restart window** (v1.4,
   proposed). An update is a code/schema/config change, not a data-custody
   event:
   - It **never** dumps, backs up, snapshots, or restores a database or data
     volume as part of `apply`/`rollback`/`first_install`. Backup is an
     operator-scheduled concern and must never be coupled to an update — an
     update that depends on taking a backup first cannot be bounded and will
     eventually block the fleet. Rollback is a code/symlink revert plus
     expand/contract-safe schema, not a data restore.
   - It **never** runs table-rewriting or long-locking operations inside
     `apply` (`VACUUM FULL`, `REINDEX`, `CLUSTER`, `ALTER ... TYPE`,
     non-concurrent index builds on large tables). Schema changes are
     expand/contract and complete within the bounded budget (requirement 2).
   - The **only unavailability** an update may cause is the process restart
     window. `systemctl stop` (or equivalent) of the product outside that
     window is forbidden. The updater measures the window and records
     `unavailable_seconds`/`availability_violation` in the report; exceeding the
     host's `max_unavailable_seconds` (default 120s) never aborts an in-flight
     apply (that would only lengthen the outage) but flags the report, and the
     updates server refuses to widen such a release beyond the alpha ring
     without an audited override. Alpha is where downtime is caught.

## Reference implementations

- mysoc host shim: `deploy/mysoc-apply-update.sh` (delegates to
  `updater/apply` first; fallback apply with migration guard, code sync,
  restart, health gate, provenance file for entrypoint-less releases).
- siemcore host shim: `/usr/local/sbin/siemcore-apply-update` (tree
  containment, ownership, manifest cross-check, `checksums.txt`
  verification) delegating to the app repo's `update.sh` (v3 compose
  contract). Canonical executor config lives in the siemcore repo under
  `deploy/cascade/` and `docs/operations/cascade-updater.md`.
- swf (Windows leaf): the 2.x agent's `update` verb — install logic ships
  inside the artifact; rollback executes within the same apply process
  (accepted platform divergence: the outcome triple — previous release
  live, readiness-verified against a disk-read version, durably reported —
  is preserved).

## Changelog

- **v1.4 (proposed, 2026-10-31)** — added requirement 6: an update never backs
  up or restores data and never takes the product down beyond the process
  restart window (no data dump/snapshot/restore coupled to an update; no
  table-rewriting/long-locking ops inside `apply`; expand/contract schema only;
  `systemctl stop` outside the restart window forbidden). The updater measures
  the window (`unavailable_seconds`) and flags `availability_violation` when it
  exceeds the host's `max_unavailable_seconds`; the updates server refuses to
  widen a flagged release past alpha without an audited override. States the
  updater's long-standing no-backup/no-outage expectation explicitly so it can
  be countersigned. Awaiting countersign from mysoc, siemcore, swf. Raised by
  the updater survivability work (platform 1.18.0).
- **v1.3** — added the `first_install` phase for fresh hosts (version
  `0.0.0`): the updater sends `first_install` instead of `apply` (and on every
  retry while still `0.0.0`); the artifact self-provisions the whole host behind
  a mandatory read-only preflight head with classified exit codes; it runs under
  its own `first_install_timeout` (default 30m); failures are NOT rolled back —
  the updater clears the staged release, reports the error, and backs off
  (5m/15m/60m) so a resumable host continues. The shim refuses `first_install`
  for an entrypoint-less release. Raised by the one-command fresh-install work
  (mysoc-enroll.sh + kit `--executor filesystem`).
- **v1.2** — delegation ordering fixed: the shim checks universal host
  policy, then delegates to `updater/apply` when present; the shim's
  fallback preconditions (layout checks, unapplied-migration guard) apply
  only to entrypoint-less releases. Stated explicitly that an entrypoint
  owns the complete apply lifecycle (migrate → sync → restart → health) —
  no split ownership. Raised by mysoc while building their first
  entrypoint-bearing release; without this fix a migration-bearing release
  shipping its own installer would be refused before the installer ran.
- **v1.1** — post-countersign clarifications, no semantic change for
  conformant implementations: `command_timeout` documented as per-host
  (mysoc 300s, siemcore 900s); requirement 3 distinguishes secrets
  (forbidden) from app-owned version-pin/provenance fields (permitted);
  requirement 4 names startup self-migrate-and-record behind a
  version-matching health gate as a third compliant mode; fresh-install
  rollback exit code reports actual host state; reference implementations
  expanded with the siemcore shim and the swf agent.
- **v1** — initial contract, countersigned by swf, mysoc, and siemcore.
