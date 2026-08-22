# Update Entrypoint Contract (v1.1)

How the updater invokes application-owned install logic. This is the interface
app teams (mysoc, siemcore, swf) code against; the updater guarantees it from
platform version 1.9.1.

v1.1 amends v1 with clarifications raised during the three-team countersign
(no semantic change for any conformant implementation): per-host
`command_timeout`, secrets vs. version-pin fields in host config, a third
compliant migrations mode, and the fresh-install rollback refinement. The
changelog is at the bottom.

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

## Entrypoint location

An artifact opts in by shipping an executable at:

```
updater/apply        required for opt-in (invoked for apply and rollback)
updater/health       optional (invoked for health when the operator's
                     health_command delegates to it)
```

Artifacts without `updater/apply` install via the host shim's built-in logic —
nothing breaks; adoption is per-release.

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
| `UPDATER_PHASE` | `apply`, `rollback`, or `health` |
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
| `health` | update reported successful | automatic rollback |
| `rollback` | previous version restored | logged; host may need manual attention |

## Requirements on the entrypoint

1. **Idempotent** — re-running `apply` for the same release must be safe.
2. **Bounded** — must finish within the operator's `command_timeout`,
   including any health polling you do internally. This is a per-host
   operator setting, not a fleet constant (current fleet: mysoc hosts 300s,
   siemcore hosts 900s to cover image load, recompose, and health gates).
   Do not normalize one product's budget onto another.
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

## Reference implementations

- mysoc host shim: `deploy/mysoc-apply-update.sh` (migration guard,
  code sync, restart, health gate, provenance file).
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

- **v1.1** — post-countersign clarifications, no semantic change for
  conformant implementations: `command_timeout` documented as per-host
  (mysoc 300s, siemcore 900s); requirement 3 distinguishes secrets
  (forbidden) from app-owned version-pin/provenance fields (permitted);
  requirement 4 names startup self-migrate-and-record behind a
  version-matching health gate as a third compliant mode; fresh-install
  rollback exit code reports actual host state; reference implementations
  expanded with the siemcore shim and the swf agent.
- **v1** — initial contract, countersigned by swf, mysoc, and siemcore.
