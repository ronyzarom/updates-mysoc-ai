# POD bootstrap registry fixture — integration review only

`registry-v1.fixture.json` matches the current product `internal/podbootstrap`
Registry field names/order. It contains PUBLIC TEST trust and a signature over
non-executable fixture bytes. Never publish/use it for a host installation.
Certificate inputs are synthetic bytes matching existing product unit fixtures;
they do not qualify a TLS listener or real certificates.

The expected registry fingerprint uses compact JSON in Go struct field order,
nodes sorted by node_id. Input file digests are intentionally excluded to avoid
recursive hashing. The kit hashes original input bytes before adding machine_id.
Observer registration must use that original input_sha256, not reserialized data.

## Open wire issues

- Agreed encoding: Registry release.public_key is BASE64; existing kit release
  public_key and local signing pin remain HEX. Integration must decode the locally
  pinned hex key to exactly32 bytes, compare decoded registry key bytes, and
  encode those same bytes as base64 when constructing the registry. Incoming
  registry keys never establish trust. Existing release-v1 signature is unchanged.
  Fixture records both representations.
- Agreed shared identity constraints: letters/digits/underscore/hyphen, first
  character a letter/digit, maximum101 characters. Product is aligning to this
  existing kit constraint; do not accept dotted or longer identities.
- Observer connection/TLS settings are not fields in the current Registry struct.
  The outer schema4 envelope and exact hash boundary still need agreement.
- Current BeginOrResume returns phase barrier before node registrations. This is
  admission protection, NOT permission to initialize/seed/activate. Register
  returns registered for one node; it does not prove all three registered.
- No readiness, activation, barrier-clear or invitation-renewal API exists yet.
  No bootstrap success may be inferred from current registry/barrier acceptance.

Kit schema4 acceptance stays OFF. Shared fixtures must cover all-node registration,
original-byte hash matching, lost response, immutable identity, wrong cert/key,
existing initialized pod refusal, same-operation resume and no processing grant.
Only then qualify the actual installer across three clean nodes.

## Reviewed transport checkpoint

Product now exposes source-only POST `/v1/bootstrap/{begin-or-resume,status,register,all-registered}`.
Request fields: protocol, operation_id, registry_sha256, generation, exact registered
node; input_sha256 appears only on register. Begin uses generation0. Status may
reconcile0 or verify the persisted generation. Register/all-registered require a
positive persisted server generation. Matching responses must retain protocol,
operation, registry digest and generation; processing_allowed must be false.
Phases are barrier, registered (one node), all-registered (three nodes).
All-registered is still not DB initialization/readiness/activation permission.

`protocol.py` is an isolated request/response validation helper, not an installed
or network-enabled client. Five tests cover exact binding/generation, allowed
phases, refusal of permission, partial registration, unsupported actions and
ambiguous response JSON. Run `python3 -m unittest discover -s
scripts/qualification/pod-bootstrap -p 'test_protocol.py'`.

Before connecting this helper: TLS1.3 CA/hostname/Observer pin verification,
protected client credentials, no redirects, bounded I/O, durable original input
hash and generation persistence, interruption reconciliation, invitation renewal,
and stage execution must pass joint native tests. No schema4 acceptance changed.

## Isolated registration client (2026-09-17)

`client.py` is source-only qualification tooling. It is not shipped by the admin
kit and cannot install, initialize databases, clear maintenance, or grant active
processing. Schema 4 remains disabled in the shipped installer.

The client validates the independently pinned Ed25519 release identity and local
architecture, original-input receipt/hash, local machine/updater/role binding,
unique registry identities, protected credentials and journal paths. TLS 1.3
requires a trusted CA, hostname validation, client certificate, and exact Observer
leaf-certificate pin before an HTTP request is sent. The operation intent and
server generation are persisted before subsequent mutations. Lost responses use
status for the same operation; registration retries retain the same input hash.
`all-registered` explicitly returns `installation_complete:false`.

Run the local tests with Python supporting TLS 1.3 and `cryptography`:

```
python -m unittest discover -s scripts/qualification/pod-bootstrap -p 'test_*.py' -v
```

23 tests passed with Python 3.13 / cryptography 44.0.3: protocol binding, signature
and architecture checks, duplicate identities, durable restart, lost responses,
disk failures, pending timeout, real loopback mTLS, untrusted CA/hostname/client
certificate failures, pin mismatch before HTTP, redirect refusal and size limits.
Fixture tests replace the root-protected file reader to run unprivileged; this is
not evidence of a root-owned production filesystem qualification.

Remaining gates before installer integration: protected infrastructure identity
receipt/attestation (currently only registry uniqueness is checked), finalized
schema-4 envelope, detached invitation validation and durable renewal ordering,
absolute wall-clock network deadline (current sockets have bounded inactivity
and coordinator deadlines), root filesystem integration, live product endpoint
integration, stage execution, rollback/retry acceptance, and clean VM qualification.
No activation or deployment qualification is claimed by these tests.

Reviewed schema-4 coordinator envelope: `application.bootstrap_coordinator` contains
exactly `registry`, `observer`, `invitation_file`, and `authorization_key_file`.
Observer contains `endpoint`, `ca_file`, `client_cert_file`, `client_key_file`,
and `certificate_sha256`. Invitation/key paths are detached protected files.
The original input never supplies generation; only the durable updater receipt
records the Observer-issued generation. This registration-only client verifies
the detached files are protected but does not accept invitations or execute any
provisioning stage. Product-side invitation acceptance remains a separate gate.

`authorization.py` adds isolated request/response validation for `authorize` and
`authorization-status`. Exact invitation ID and RFC3339Nano UTC expiry (including
nanoseconds), operation, registry and generation must match for lost-response
reconciliation. A newer renewal is a conflict, not evidence the old invitation
was accepted. Expired status can be inspected but cannot count as authorize
success. No network integration or stage execution is enabled by this helper.
The expanded suite passes 29 tests; native endpoint integration remains pending.

Authorization transport integration now follows registration in the isolated
client. It verifies the detached Ed25519 invitation using the separately pinned
hex authorization key, exact original binding, UTC lifetime (maximum one hour),
and exact-byte signature domain. Intent is persisted before authorize. A lost
response or existing receipt uses authorization-status with exact invitation
identity/expiry checks. Expired existing invitations permit read-only status;
expired invitations never initiate authorize. The expanded suite passes 35
local tests. This is mocked endpoint integration plus real transport qualification,
not independent evidence of the product's native endpoint tests. Explicit renewal
reconciliation and no-authorization-409 recovery remain fail-closed manual
integration gates. No provisioning stage execution or kit shipment is enabled.

Typed missing-authorization reconciliation is now supported: only HTTP409 JSON
`authorization_not_recorded` with the exact protocol, operation, registry,
generation, node, original-input hash and `processing_allowed:false` permits one
same-invitation authorize retry. Its signature and current lifetime are rechecked
immediately before retry. Generic409, mismatches and expired invitations block.
37 local tests pass. Automatic renewal remains unimplemented; a changed invitation
requires explicit reconciliation rather than silently replacing the local intent.

## Product data-stage adapter

`data_stage.py` prepares the exact artifact-owned command and validates config and
receipt shapes against snapshots of SiemCore's `pod-bootstrap-data-v1` contract.
It enforces original registry/node/generation/input and release receipt bindings,
expected schema/seed phase, and false installation/processing flags. Fixture
invocation durably records potential partial commit before calling the injected
runner; failed exit, timeout or invalid output retains it. Same-operation retries
preserve binding; changed config requires reconciliation. It never removes product
receipts, data, replication slots or the Observer barrier.

There is deliberately no Docker/process launcher, CLI entrypoint or kit wiring for
this adapter. The eventual qualified runner must verify the signed image, establish
protected read-only mounts (including matching TLS and host identity), exclude the
Docker socket, restrict networking, verify runtime readiness and current Observer
authorization, and bound process lifetime/output. Source config validation alone
does not establish these prerequisites. Product source schemas were copied from
`deploy/cluster/updater/contracts/pod-bootstrap-data-v1.{config,receipt}.schema.json`.

45 local tests pass with Python 3.13, cryptography44.0.3 and jsonschema4.26.0.
Data-stage tests use an injected runner, not a live PostgreSQL/container deployment.
Full signed A/B/Observer installer qualification and runtime/activation stages remain
outstanding. Normal installation/update paths are unchanged.

## Bounded qualification runner

Product corrected the common bundle executable to `/app/siemcore` (the earlier
`/app/cyfox-siemcore` path belonged to the legacy image). Adapter and runner now
use `/app/siemcore pod-bootstrap-data --config /run/bootstrap/data.json`.

`data_runner.py` supplies a qualification-only process runner with mandatory
artifact-image and runtime/authorization verification callbacks. There are no
permissive default verifiers or production entrypoint. It requires an existing
immutable local Linux image ID and internal bridge network ID; pulls are disabled.
Only protected root-owned bootstrap, TLS and PostgreSQL socket directories may be
mounted, read-only. No Docker socket, privileged mode or implicit image volumes.
The process has memory/CPU/PID limits, read-only rootfs, dropped capabilities and
no-new-privileges. Combined output and wall time are bounded. Interrupted daemon
containers are killed and checked stopped, then retained for evidence; data,
slots, receipts and barriers are never removed. Stderr is not included in receipts.

51 local tests pass. These include real local subprocess output/deadline checks
and fixture-based Docker command/inspection/termination checks. No signed image
was launched by Updates in this test run. Image trust, mount contents, isolated
network peers, effective runtime readiness and valid authorization must all be
established by the forthcoming end-to-end qualification harness before invocation.
No kit delivery or VM changes are enabled by this module.

PostgreSQL socket compatibility correction: only the `/run/siemcore-postgres`
mount leaf may use an independently verified dependency UID and exact01775/03775
mode. New mandatory `verify_socket_dependency` callback must verify the pinned
PostgreSQL image/runtime identity and return its nonroot UID; no hardcoded UID
establishes trust. Ancestors remain root-owned and nonwritable. The leaf must
contain the expected `.s.PGSQL.5432` socket (optionally its regular lock file),
owned by that UID, with no symlinks, unexpected entries or hard links. Other mount
leaves remain root-owned private. The runtime verifier must also establish that
root-owned read-only TLS copies match PostgreSQL's TLS bytes; the PostgreSQL-owned
private key directory cannot be mounted directly for the capability-free command.
54 tests pass, including narrow socket exception and rejection boundaries.

External native handoff now completed against the disposable arm64 common-image
fixture: wrong generation rejected, schema preparation succeeded, exact retry
succeeded, expired invitation rejected. See
`docs/verification/pod-bootstrap-runner-20260917/README.md` and response receipts.
This uses a test-signed manifest and separate fixture trust, never production
release trust. 58 distinct local tests pass (duplicate imported unittest class
collection removed). Full clean-POD signed-release acceptance remains outstanding.

## Per-node stage coordination

`data_orchestration.py` coordinates the reviewed data-node schema stage and optional
seed stage after authenticated registration/authorization. The planned stages and
both configuration hashes are persisted as one immutable operation before any
runner is created. Schema and seed retain separate receipts, so adding the seed
field is not mistaken for changing a completed schema-stage input. On retry both
artifact-owned stages revalidate through fresh verified runners; successful local
receipts alone never authorize a skip. A seed failure preserves schema progress
and all retained product state. Planned source or stage-set changes require explicit
reconciliation. The final internal status is `awaiting-product-readiness`, with
installation_complete=false and processing_allowed=false.

Witness cannot invoke the data command. Its runtime/bootstrap contract remains a
separate product-owned input. This module is not imported by the shipped kit and
cannot perform activation, assign role IPs, infer an initial active node, delete
VMs or publish releases. Runner factories must present each exact planned config
at the protected container path and recheck runtime/authorization before execution.
63 local tests pass; the new multi-stage coordination is fixture-tested, while the
preceding live local handoff qualified the schema-stage runner specifically.

## Observer management receipt

`observer_stage.py` implements the source-owned `pod-bootstrap-observer-v1`
config/receipt shapes. It prepares the verified host binary command with both
`--health` and `--receipt-config`, immutable witness/updater/pod/operation bindings,
and recorded drain/update configuration digests. A plain configuration-validation
exit0 is never sufficient. Only `observer-management-verified` with exact release,
registry, input, node and generation and both completion/processing flags false is
accepted. Interrupted verification retains an incomplete receipt; same-operation
retry is allowed, changed plans/settings require reconciliation. No authority
service installation/start or host runtime launcher is supplied by this adapter.
The future verified runner must check artifact/configuration integrity and bound
execution; product owns systemd MainPID, executable continuity, TLS/quorum and
pre/post authorization checks. Native whole-service qualification remains pending.
67 local tests pass; Observer adapter testing is fixture-only.

The agreed immutable seed declaration is `application.selective_sync` in the
original schema4 input, with exactly `{source_node_id:"1",target_node_id:"2",
allowlist_version:2}` for this clean-rebuild qualification. Data orchestration now
requires original input bytes, verifies their existing hash and pod/schema binding,
and rejects an inconsistent seed target/source or unsupported allowlist version.
The declaration never grants active/standby processing roles. The existing
`application.bootstrap_coordinator` object is unchanged.71 local tests pass.
