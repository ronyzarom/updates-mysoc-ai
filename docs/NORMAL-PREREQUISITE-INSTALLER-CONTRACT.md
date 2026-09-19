# Normal prerequisite installer contract

Status: local implementation and fixture qualification; no new kit published or
host enrolled by this change. The existing `1.16.1.33-r1` kit does not support this
contract. Product installation and security qualification are separate gates.

The optional application setting is supported only for integer `schema: 3` and
`topology: "single"`:

```json
{
  "normal_prerequisites": {
    "schema": 1,
    "path": "/root/provisioning/normal-handoff.json",
    "sha256": "<64 lowercase hexadecimal characters: raw manifest SHA-256>"
  }
}
```

The enclosing release must declare exactly
`"required_capabilities": ["normal-prerequisites-v1"]`. This field is a
requirement, not proof of artifact support. The signed product manifest must
declare `normal_capabilities: ["normal-prerequisites-v1"]`; the product-owned
outer hook checks that declaration and its prerequisite executor before use.
The new application field independently permits schema 3 without archive
configuration. It is not accepted for POD nodes or Observer.

Updates validates a canonical absolute path, root-owned protected directory
ancestors, a non-symlink root-owned 0600 file of at most 65536 bytes, exact raw
SHA-256, and a JSON object without duplicate keys. It forwards the reference
unchanged. SiemCore owns the manifest's schema/profile, image identities,
security qualification, TLS validation and execution. Installer acceptance
does not grant security approval or permit public dependency downloads.

The kit must include `NORMAL-PREREQUISITES.json`, binding protocol, exact product
provisioning commit and outer-hook SHA-256. Packaging emits this marker only
when the product hook declares the protocol and its verifier. The marker is
covered by the kit checksums and signed archive. This declaration check does
not substitute for reviewing and testing the product hook. An old kit or
modified hook is rejected by installer preflight before service startup.

The existing root-private `/etc/siemcore/updater-bootstrap.json` execution
receipt retains `input_sha256` (raw input envelope) and, for this feature, adds:

- `schema: 1`, `protocol: "normal-prerequisites-v1"`;
- `application_canonical_sha256`, including the actual local `machine_id`;
- `prerequisite_manifest_sha256`, over the original raw manifest bytes.

Canonical application encoding is Python JSON with `sort_keys=True`,
`separators=(',', ':')`, `ensure_ascii=False`, `allow_nan=False`, UTF-8, and no
newline. This is an object hash, not a claim about installed JSON formatting.
Retries revalidate the protected manifest and compare the execution receipt
and stored application before starting the updater. A changed machine,
application or manifest is refused.

The preparation receipt's `configuration_sha256` attests the original bare
candidate bytes. Preserve that receipt unchanged: the installer cannot infer
those original bytes from the enclosing parsed input. Do not put an application
hash inside the prerequisite manifest, which would introduce a circular hash
dependency.

Inputs without this optional field keep the existing configuration serialization,
receipt shape and delivery behavior. Updater self-update channels, rings and
holds are unchanged.

Local verification:

```sh
python3 -B -m unittest scripts.tests.test_normal_prerequisites_bootstrap \
  scripts.tests.test_greenfield_bootstrap scripts.tests.test_installation_type \
  scripts.tests.test_independent_node_kit scripts.tests.test_independent_node_bootstrap
```

These 37 tests cover the new validation, capability marker, changed-input retry
refusal and existing Normal/POD/Observer boundaries. They do not establish live
product bootstrap, runtime health, security acceptance or a deployed kit version.
