# Actual Observer coordinator fixture handoff

Coordinator commit: `695fc23`; Observer coordinator and simulator source tests passed,
including focused race tests. This handoff adds suite-mode validation and collection
of observer-operation.json. These are local test drivers, not updater releases.

## Built drivers

Host-native: `/tmp/updates-observer-qualification`
SHA256 `0a2daab6e14a4e922f18d11a595bd0a0a2fd2d071c74fae656fb2bec6b579e3b`

Linux amd64: `/tmp/updates-observer-qualification-linux-amd64`
SHA256 `c15e66fdabdb31a8e861e68f346f82fa6bd9a723d15a3d95fd6a05ca0bd43b2a`

These files were built locally, not copied to any host. Linux product qualification
must run the Linux driver inside the disposable Linux fixture. Rebuilding changes
provenance; record its actual SHA256 in the suite output.

## Exact invocation

Assuming the fixture exposes the Linux driver and generated case at these paths:

```sh
/tmp/updates-observer-qualification-linux-amd64 --config /root/observer-qualification/case.json
```

For the existing suite runner, in an environment containing this source checkout:

```sh
python3 /tmp/updates-independent-review/scripts/qualification/pod-maintenance/run_suite.py \
  --driver /tmp/updates-observer-qualification-linux-amd64 \
  --plan /root/observer-qualification/plan.json
```

Use the host-native driver only for components executable on the host OS. Do not
label those results as Linux service/runtime qualification.

## Case configuration

`observer-case.template.json` lists exact fields. The product fixture reset fills
the four cryptographic placeholders and product-owned adapter subcommand. It must
supply real disposable signed archive bytes at artifact_path. The target SHA and
signature are independently verified by the driver before any adapter call.
The previous digest must identify the actual retained predecessor used by product
verification, not an invented hash.

The driver generates and persists an operation ID and original 30-minute deadline
when starting a new journal. A supplied original deadline may instead be specified
as RFC3339 UTC; it must not change between retries. Do not reset the journal between
fault and retry. On restart the retained binding/generation/snapshot controls work.

Product must supply the actual adapter subcommand and protected config schema;
Updates has not invented a production command name. The driver appends exactly one
action, for example `prepare` or `reconcile`. The response schema is ObserverResponse
in pkg/podmaintenance/observer.go, with no additional unknown fields.

The local fixture adapter must represent the actual supported executor and may
declare its fixture lifecycle ready for testing. This does not enable any live
readiness, fleet advertisement, release or deployment. If the executor is absent,
capabilities must remain not ready and the expected result is failure.

## Suite plan requirements

Use the existing PRODUCT-FIXTURE-CONTRACT.md schema: isolated=true,
adapter_kind=product, NEW evidence_directory, product_binaries mapping exact absolute
executables to SHA256, and scenario reset/ready/verify/stop argv. Reset runs once per
scenario. Evidence tier remains native-with-synthetic-host until actual service
restart and runtime verification replace host spies. Only then use executable-real-host.

For lost replies, invoke fault_proxy.py through adapter_command with an absolute
proxy JSON config containing the actual pinned product argv. The suite step sets:

```json
{"fault":{"action":"apply","occurrence":1,"point":"after","effect":"lost-response"}}
```

That step expects failure; its next run uses the SAME case, journal, proxy counters
and product state and expects success via reconcile. Also test lost complete,
service unavailable during apply/reconcile, and process death using effect=crash.
Do not use fault_checkpoint for Observer mode; that field currently belongs to the
separate next-operation archival driver.

## Product verification requirements

- Durable execution receipt exists before changing the protected content-addressed
  pod/bin/siemcore selection or restarting the allocation-observer service.
- Only the intended signed service binary/selection changes. No etcd binary/data,
  database schema, bootstrap initialization or authority-history rewrite.
- Prepare leaves existing owner valid; explicit apply starts protection. Other pod
  updates and takeover are blocked. Active permission is not extended indefinitely.
- Existing assignment and authority generations survive restart. Product baseline
  checks and receipt prove preservation; management/quorum/authentication and exact
  target bytes are independently observed.
- Lost apply reconciles the same operation without blind reapply; lost complete
  remains readable after later legitimate history changes. Failed health, changed
  baseline, wrong signature and expired incomplete operation retain protection.
- Cleanup removes only fixture-owned resources. No production credentials or data.

Suite output must include observer journal snapshots/hashes, adapter request/reply
traces, pinned executable hashes, product execution receipt evidence and independent
service verification. deployment_qualified remains false; publication is separate.
