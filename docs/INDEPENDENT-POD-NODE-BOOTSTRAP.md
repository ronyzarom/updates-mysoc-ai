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

## Planned CLI (not yet enabled)

The existing clean installer will use `--server-type pod-node --node-id 1`
(or `2`) with `--greenfield-input /root/node-install.json` and the normal
parent/enrollment/signing options. No `--pod-id` is supplied. Production binary
capability output deliberately does not advertise the new type yet. Installer
and runtime also explicitly refuse unqualified independent-node delivery.
Updater self-update remains the normal stable channel and alpha fleet group;
product channels and prerequisites remain separate.

## Qualification

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
