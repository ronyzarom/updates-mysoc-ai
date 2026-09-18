# Independent POD node bootstrap v1

Agreed with SiemCore on 2026-09-18. Source and fixture work only. Delivery,
publication, and fleet mutation are disabled pending joint qualification.

## Wire contract

The protected installer input retains the existing `{application, release}`
envelope and signed common SiemCore artifact. The exact application fields are:

```json
{
  "schema": 5,
  "topology": "node-unlinked",
  "machine_id": "0123456789abcdef0123456789abcdef",
  "installation_id": "node-one-installation",
  "updater_instance_id": "node-one-updater",
  "node_id": "1",
  "management": {
    "listen": "0.0.0.0:443",
    "hostname": "node-one.example.org",
    "certificate": "/etc/siemcore/tls/server.crt",
    "key": "/etc/siemcore/tls/server.key"
  },
  "settings_file": "/etc/siemcore/node-settings.json",
  "settings_sha256": "REPLACE_WITH_SHA256_OF_EXACT_SETTINGS_BYTES"
}
```

`node_id` is string `1` or `2`, an immutable slot, not active authority.
No POD ID, Observer, peer, registration, or role IP is required or permitted in
this envelope. `machine_id` must match the actual machine and is never rewritten.
The release object retains version, channel, sha256, signature, and public_key.
Checksums in examples are placeholders, not usable release inputs.

The settings file must be root-owned regular 0600 JSON, at most 65536 bytes,
with protected parents, no symlinks, duplicate keys, or digest mismatch.
SiemCore owns the settings schema and binds all referenced configuration and
credential bytes into its original durable operation before any mutation.
Replacing settings or credentials cannot bypass that operation on retry.

Immutable updater `server_type` and API `installation.kind` are `pod-node`,
with `node_id` and no `pod_id`. Linking and ACTIVE/STBY roles are later runtime
state. Linking preserves installation, machine, updater, and node identities.
An unlink-to-Normal transition is a separate explicit operation; it is never an
implicit installer reclassification.

## Execution contract and gates

The common signed MANIFEST must contain
`pod_node_capabilities: ["pod-node-bootstrap-v1"]` and the signed module
`updater/pod_node_unlinked.py`. The existing root-protected
`siemcore-apply-update apply|health` boundary verifies the archive, product,
version, signature, capability, and identities before invoking product logic.
Extracted mutable code is not an independent trust source.

Updates retains signed staging for exact retry and rejects a changed artifact,
version, or signature. A fresh bootstrap failure retains product state; it never
invokes generic rollback or reinitializes a database. The product journal must
make repeated completed bootstrap a health check, not another initialization.
Ordinary upgrades and linked maintenance require separately qualified paths.
No public dependency fallback is introduced.

Successful independent installation requires measured `management_ready=true`,
`data_ready=true`, and `installation_complete=true`, with exact version, binary,
and identities. Local PostgreSQL schema/write readiness and Redis readiness
must be real. Intermediate receipts remain incomplete. `link_ready=true` is
allowed only when authenticated linking is operational. State remains
`installed-unlinked`; `processing_enabled`, `authority_enabled`, and `pod_ready`
remain false. Bootstrap makes no Observer/peer calls and claims no role IP.

## Candidate kit CLI

Candidate kit `1.16.1.28-r1` uses `--server-type pod-node --node-id 1`
(or `2`) with `--greenfield-input /etc/siemcore-input/node-install.json` and the
normal parent/enrollment/signing options. No `--pod-id` is supplied. Keep all
referenced credentials and TLS material under protected `/etc` paths: the
systemd service retains `ProtectHome=true`.

The candidate binary advertises `pod-node`; this is an immutable installation
type, not permission to process traffic. The installer enables
`independent_node_bootstrap` only for `node-unlinked` envelopes and only when
`INDEPENDENT-NODE-BOOTSTRAP.json` binds the bundled hook to `PROVISIONING_COMMIT`.
Normal and Observer executor configurations do not gain this switch.
Updater self-update remains enabled on stable; alpha targeting is assigned by
the parent fleet policy. Local fixture alpha responses do not verify live alpha.

The dedicated packager `scripts/packaging/independent_node_kit.py` requires clean
committed Updates and product sources, an architecture-matching ELF binary,
and a valid Ed25519 receipt checked against an independently supplied public key.
It creates a new archive and checksum manifest without publishing anything.
Repository signing and publication remain separate release operations.

## Qualification history

The following records describe successive checkpoints, not current release approval.

Updates boundary fixtures cover both nodes, exact envelope validation, private
settings/digest/machine checks, immutable identity persistence, rejection of
reclassification, retained apply/health failure retry, and changed artifact or
version refusal. Existing Normal, Observer, and legacy POD tests remain required.

Joint clean/runtime acceptance is still pending: actual local data preparation,
protected management, no-peer installation, interruption and completed retry,
settings/credential mutation rejection, signed capability rejection, and
independent measured readiness. Do not call source admission tests an installed
kit qualification. No new kit version has been published for this contract.

### Updates verification (2026-09-18)

- `python3 -m unittest scripts.tests.test_independent_node_bootstrap scripts.tests.test_greenfield_bootstrap scripts.tests.test_installation_type`: 24 passed.
- `go test ./pkg/updatersim ./pkg/types ./internal/server/licensing ./cmd/updater-simulator/cmd`: passed (types has no standalone tests).
- `TestInstallationIdentityRetainedAcrossHeartbeats`: all 24 database cases passed on a new local PostgreSQL16 tmpfs fixture. Covers direct and relay heartbeat routes, omitted/mutated identities, and update-attempt reporting. Fixture removed afterward; no existing database was used.
- `bash -n kits/siemcore/install.sh` and `git diff --check`: passed.
- Root product lifecycle, real dependency installation, end-to-end signature execution, and clean-host readiness are not qualified by these boundary tests.

### Joint admission and reporting follow-up

Run `python3 scripts/qualification/independent-node/check-contract.py
--siemcore-source /path/to/siemcore-source` to compare both implementations.
All 31 cases pass against the product source on 2026-09-18. This uses invalid
fixture signing material and performs validation only; it does not qualify
signed delivery or runtime installation.

Updater retained-retry tests additionally recreate the Simulator before replay
and reject modified signatures and artifact kinds. Targeted Go tests pass.
The dashboard recognizes immutable `pod-node` and `observer-unlinked` identities
as read-only POD node/Observer labels without implying linkage, readiness, or
processing authority. Five badge tests and TypeScript checking pass.

### Product staging independently exercised

The product's isolated root fixture passed all 10 `test_node_local.py` tests:

```sh
docker run --rm --network none --entrypoint python3 \
  -v /path/to/siemcore/deploy/cluster/updater:/src:ro \
  siemcore-data-runtime-fixture:local \
  -m unittest discover -s /src/tests -p test_node_local.py
```

This verifies durable local staging, interrupted-write retry, identity and
credential binding, management certificate/key byte binding, and preservation
of existing data. It does not start the product or prove schema/application
readiness. Reviewed product source SHA256 at this checkpoint:

- `pod_node_local.py`: `76d52e177893611837201f60cfa080a687783ef70d9ebca4dd951d595da8cddb`
- `tests/test_node_local.py`: `2d261315b601e0742b3aefd06e9b488e64bd5b53373ef4d2723001ae6a5d292c`

Updates review identified the missing management TLS byte binding; the product
fix is included in these passing tests. Full runtime remains pending.

### Real runtime/schema follow-up (2026-09-18)

Independently rebuilt SiemCore `internal/podcontroller` native test binary for
Linux ARM64, then ran `FIXTURE_SCHEMA_TEST=1 bash
scripts/tests/node-unlinked-runtime.sh` from the product worktree. Passed:

- Real pinned PostgreSQL16 and Redis startup with no host ports or Observer/peer.
- Injected interruption after PostgreSQL creation; retry created Redis without
  replacing the PostgreSQL container.
- Authenticated application-user PostgreSQL write/rollback and Redis checks.
- Full shipped local schema initialization, repeated initialization, and verification
  (`TestNativeUnlinkedNodeSchema`, 0.39s).

The disposable fixture was cleaned up. This fixture exercises node1 runtime
and schema using synthetic bindings. It does not yet exercise both complete
installer envelopes, signed artifact delivery, management startup, or final
installation acceptance. Receipts correctly remain `installation_complete=false`
and `processing_enabled=false`. Delivery gates remain disabled.

The expanded native fixture was independently rebuilt/rerun after management
support landed: `TestNativeUnlinkedNodeSchema` passed (0.39s), and
`TestNativeUnlinkedNodeManagement` passed (0.03s) against real local data services.
This tests the management handler, including disabled mutations and dependency
failure status. It still does not qualify the TLS listener/login roundtrip or
the full signed installer. No delivery gates were changed.

### Compiled updater qualification

Both clean independent nodes subsequently passed first install driven by the
actual CLI, plus restart/identity heartbeat. See
[the qualification record](verification/independent-node-cli-20260918/README.md).
Execution now has an explicit default-off `independent_node_bootstrap` switch
for matching protected-node executors. The ordinary installer and published
capabilities remain gated pending versioned kit qualification; no fleet changes
are implied by these local passes.

### Versioned kit qualification

Candidate `1.16.1.28-r1` now passes actual `install.sh` plus systemd relay first
installation on both independent node slots, exact installer retry, restart,
identity reporting and authenticated management checks. See
[kit evidence](verification/independent-node-kit-20260918/README.md).
This is local Linux ARM64 qualification using disposable signatures. Live
AMD64, fleet alpha verification, release signing and publication remain separate.

## Deployment TLS requirement — user direction 2026-09-18

Every deployed TLS listener must use an SSL.com-issued server certificate with
its valid full chain and matching protected private key. This covers management
and customer HTTPS, ingestion TLS, exposed updater/relay HTTPS, Observer authority,
and database TLS. There is no self-signed server fallback for deployment.

Deployment validation must inventory all listeners, verify certificate issuer
chain, expiry, hostname/SAN coverage and key matching, and confirm the running
listener serves the approved certificate. Missing certificate inputs block that
listener's deployment; do not silently generate a replacement CA/certificate.
For exposed updater/relay HTTPS, supply both `--relay-cert-file` and
`--relay-key-file`; the existing generic self-provisioning default does not meet
this deployment requirement. This document records the gate; it does not claim
an automatic issuer-enforcement implementation or live certificate changes.

Private fixture CAs and loopback test certificates used in qualification are
isolated test inputs only. They must never be selected in deployment settings.
Product database TLS settings remain unresolved until a complete SSL.com-compatible
configuration is provided and validated; do not substitute the fixture CA.
