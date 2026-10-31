# Branch / stranded-commit audit

A fix that lives only on a side branch is a regression waiting for the next
release. Before cutting a release branch the development cycle now requires
`git log --oneline main..<fleet branch>` to be empty (see
[`.cursor/rules/development-cycle.mdc`](../.cursor/rules/development-cycle.mdc)
rule 2). This ledger records the dispositions found when that gate was first
applied, during the 1.18.0 survivability work.

## 2026-10-31 — audit at `main` = `e860959` (1.17.1.1)

### `main..version/1.16.1` (the line the fleet ran as 1.16.1.33) — 12 commits

| Commit | Summary | Disposition |
|---|---|---|
| `1d658e1` | Preserve signed release receipts through filesystem updates | **Resolved in main.** Reimplemented as `e860959` (1.17.1.1): `signature` restored in the staged `.updater-release.json`, with receipt + rollback tests. This is the one that was a live regression (1.17.0.2 dropped it); it is the reason the gate exists. |
| `f5dc7dd` | Make updater config readable under strict umask | **Ported to `version/1.18.0`.** Survivability-relevant: under umask 077, `/etc/<unit>` is created 0700 root:root and the unprivileged service cannot traverse it to read `config.yaml`, so the updater never starts and can never self-update. `install.sh` now sets `root:<unit>` + `0750` explicitly. |
| `f2f291b`, `ff0f672`, `56a6fe0`, `74a90e2`, `43368d4`, `4ff1dbc` | Opt-in signed greenfield bootstrap in the SiemCore kit, bootstrap channel isolation, pod-identity/enrollment split, transactional-recovery install, lifecycle health-invocation fix (`kits/siemcore/greenfield-bootstrap.py`, ~175 lines + tests) | **Deferred — siemcore provisioning feature, not updater survivability.** `kits/siemcore/greenfield-bootstrap.py` does not exist on `main`. These only affect **fresh-host provisioning** via the kit; an already-enrolled host does not lose anything by self-updating its binary. Porting them into the mainline kit changes install behaviour and must be confirmed against siemcore's current greenfield/recovery flow (their `release-process.md` "one artifact" model and the root recovery runtime 1.0.0.9) before it lands. Tracked for a dedicated kit decision with siemcore; not folded into the survivability binary release. |
| `1650b17`, `47e6023`, `ceb48b1`, `2814ed1` | "Prepare updater kit 1.16.1.5–.8" (VERSION bumps only) | **Retired.** Build-number bookkeeping for the 1.16.1 line; nothing to port. |

### `main..version/1.15.0` — 1 commit

| Commit | Summary | Disposition |
|---|---|---|
| `894e4be` | SiemCore kit: publicly trusted relay TLS (`:18443`) as the default, plus `provision-relay-tls.sh` (268 lines) | **Deferred to Phase 2 kit work.** The fleet's siemcore relays run operator-provided (SSL.com wildcard) certs on `:18443`, but `main`'s kit defaults to self-signed with children pinning the CA, and the reconciler script is absent. This is relay-backbone material (invariant 8). The gap is documented in [`RELAY-DEPLOYMENT.md`](RELAY-DEPLOYMENT.md) §Listener TLS; porting the default + script belongs with the Phase 2 kit roll (relay hosts first) and needs the cert material and a soak, so it is not force-fit into the binary release. |

### Branches archived

`version/1.16.1` and `version/1.15.0` are preserved as tags `archive/version-1.16.1`
and `archive/version-1.15.0` so the deferred work is never lost, and can be
deleted as live branches once the deferred items above are dispositioned.
