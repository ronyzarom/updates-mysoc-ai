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
