# Fresh Normal A installation — 2026-09-19

The user replaced the old independent-node recovery with a fresh Normal server.
Old updater `siemcore-node-4a3160d7-956f-4e9b-8c0c-3bceb525b91d` is held
(`auto_update_enabled=false`) and soft-decommissioned, retaining audit history.
Do not execute its repair/successor kits or reuse its immutable VM/updater identity.
SiemCore owns deletion of the exact old VM/dedicated disk and provisioning the new
VM, preserving reserved addresses, DNS, MySoc logical application identity and Observer.

## Installer candidate

- Updater: **1.16.1.33**, existing signed Linux amd64 binary, stable channel.
- SiemCore kit: **1.16.1.33-r1**, SHA256
  `ed55ab3d755e60064fd89ab9f14b5caf5ba0fe996dfe73449aea697b32a629f8`.
- Product provisioning source: `cff389c36e2464e5f2155625988ecf421902675a` (.44 common artifact source).
- The candidate contains no credentials and does not publish an application release.
- Qualification: 17 envelope/type tests, installer syntax and all package member
  checksums pass. Updater .33 signed automatic delivery/restart/heartbeat passed on
  MySoc, Normal testing SiemCore and Observer. **Fresh Normal product bootstrap
  with .44 is not yet qualified.** Prior .44 GCS evidence covered transition from
  an existing independent node, not this Normal clean path.

## Invocation

Use only on the new empty Linux amd64 VM with Docker and Tailscale prepared.
Root, systemd, Python >=3.9, OpenSSL Ed25519, sudo/visudo and coreutils are required.
From the verified extracted kit, after preparing protected input/credentials:

```sh
sudo ./install.sh --clean --server-type normal \
  --greenfield-input /root/provisioning/normal-a.json \
  --instance-id "$NEW_UPDATER_ID" \
  --parent-url https://testing.mysoc.ai:18443 \
  --parent-id mysoc-testing-mysoc-ai \
  --customer-id "$CUSTOMER_ID" --customer-name "$CUSTOMER_NAME" \
  --license-key "$ENROLLMENT_CREDENTIAL" \
  --signing-key 1f1aa11a80d6ac549a26bb25daac4798c42dd469138680e68f89832bd32e7f57 \
  --self-update-channel stable
```

Use the existing customer mapping, a new unique updater identity, and fresh VM
machine identity. The installer writes local machine identity for Normal. No
`--pod-id`, `--node-id`, Observer credentials, maintenance authority, pod role or
management-only executor is supplied. Central Fleet must show alpha and automatic
updates enabled; verify effective local self-update channel stable independently.

## Protected input contract

Root-owned mode0600 file under a root-owned directory. Exactly two top-level
objects: `application` and `release`.

`application` contains:

| Field | Value |
|---|---|
| `schema` | `3` (explicit GCS integration) |
| `topology` | `single` |
| `cluster_id` | New Normal installation identifier, product-approved |
| `instance_id` | Existing logical application ID `siemcore-bezeq-pod-test` |
| `updater_instance_id` | New unique updater ID, exactly matching CLI |
| `database_name` | Product-approved fresh database name |
| `frontend_url` | `https://bezeq-pod-test.siemcore.ai` |
| `mysoc_url` | `https://testing.mysoc.ai` |
| `admin_email` | Product-approved administrator |
| `mysoc_api_key` | Existing approved application credential, privately provisioned |
| `archive` | Complete object below |

`archive` has exactly `backend: gcs`, `project_id: siemcore-cluster-cyfox`,
`bucket: siemcore-archive-bezeq-pod-test`, `location: me-west1`,
`storage_class: STANDARD`, product-approved integer `retention_days` (1–36500),
`auto_provision: false`, and `authentication` either `{ "mode": "adc" }` with
verified runtime IAM or `{ "mode": "file", "path": "/root/provisioning/gcp-archiver.json" }`.
File credentials must be separately root-provisioned; never included in the kit.
No paid AI key is supplied; verify all AI providers disabled after bootstrap.

`release` has exactly the approved version, sha256, public_key, signature and
product channel. Obtain values from a signed published release, not a fabricated
receipt. Proposed isolated product channel `normal-a-20260919` fits the installer
20-character limit. The previous 24-character standalone channel does not.
SiemCore must approve product target and qualify Normal bootstrap before publication.

## Acceptance

Verify full customer UI and trusted HTTPS, MySoc registration/SSO, healthy fresh
PostgreSQL/Redis, ingestion and detection, real GCS upload/download/checksum,
paid AI disabled, same-input interrupted retry, and cascade success reporting.
Normal health must not depend on Observer availability. Preserve customer/DNS/IP
identity, never copy an old database, and keep the benchmark hold unchanged.
