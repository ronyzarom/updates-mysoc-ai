# Dual-artifact v1 contract — implementation candidate

Status update 2026-09-15: API and dashboard 1.16.1.13 are deployed with isolated dual-alpha-<product> publication channels. See DUAL-ALPHA-READY-2026-09-15.md. Product qualification remains pending. The implementation notes below include historical predeployment evidence. No paired MySoc, SiemCore, or SWF release has been qualified. SiemCore accepted the additive vocabulary on 2026-09-15, subject to its product-owned verifier and host tests. SWF explicitly remains single-artifact-only in available evidence.

## Negotiation and rollout

The server flag `DUAL_ARTIFACT_ALPHA=true` permits alpha publication/selection only. Default is false. Clients must send both `protocol_version: "dual-artifact-v1"` and `capabilities: ["verified-dependencies-v1"]`. The cascade updater advertises these only for a product with an explicitly configured `prerequisite_verifier`. Older relays/clients and requests without both fields retain the original response/download contract. Self-update products never use this protocol.

Do not turn on the flag or advertise capability on a live product until reviewed clean/existing-host and rollback tests pass for that product. Flag activation alone cannot turn an existing single artifact into a pair. Existing release rows are not modified.

## Atomic publication

The existing release upload endpoint accepts `artifact_variants` JSON (exactly two artifacts), a `bootstrap` file, an `update` file, and shared product/version/channel/target_groups. Initially target_groups must be exactly alpha. An update-only upload is rejected. The JSON entries use:

- `kind`: bootstrap or update
- `product`, `version`, `arch` (OS/architecture, e.g. linux/amd64), `source_commit` (full 40-character hexadecimal commit)
- `name` (uploaded filename), `size`, `checksum` (64-character lowercase SHA-256)
- `required_dependencies`: objects with `reference`, immutable `digest` (`sha256:` plus 64 lowercase hex characters), and `capabilities` (exact product-defined strings)

Unknown publication fields are rejected. Both files must match the declared sizes and checksums. The origin assigns unique storage filenames and origin-relative download URLs; it signs both files and normalized metadata with the existing Ed25519 key. One release record owns both variants and its target groups. Promotion does not rebuild files. The bootstrap file populates all existing single-artifact columns. The collection is stored as `manifest.artifact_variants`; no database migration is needed.

Artifact `signature` retains the existing release signing message. `metadata_signature` uses that same signing message with SHA-256 of canonical Go JSON `{protocol, artifact}` as its digest. URL and both signatures are excluded; dependencies sort by reference and their capabilities sort lexicographically. All remaining artifact fields, including kind/product/version/arch/commit/size/checksum/name and the complete dependency requirements, are bound. The implementation and test fixtures in pkg/artifactprotocol are the canonical encoding reference. Do not implement an independently guessed serialization.

## Local verifier

Product configuration optionally declares an absolute executable and arguments:

```yaml
prerequisite_verifier: [/usr/local/lib/product/verify-prerequisites, --json]
```

It must inspect actual completed installation state, installed version, local immutable dependency digests and capabilities, without installation, network pulls, mutation, or directory-only inference. It outputs one JSON object:

```json
{"lifecycle":"installed","installed_version":"3.3.152.19","cached_dependencies":[{"reference":"product-owned-image-reference","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","capabilities":["product-defined-capability"]}]}
```

Fresh hosts report lifecycle empty only after proving that there is no installation to preserve. Partial/corrupt/unreadable or interrupted installs must fail the verifier (nonzero); they must not report empty or installed. Verifier failure, timeout, invalid JSON, and unknown lifecycle are fatal locally, before requesting an offer and before applying either variant. No invalid local inspection is converted into a bootstrap request. A dependency-free completed installation explicitly returns an empty list, not null/omitted. The updater executes the verifier with a 30-second timeout and 1 MiB output cap. Its reported installed version must match the updater's current product version.

The server chooses update only for installed state with a nonempty installed version and matching digest/capability evidence for every requirement. Missing/invalid network evidence selects bootstrap on the server, but does not authorize a client with a failing local verifier to apply it. A completed installation with verified identity but missing dependencies may select bootstrap only under a product executor that preserves its existing state. The updater verifies both signed variants, selected identity and architecture, then re-runs local evidence checks before download and immediately before apply. Any failure stops before executor apply and persists its reason. Product installers own controlled prerequisite installation on bootstrap. Updates does not invoke public registries/package managers or synthesize prerequisites.

## Delivery and status

Responses carry protocol_version, artifacts, selected_artifact_kind, dependency_validation and required_dependencies. Existing sha256/signature/download_url fields describe the selected bytes for negotiated clients. Each relay preserves variant selection and routes `?artifact_kind=bootstrap|update` through its own signed cache. Cache entries for variants are separated by kind and digest and rechecked on reuse.

Heartbeat `last_update_attempt` adds selected_artifact_kind, dependency_validation and artifact_digest alongside existing success/error/time. These survive local state persistence and the existing heartbeat JSON storage. The detail page and compact fleet-list response display them. Immediate dual-artifact reports persist in the same JSON, scoped to the authenticated license. Executor commands receive ARTIFACT_KIND, DEPENDENCY_VALIDATION and ARTIFACT_SHA256; local release receipts also retain kind and validation status. The product executor remains responsible for application health checks and local artifact rollback; this extension adds no database restore/migration or dependency download path.

## Remaining live gates

1. Product team provides verifier implementation, signed pair and clean/existing/interrupted/retry/rollback acceptance evidence.
2. Review the code and exact metadata serialization with each product team; add their canonical cross-language fixture when supplied.
3. Stage an isolated product canary with its verifier. Atomic publication, concurrent writers, corrupt-upload cleanup and promotion preservation now pass against disposable PostgreSQL; two-hop fixture delivery passes. These do not replace live product installation qualification.
4. Build the actual relay-capable updater from cmd/updater-simulator with clean provenance and a version newer than 1.16.1.12. Verify self-update/restart, then enable alpha only.
5. No automatic product publication, broader promotion, or changes to existing maintenance holds.

## Local verification on 2026-09-15

- Full Go suite including delivery fixtures passed; final verification is recorded in the task.
- Protocol tests cover all three product names, clean/completed/partial states, missing digests/capabilities, inconsistent identities and signed-metadata tampering.
- Recording-executor delivery fixtures cover bootstrap, update, changed evidence after download, interrupted download, persisted failure and retry for all three product names. They do not install real products.
- Two-hop relay fixture verifies legacy/bootstrap/update byte separation and corruption detection on variant cache reuse.
- Dashboard TypeScript check and 27 tests (including the paired-upload request test) passed. New upload controls are not browser-verified yet.
- Live flag remains unchanged/off; no binary or product release was published by this implementation work.

## Completion pass

Updates-side implementation now includes database-backed publication tests for all three products; concurrent publication and corruption cleanup; compact fleet-list status persistence; immediate report persistence restricted to the authenticated license; artifact context passed to executor hooks; local filesystem apply/retained rollback tests; and legacy relay responses without new empty fields. Full Go tests and focused race checks passed with the disposable database enabled. Dashboard type checks and27 tests passed. Product-specific live canary verification and signed release publication remain distinct rollout steps. No production host, release or database was modified in this implementation pass.

Reproduce integration tests by setting UPDATES_TEST_DATABASE_URL to a disposable PostgreSQL database and running go test ./.... Integration tests use private temporary schemas and clean up after themselves.
