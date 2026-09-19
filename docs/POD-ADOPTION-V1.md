# POD adoption v1 — Updates integration proposal

Status: implemented **offline admission library and unavailable CLI**, not an
installed host executor. No kit, publication, fleet changes or capability
advertisement. Existing Normal installations and updates are unchanged.

SiemCore owns desired settings, invitation-based node-key possession proof,
admin registration approval, data adoption/mirroring and authenticated Observer
readiness. These UI actions create pending desired membership only. A registered
key is not proof of machine ID, updater ID or permission to activate processing.

Updates owns independent root measurements, signed artifact/authorization
verification, durable operation admission and eventual update-executor routing.
A qualified host loader, product apply/recovery integration and atomic membership
adoption are still required. The library's trusted-key, measurement and clock
arguments are in-process integration seams; never accept these as remote inputs.

## Exact authorization envelope

UTF-8 JSON, at most 32,768 bytes, duplicate and unknown fields rejected:

```json
{
  "protocol": "pod-adoption-v1",
  "binding": {
    "protocol": "pod-adoption-v1",
    "operation_id": "12345678-1234-1234-1234-123456789abc",
    "source": {
      "installation_class": "normal",
      "machine_id": "<32 lowercase hex>",
      "installation_id": "<measured retained installation identity>",
      "updater_instance_id": "<measured updater identity>",
      "version": "3.3.152.47",
      "artifact_sha256": "<64 lowercase hex>",
      "bootstrap_receipt_sha256": "<64 lowercase hex>"
    },
    "target": {
      "product": "siemcore",
      "version": "3.3.152.49",
      "architecture": "linux/amd64",
      "source_commit": "<40 lowercase hex>",
      "artifact_sha256": "<64 lowercase hex>",
      "binary_sha256": "<64 lowercase hex>"
    },
    "membership": {"pod_id": "pod-test", "node_id": "2"},
    "observer": {
      "installation_id": "observer-test",
      "endpoint": "https://observer.example.org",
      "public_key_sha256": "<64 lowercase hex>",
      "registry_revision": 1,
      "registry_sha256": "<64 lowercase hex>"
    },
    "desired_settings": {"revision": 1, "sha256": "<64 lowercase hex>"},
    "approved_node_key_sha256": "<64 lowercase hex>",
    "adoption_plan_sha256": "<64 lowercase hex>",
    "issued_at": 1789820000,
    "expires_at": 1789820300,
    "expected_state": "linked-paused"
  },
  "authorization_signature": "<base64 Ed25519 signature>"
}
```

Version `.49` is illustrative, not reserved or published. Hash placeholders are
not valid requests. Normal source admits only slots `1`/`2`. Observer source uses
class `observer-unlinked`, slot `witness`, and must match its own Observer
installation ID. Normal source cannot claim that identity. Neither is current
ACTIVE/STBY authority. Downgrades are rejected.

Sign bytes `mysoc-pod-adoption-v1\n` followed by canonical binding JSON: recursively
sorted keys, compact separators, ASCII escaping, no floats/nonfinite values.
The signature key must come from independently protected operator authorization
policy using the existing approved trust, never the registration request or
Observer invitation. No production authorization issuer/policy loader is shipped.
The new signature domain does **not** introduce a new trust root. Fingerprints
are SHA256 of raw 32-byte Ed25519 public keys. The Observer pin identifies its
application signing key; HTTPS must separately validate its certificate/hostname.

Timestamps are integer UTC Unix seconds; validity is issued_at <= now < expires_at
and maximum lifetime 900 seconds. Every admission retry rechecks time/signature
and all independent measurements. Expired requests never authorize new mutation.
Recovery after expiry will need a separately qualified reconciliation contract;
this implementation cannot resume host mutation or renew an existing operation.

## Independent evidence and artifact binding

Root must measure actual machine ID, persisted updater installation class and
updater identity, exact original receipt bytes and retained source archive. The
Normal installation ID mapping must be agreed against the existing receipt;
never invent an ID to fit an application registration. Bootstrap receipts remain
unchanged. The application cannot supply authoritative measurements.

Verify both retained source and target release archives using the existing
`mysoc-release-v1\nsiemcore\n<version>\n<archive sha256>` Ed25519 domain.
Then safely inspect the verified target manifest and binary to measure version,
architecture, commit, required product capability and binary hash. The library's
`verify_release` covers exact bytes and signature; safe archive staging and
manifest/binary measurement are **not yet implemented by this adapter**.

Desired settings and adoption-plan digests must cover exact protected bytes.
Settings revision, approved node fingerprint and registry revision/digest need
an independently authenticated current Observer snapshot. Compare the entire
measured binding, not just a matching version. Invitation replay protection is
owned by SiemCore, and does not replace this root admission.

## Files, persistence and CLI status

- `deploy/pod-adoption-v1/contract.py`: strict schema, authorization and release
  signature checks, comparison against independently measured binding.
- `deploy/pod-adoption-v1/journal.py`: offline locked atomic admission journal.
  Caller supplies an already securely opened private directory descriptor.
  File accesses are descriptor-relative, no-follow, private-owner checked;
  hardlinks are rejected. File and directory fsync surround atomic replacement.
- `deploy/pod-adoption-v1/cli.py`: all four actions return exit 78, unavailable.
- `scripts/tests/test_pod_adoption.py`: synthetic signatures and filesystem tests.

Proposed future host directory is `/var/lib/siemcore-pod-adoption/`; it is not
created/installed by this work. One installation-wide lock/journal prevents
concurrent or conflicting operations. Journal phase is only
`admitted-awaiting-product`, with `host_mutation=none` and
`processing_authorized=false`. An identical retry revalidates current evidence;
a different binding is rejected, even under another operation ID. No completion,
archival, membership rewrite or bootstrap retry is implemented. Original
installation state, databases and authority state are never touched.

`python3 deploy/pod-adoption-v1/cli.py readiness|apply|status|recover` returns:

```json
{
  "protocol": "pod-adoption-v1",
  "action": "readiness",
  "capability_enabled": false,
  "adoption_required": true,
  "status": "unavailable",
  "operation_state": "unknown",
  "error_code": "host_adoption_not_qualified",
  "reasons": [
    "protected_identity_loader_not_qualified",
    "product_host_adoption_not_qualified",
    "membership_executor_routing_not_qualified"
  ],
  "processing_authorized": false,
  "mutation": "none"
}
```

The CLI does not read a live journal and must report unknown, never absent or
completed. This is a proposed product-consumable result shape, not an HTTP
endpoint. No arbitrary command/callback or caller-supplied file path is accepted.

## Remaining qualification before enablement

Agree the host product apply/health/recovery protocol and signed capability,
protected policy/identity loaders, preserved database/volume plan, authenticated
Observer maintenance/registry checks and crash reconciliation. Adopt membership
and coordinated update routing only after measured paused completion. Retain
Normal origin identity separately from adopted operational membership; never
relax persisted installation checks globally or leave linked hosts using the
Normal executor. Observer-unlinked adoption additionally requires preserving
its durable authority state and enabling the Observer-specific maintenance path.

Test interruption at each real product mutation, expiry/revocation, concurrent
operations, stale registry, lost responses, replay, database preservation,
unchanged Normal updates and fresh processing permission after completion.
Offline admission tests are not native adoption or activation qualification.
