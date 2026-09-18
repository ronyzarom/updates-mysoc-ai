# Independent node adoption executor — offline scaffold

This directory is **not installed, published, advertised, or wired into the
updater**. All four entry points (`readiness`, `apply`, `status`, `recover`)
return `link_contract_not_qualified` with nonzero exit status. There is no enable
flag. These responses do not constitute a final product wire schema.

## Offline contract integration

`product_contract.py` snapshots SiemCore's reviewed parser (`e1374ec`), SHA256
`6f5d6b7617afc01d41b43a28fa44428e43edb8af0f88a9c1bbe865e24a3687b4`.
`admission.py` compares the entire binding against a supplied protected-policy
binding, checks plan bytes, and rejects equivalent Observer/customer origins.
`release_verification.py` verifies both archive byte digests and Ed25519 detached
release signatures in the existing `mysoc-release-v1` domain.
`journal.py` exercises private, locked, atomic admission persistence and rejects
conflicting replay. Its only phase is `admitted-awaiting-product`, never completed.

These are offline libraries, **not a qualified root adapter**. The protected
policy/key loader, current-host identity/receipt measurement, independently
authenticated Observer barrier, safe archive staging and manifest/binary checks,
product execution/recovery, and verified adopted-state persistence are still
required. The signature domain binds product/version/archive digest; commit,
architecture and binary digest must also be measured from the verified archive
before execution. No caller-supplied callback or policy becomes authorization.

The original independent database receipt must remain unchanged, including its
installation binding and empty PodID. A separate adoption proof must establish
the linked relationship; never rerun database initialization or rewrite the
original receipt. Existing socket/network layout must be adopted by the product
workflow, not inferred from a metadata change.

Local tests: `python3 -m unittest scripts.tests.test_node_link_scaffold
scripts.tests.test_node_link_contract scripts.tests.test_node_link_admission
scripts.tests.test_node_link_journal` (20 tests). These are fixture checks, not
native adoption qualification.

## Integration boundary

The separate `pod-node-link-v1` workflow must never invoke the legacy greenfield
bootstrap or `pod-node-update-v1` to perform adoption. Their independent/linked
identity refusals remain intact. Original bootstrap receipts remain immutable;
adopted state belongs in a separate durable transaction record. Normal server
installation and updates are outside this workflow.

Before implementing mutation, agree with SiemCore on:

- Exact request schema and signed product executable/health contract.
- Immutable machine, installation, updater, and node-slot binding; source
  artifact and original bootstrap receipt digests.
- Signed target product/version/architecture/commit/artifact identity and trusted
  signing provenance, with local verification before execution.
- Desired pod/logical application identity, Observer identity/endpoint,
  expected registry digest and generation, peers, and protected credential/TLS
  references. Never copy credentials into operation status.
- Product-owned adoption plan binding the existing database volumes, sockets,
  configuration, and stable customer hostname without resetting bootstrap.
- Durable operation ID and exact request digest; lock acquisition, atomic
  journaling before mutation, replay conflict rejection, and restart recovery.
- Explicit, authorized installation-binding adoption after verified product
  completion, preserving machine/installation/updater identity. No direct edits
  around the current persisted-identity checks.
- Completion evidence proving application health and **processing disabled**.
  Adoption does not activate routing, create authority, or grant processing
  permission. Fresh Observer admission is a separate operation.

Qualification must cover signatures, identity mismatch, stale authority,
concurrent/replayed operations, interruption at each mutation boundary, lost
responses, retry, database preservation, paused completion, and unchanged Normal
and old independent-node rejection behavior. Native product integration tests
are mandatory before capability advertisement or a signed installation kit.
