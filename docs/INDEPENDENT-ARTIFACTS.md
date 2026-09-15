# Independent Bootstrap and Update publication

This extension lets SiemCore, MySoc and SWF publish one Bootstrap or one Update artifact without a companion release. Each has its own release ID, version, source commit and target groups. The same product/version can have one record of each kind. Existing paired releases and legacy records remain unchanged.

## Upload and lookup

POST `/api/v1/releases` using the existing authenticated multipart upload:

- `product`, `version`, `channel`, `target_groups`, optional `release_notes`
- `artifact_kind`: `bootstrap` or `update`
- `artifact`: one file
- `artifact_metadata`: one JSON artifact object, using the existing dual-artifact-v1 field names: `kind`, `product`, `version`, `name`, `arch`, `size`, `checksum`, `source_commit`, `required_dependencies`.

The server validates bytes, size, checksum and identity before signing and inserting the record. It generates `signature`, `metadata_signature` and `url`; no second artifact is required. The existing `artifact_variants` paired-upload request remains supported.

Qualification still uses `dual-alpha-siemcore`, `dual-alpha-mysoc` or `dual-alpha-swf`, with alpha as the only target group. This does not change any host channel or assignment automatically.

Read, download, edit and delete an independent record using the existing product/version endpoint plus `?artifact_kind=bootstrap` or `?artifact_kind=update`. Mutation of one kind does not affect the other. No-kind lookups retain the legacy identity. Existing paired/legacy releases reserve their existing product/version; use a new version when converting from those records.

## Negotiation and selection

Clients advertise `protocol_version: "dual-artifact-v1"` and both `verified-dependencies-v1` and `independent-artifacts-v1` capabilities. Clients without the new capability receive existing legacy/paired offers, never an independent thin update. Every relay in the download path must support independent metadata; an older relay rejects it rather than treating it as a full bundle.

For the requested product, channel, ring and architecture:

1. An empty installation receives the newest eligible bootstrap.
2. A completed installation receives the newest eligible update whose required digests and capabilities match fresh local evidence.
3. If no update matches, the newest eligible bootstrap is used. No downgrade is offered.
4. If an update exists but prerequisites do not match and no eligible bootstrap exists, return HTTP 412 with an explicit prerequisite error.

Versions belong to each artifact independently; there is no same-version or same-commit pairing requirement. No implicit dependency installation occurs in Updates. Product-owned bootstrap installers may install controlled prerequisites under their existing policy.

`artifacts` contains the selected independent artifact. The response retains top-level `download_url`, `update_url`, `sha256`, `signature`, and adds `protocol_version`, `selected_artifact_kind`, `dependency_validation`, and `required_dependencies`. `dependency_validation` is a string: `complete`, `missing`, or `mismatch`. The signed artifact's `required_dependencies` is authoritative; the top-level copy is reporting convenience. Existing checksum-based Ed25519 signatures and canonical metadata signatures are unchanged.

## Product verifier placement

Configure the verifier under the corresponding entry in updater YAML:

```yaml
products:
  - name: swf
    current_version: 2.3.0.0
    channel: dual-alpha-swf
    prerequisite_verifier:
      - 'C:\Program Files\SWF\verify-prerequisites.exe'
      - --json
```

Use an absolute platform-native executable path. Linux products use their own absolute path. Output one JSON object containing `lifecycle`, `installed_version`, and `cached_dependencies`. A dependency-free completed installation must return `[]`, not omit the field. Partial/unverifiable installations return nonzero. The verifier is read-only, limited to 30 seconds and 1 MiB output, and runs before requesting an offer and again during verification/application. Products own installation, health, retry and retained-local-artifact rollback. No database rollback is added.

## Storage and rollout

Migration 016 replaces the old product/version unique constraint with product/version/artifact-kind uniqueness. It does not rewrite release rows. Do not reverse this index after independent records exist: older uniqueness cannot represent both records. Application rollback may retain the additive index; do not delete either kind to force an old constraint.

Local qualification passes: all Go packages; race tests for protocol, server, publication and updater; PostgreSQL migration/publication/selection tests for all three products; two-hop independent cache separation/corruption recovery; signature/prerequisite verification; dashboard build and 29 tests. Tests prove protocol behavior, not installation of every real product.

Deployed to the Updates API/dashboard and alpha updater/relay as 1.16.1.14 on 2026-09-15 after SiemCore completed its .21 upload. testing.mysoc.ai automatically updated and reported .14. Existing paired publication remains available. Product canary qualification and broader promotion remain separate from enabling the uploader. See verification/independent-1.16.1.14/DEPLOYMENT.md.
