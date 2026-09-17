# Minimal SiemCore fixture interface for joint P1

This document specifies the Updates harness seam. It does not claim an executable
product lifecycle exists. SiemCore supplies the actual adapter executable and its
protected configuration; the existing `pod-drain-adapter` only handles drain.
Do not point maintenance/recovery modes at the drain-only adapter.

## Fixture output

Provide one absolute `plan.json` with:

- `isolated: true` and `adapter_kind: "product"`.
- `evidence_tier`: `native-with-synthetic-host` for real observer/mTLS/etcd with
  synthetic node lifecycle; `executable-real-host` only with actual isolated
  signed installation, runtime and independently observed health.
- `evidence_directory`: a NEW private directory, outside journal directories.
- `product_binaries`: map of absolute regular executable paths to exact SHA256.
  Include the real SiemCore executable invoked by each protected adapter command.
- `scenarios`: names, absolute argv arrays for `reset_command`, `ready_command`,
  `verify_command`, `stop_command`; optional `launch_command`; and `runs`.

Those argv arrays are fixture-owned commands, not assumed production subcommands.
Reset creates disposable certificates/keys, signed target AND predecessor archives,
protected adapter config, isolated observer store and node runtime, coordinator
case configs and the required original journal. Reset runs ONCE per scenario;
never reset the operation/ledger between fault and retry. Configure the observer
lifetime/authorization validity to cover the scenario without changing a signed
authorization's bytes or extending the original maintenance deadline.

`launch_command` is optional if reset starts owned services. Ready must return
zero only when the required pinned service is available. Verify must independently
inspect lifecycle/authority/installed bytes, distinguish target success from
predecessor restoration, and prove barriers remain on failed cases. Stop cleans
only fixture-owned services/resources. No production credentials/data, database
copy, backup, restore, SSH changes, or public dependency downloads.

## Per-run coordinator config

Each run supplies absolute `config`, `expected: success|failure`, optionally an
absolute `proxy_config` and `fault`. Coordinator case JSON fields:

- `isolated`, `test_id`, `evidence_tier`, `mode`, absolute `directory`.
- `adapter_command`: real absolute executable plus fixed arguments. Driver appends
  ONE trailing action. No shell interpolation.
- `timeout_seconds`: 1..60 (proxy adapter timeout <=55).
- `binding`: exact `pkg/podmaintenance.Binding`, including original operation ID,
  pod/node/updater identity, target/predecessor version/digests, target signature,
  absolute retained artifact path, and original deadline.
- `observer_public_key`, `release_public_key`: disposable public keys in hex.
- `authorization` and `artifact` where required by the selected mode.
- Optional `drain_protocol`: explicitly select `pod-maintenance-ack-drain-recovery-v1`
  for ACK-v2 recovery fixtures; omission retains the original drain protocol.
  See ACK-DRAIN-RECOVERY-CONTRACT.md for the required epoch proof and public fixture.

`run_suite.py` requires these case configs after reset and before each driver run.
It verifies product binary hashes before execution and stores them in receipts.
For a fault proxy, the driver's argv must reference the supplied `proxy_config`;
that config's `adapter_command` is the actual product executable.

## Exact adapter action and wire contracts

One bounded JSON object on stdin and one on stdout (<=64 KiB). stderr is diagnostic.
Nonzero exit/timeout/malformed JSON means failure, never permission. Unknown and
duplicate JSON fields are refused. The privileged adapter supervises descendants;
coordinator termination is not proof that product mutation stopped.

| Driver mode | Trailing actions | Request / response Go types |
| --- | --- | --- |
| readiness | readiness | ReadinessRequest / ReadinessResponse |
| maintenance-v1 | capabilities, begin-or-resume, status, apply, recover, health, complete, acceptance | Request / Response |
| observer-maintenance | capabilities, prepare, status, apply, reconcile, health, complete, acceptance | ObserverRequest / ObserverResponse |
| recovery-v2 | capabilities, status, authorize-recovery, recover, health, complete, acceptance | RecoveryRequest / RecoveryResponse |
| next-operation | authorize-next-operation | NextOperationRequest / NextOperationResponse |
| drain-discovery | status | DrainRequest / DrainResponse |
| drain-recovery | status, authorize-drain-recovery, resume-drain | DrainRequest / DrainResponse |

All types are in `pkg/podmaintenance`. V1 protocol is inside `binding`; v2 and drain
also carry their distinct top-level protocol. Separately signed v2 authorization
is mandatory for product recovery; drain permission cannot authorize apply.
Readiness must be fresh, exact-identity and independently true for observer,
credentials AND lifecycle. Unsupported/partial lifecycle reports not ready.

Drain production argv currently supplied by SiemCore:
`/usr/local/libexec/siemcore pod-drain-adapter --config <protected-config>`.
The full maintenance argv remains product-owned and must be supplied explicitly.

## First joint cases

1. Drain gen0 status discovers exact existing generation; no mutation; repeat after
   driver death preserves operation/deadline/capture. Separate valid grant reaches
   paused; lost reply retries without new operation. Revoked/expired grants retain
   barrier. Evidence cannot be missing, >5s old, or later than response observation.
2. Full readiness negative until actual signed lifecycle is available.
3. V1 clean paused begin → signed apply → measured health → complete → acceptance;
   kill/lost replies around begin, apply/recover, health, complete and acceptance.
4. Separately authorized v2 resume-target and restore-predecessor, including
   expired/revoked grants, wrong signatures/digests, unsafe runtime state and retry.
5. Next-operation archival retains exact operation/drain/recovery evidence and
   ledgers, including interrupted finalization.
6. Normal standalone remains the existing signed/health/rollback path; POD identity
   never grants active authority. Observer-specific updates remain separate.

## Evidence and acceptance

The harness records actual binary/config hashes, bounded adapter request/reply
traces, exit status, journal snapshots (including drain), archive hashes and product
verification logs. `suite_passed` is only the submitted scenario result.
`deployment_qualified` ALWAYS remains false: qualification also requires review of
scenario coverage and genuine node lifecycle evidence. No flag, ring, host hold,
release, deployment or publication is changed by this harness.
