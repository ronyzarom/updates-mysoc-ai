# Joint pod-maintenance qualification harness

The driver calls the **real** Go maintenance/recovery/transition coordinators.
It is not an installer and has no publication or deployment command.

Build: `go build -o /private/test-root/coordinator ./cmd/pod-maintenance-qualification`
Run: `/private/test-root/coordinator --config /private/test-root/case.json`

Config fields: isolated=true, test_id, evidence_tier, mode, directory,
adapter_command (absolute executable + fixed arguments), timeout_seconds (1..60),
binding, authorization, artifact, observer_public_key, release_public_key.
Modes: maintenance-v1, recovery-v2, next-operation, readiness, drain-discovery,
drain-recovery. Relevant wire objects
are the exact pkg/podmaintenance structs/canonical fixtures. The driver checks
retained artifact signature/checksum before v1, as the normal delivery path does.
V2/next-operation coordinators independently verify their retained artifacts.
Keys must be disposable fixture keys; never use live grants or credentials.

Adapter invocation is configured argv plus ONE trailing action. A single strict
JSON request arrives on stdin; exactly one JSON response (<=64 KiB) goes to stdout.
Diagnostics go to stderr. Nonzero exit, timeout, duplicate/unknown fields or extra
JSON fail closed. No shell interpolation. The adapter owns authenticated observer
calls, signed product staging/recovery, actual installed health and native child
supervision. Coordinator cancellation does not prove privileged descendants exited.

## Fault proxy

Set adapter_command to `[absolute_python, absolute_fault_proxy.py, "--config",
absolute_proxy_config]`. Proxy config contains isolated=true, evidence_directory,
adapter_command for the REAL product adapter, adapter_timeout_seconds (<=55), and
optional fault {action, occurrence:1, point:before|after,
effect:crash|lost-response|timeout}. Proxy appends the action unchanged.

Each request/response/stdout/stderr/exit and SHA receipt is stored privately.
Fault counts are fsynced before injection. `crash` SIGKILLs only the verified driver
parent PID supplied by the test driver. `lost-response` executes the adapter then
discards its reply. `timeout` stalls at the chosen boundary. It does not simulate
killing the product mid-write; product/native supervision tests supply those cases.

Next-operation driver `fault_checkpoint` permits actual process SIGKILL after
prepared evidence/receipt, live recovery archival, or writing next intent. Injection
is once-only using a durable marker. Production callers leave checkpoint callbacks
nil. Retry verifies retained archive hashes and finishes that same transition.

## Configurable orchestration

`python3 run_suite.py --plan /private/test-root/plan.json --driver /private/test-root/coordinator`

Plan: isolated=true, evidence_tier, adapter_kind=product|reference,
evidence_directory (new), scenarios array. Each scenario has name and optional
reset_command, launch_command, ready_command, verify_command, stop_command (argv
arrays), plus runs [{config:absolute_path, expected:success|failure,
proxy_config:absolute_path, fault:{...}}]. Product team owns launch/reset/stop for
its disposable observer, etcd, nodes and credentials. Reset occurs once per case,
NEVER between a crash and retry. Cleanup stops owned process groups and supplied
services. No database is copied or backed up. Product verification command checks
real installed state and records its evidence separately.

Every result explicitly identifies one evidence tier:
- component: protocol fixtures/synthetic adapter and health.
- native-with-synthetic-host: real observer/mTLS/etcd but synthetic host evidence.
- executable-real-host: real isolated host/runtime/product lifecycle and evidence.

Tier labels are assertions requiring evidence review, not automatic qualification.
The suite never sets deployment_qualified=true. Reference adapter refuses readiness
and cannot be labeled native/product. It exists only to exercise coordinator fault
semantics; it does not install SiemCore or prove application health.

## Executable component suite

`go test ./cmd/pod-maintenance-qualification -parallel 4 -timeout 180s`

Covers real SIGKILL before/after and lost replies at v1/v2 command boundaries,
both target/predecessor outcomes, interrupted apply->recover, unsupported/not-ready
refusal, lost next-operation authorization and three archival crash checkpoints.
Additional package tests cover expired/revoked/replayed grants, exact identities,
retained tamper and v1/v2 separation. These results remain component evidence.

## Real product fixture submission

See [PRODUCT-FIXTURE-CONTRACT.md](PRODUCT-FIXTURE-CONTRACT.md) for the exact fixture
interface and action/type table. Product plans require pinned `product_binaries`
and explicit reset/ready/verify/stop commands. The suite validates binaries before
each step and includes drain journals/archives in evidence. Fixture validation is
not product qualification; do not relabel a synthetic lifecycle as real-host work.
