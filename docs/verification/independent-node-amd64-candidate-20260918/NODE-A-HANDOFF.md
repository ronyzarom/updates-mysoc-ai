# Node A installer handoff (prepared, not executed)

Await qualified common product artifact before publication or installation.
Use version 3.3.152.40 only after checking again that it is unused. Product
channel `node-a-20260918`, alpha only; updater channel `stable`.

Protected input inventory reviewed without reading credential contents:

- GCP VM: bezeq-pod-test-a, me-west1-a, ID7472527367529867136.
- Machine: bed377a24032485c8a58d8734eab6368.
- Installation: node-d6e43db4-e83e-4e7d-939e-909b7644961a.
- Updater: siemcore-node-4a3160d7-956f-4e9b-8c0c-3bceb525b91d.
- Node slot: 1, topology node-unlinked, schema5.
- Management: https://bezeq-pod-test-a.siemcore.ai:443.
- TLS: /etc/ssl/siemcore/fullchain.pem and privkey.pem.
- Settings: /etc/siemcore-node-inputs/settings.json,
  SHA256 bed04a7e9c9364774cd85a8675c7a00584931af60a8379c5dfe5364cb53bcd69.
- Local enrollment credential exists as protected a/updater-enrollment.secret;
  confirm its protected remote path before use. Never reuse Observer credentials.

**Still unresolved in the collected parameters:** deployed parent enrollment
contract and live alpha membership. The proposed URL https://testing.mysoc.ai
must be checked against the Observer's actual cascade parent URL/port and TLS
trust. Do not assume the application HTTPS endpoint is the relay endpoint.

After receipt verification, create exact `{application,release}` envelope at
`/etc/siemcore-node-inputs/bootstrap-envelope.json` (proposed path, root0600).
The release object needs version/channel/checksum/signature/public_key from
verified read-back. Verify the signed kit manifest/archive and packaged SHA256SUMS
before running. Populate the following variables from protected inputs, without
shell tracing, logging credentials or placing values in documentation:

```bash
# Illustrative invocation only; no values may remain unresolved at execution.
./install.sh --clean --server-type pod-node --node-id 1 \
  --greenfield-input /etc/siemcore-node-inputs/bootstrap-envelope.json \
  --instance-id siemcore-node-4a3160d7-956f-4e9b-8c0c-3bceb525b91d \
  --parent-url "$VERIFIED_PARENT_RELAY_URL" \
  --parent-id mysoc-testing-mysoc-ai --customer-id testing-mysoc-ai \
  --customer-name testing-mysoc-ai \
  --license-key "$PROTECTED_NODE_ENROLLMENT_CREDENTIAL" \
  --signing-key "$VERIFIED_FLEET_PUBLIC_KEY" \
  --self-update-channel stable \
  --relay-cert-file /etc/ssl/siemcore/fullchain.pem \
  --relay-key-file /etc/ssl/siemcore/privkey.pem
```

Only node A may receive this product channel. Confirm supported central alpha
assignment after enrollment, preserve every other fleet row/hold, and verify
signed apply, service restart, cascade heartbeat and exact application health.
An installed-unlinked node must retain processing/authority/POD/link readiness
false. Do not report bootstrap success from publication or CLI capability alone.
