# Independent POD node: standalone customer application transition

Configuration digest agreement: SHA256 of compact, sorted-key JSON mapping
approved relative input names to SHA256 of their exact bytes (`ensure_ascii=true`).
Required inputs are `application.env`, `installation/tls.crt`, and
`installation/tls.key`; include approved `archive-credentials/<basename>` and
`mysoc-bootstrap.json` only when required. Exclude `mode.json` to avoid circular
hashing and hash before creating it. The offline implementation is
`deploy/node-standalone-v1/configuration_inventory.py`; it requires an exclusively
locked private staging tree and protected owner/group/mode inventory, rejects
extras, missing files and symlinks, and never changes input bytes or permissions.
Four fixture tests pass; no executor is wired to this helper.

Product preflight still must establish that enabled paid AI providers are absent
or disabled in the retained database and no AI credentials are injected into the
environment. MySoc empty configuration may be initialized using its supported
admin command; existing mismatched configuration must fail without overwrite.
Runtime `/health/live` must show `installation_mode=independent-standalone`,
immutable IDs and `pod_authority_enabled=false`; acceptance additionally requires
`/health/ready` and native pipeline qualification, never liveness alone.

Status: proposed integration contract, offline only. No kit, capability, release,
or live activation is enabled by this document. Prior `node-link-v1` work is
preserved and paused pending this changed product model.

## Identity and operational mode

Keep immutable installation type `pod-node`, local node slot, machine ID,
installation ID and updater ID. Do not reclassify the installation as `normal`.
Operational mode is separate persisted state, backed by a verified root receipt:

| Mode | Application behavior | Update executor |
| --- | --- | --- |
| `independent-management` | Existing management-only, processing disabled | Existing qualified `pod-node-update-v1` |
| `independent-standalone` | Full customer application; local standalone processing, no pod authority | New separately qualified standalone-node executor |
| `linked-paused` | Drained, processing disabled while joining pod | Qualified adoption/reconciliation path |
| `linked` | Observer grants current ACTIVE/STBY processing authority | Existing qualified coordinated POD executor after explicit binding adoption |

These mode names are proposed; product wire vocabulary must be agreed before
implementation. Absence of a new receipt must never infer standalone permission.
For existing independent installations, retain current management-only behavior.
Conflicting/missing mode evidence blocks new transitions, without silently
selecting Normal or linked executors. Normal and Observer paths remain unchanged.

### Product runtime receipt alignment

SiemCore's current implementation consumes the following exact runtime receipt
at `/run/siemcore-installation/mode.json` inside the application container:

```json
{
  "protocol": "pod-node-standalone-v1",
  "mode": "standalone",
  "installation_id": "<retained installation ID>",
  "updater_instance_id": "<retained updater ID>",
  "node_id": "1",
  "instance_id": "<explicit customer application identity>",
  "version": "<verified target version>",
  "operation_id": "<durable transition operation ID>",
  "bootstrap_receipt_sha256": "<original receipt digest>",
  "configuration_sha256": "<agreed protected configuration digest>"
}
```

Use `mode=standalone` in this product wire receipt; the broader mode labels above
describe updater routing, not extra accepted receipt values. The root transition
must generate the file from verified retained identity and protected inputs,
never accept it as a caller-provided permission file. Bind machine ID, source and
target signed artifacts, binary/commit evidence and transition phase in the
separate root journal; these are not extra fields in the product receipt.

The planned host source is
`/opt/siemcore-node-standalone-<node_id>/installation/mode.json`, mounted read-only
into the container. Root-owned directories and file must not be writable by the
application; permit application UID 10001 to read the receipt and TLS material
without granting write access. Exact configuration hashing and secret-file
ownership must be agreed with the product executor before installation.

This receipt authorizes an application launch, **not successful completion**.
Persist the root operation before creating it; only mark the transition accepted
after measured full application health. Recovery must distinguish staged receipt,
running application and accepted state and cannot infer success from file presence.
The product planner keeps PostgreSQL/Redis containers intact, uses the retained
PostgreSQL socket and Redis internal network, and adds a standalone outbound
network. It starts app and archiver from the same verified image with pull disabled.
TLS files at `installation/tls.crt` and `installation/tls.key` must use approved
SSL.com material; product verifies hostname/system trust before database/workers.

## Separate enablement operation

Proposed protocol: `pod-node-standalone-v1`, with `readiness`, `apply`, `status`,
and `recover`. This is not the existing node update protocol and must not relax
its `processing_enabled=false` acceptance rule.

Bind one durable operation ID and request digest to:

- All immutable local identities, source version and signed artifact digest,
  original bootstrap receipt digest, and current mode receipt (if present).
- Signed target product/version/architecture/source commit/artifact and binary
  digests, measured from the verified archive with existing release trust.
- Exact source mode and requested `independent-standalone` mode, protected
  configuration/adoption-plan digest, preserved database/Redis/volume identity.
- Customer URL, SSL.com TLS material references, explicit application instance
  identity, protected application settings references, and paid-AI-disabled
  policy. Do not embed credentials in requests, receipts or fleet status.

Root admission must independently establish no linked membership/active pod
authority or unresolved prior transaction. It must not equate a missing file with
proof that another pod assignment is absent. Product readiness must define the
authoritative local evidence for a never-linked independent installation.

The product owns starting the full application against retained local data and
the required schema compatibility checks. Never rerun fresh database bootstrap,
overwrite the original database receipt, restore a database, or reuse legacy
greenfield execution. Retain original receipts and write a separate transition
receipt. Stage and journal before mutation; recover the same operation on lost
response/restart. Do not mark completed merely because a process is running.

## Health, future updates, and later linking

Acceptance must verify exact identity/version/binary, customer HTTPS/login,
database and Redis readiness, application readiness, ingestion, connectors,
archive behavior with controlled fixtures, enabled standalone processing, paid
AI disabled, and absence of pod authority. Management-only health is insufficient.

After success, persist the verified operational mode and route routine updates
to its dedicated executor: drain standalone processing, verify artifact and
prerequisites, apply, restart, verify full application health, then resume.
Interrupted updates must recover the same transaction; no fallback to the old
management-only executor or identity rewrite. Product defines data-compatible
recovery; generic database rollback is forbidden.

Later pod linking must first drain and prove standalone processing stopped,
adopt membership durably, and finish paused. Only fresh Observer permission can
then enable pod processing. Retain immutable installation and original database
receipts throughout. Standalone enablement must not grant pod authority, move
role IPs, or alter Observer state.

## Qualification gates

Product executable/schema and evidence contract must precede host integration.
Test original data/identity preservation, explicit opt-in, signed source/target
verification, application health and controlled pipeline probes, interrupted
enablement/retry, routine standalone-node update/recovery, conflicting membership
refusal, later drain/link refusal without fresh authority, and unchanged Normal,
Observer and old management-only behavior. Follow with isolated native tests.
No live mutation or release until concrete product implementation and joint
qualification pass. Updater self-update remains `stable` with fleet `alpha`;
product release scope and explicit holds remain independent.
