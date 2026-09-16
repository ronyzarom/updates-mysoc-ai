# SiemCore installation by server role

The same SiemCore updater kit handles standalone, pod A, pod B, and witness.
There is no `--role` flag: the product role is supplied in the root-owned
JSON passed to `--greenfield-input`. Bootstrap is an artifact type, not a pod role.

## Role selection

| Host | Example input file | Fields inside `application` |
| --- | --- | --- |
| Standalone | `/root/provisioning/standalone.json` | `schema: 1`, `topology: "single"` |
| Pod A | `/root/provisioning/pod-a.json` | `schema: 2`, `topology: "pod"`, `pod_role: "a"` |
| Pod B | `/root/provisioning/pod-b.json` | `schema: 2`, `topology: "pod"`, `pod_role: "b"` |
| Witness | `/root/provisioning/witness.json` | `schema: 2`, `topology: "pod"`, `pod_role: "witness"` |

These are field summaries, not complete JSON files. Obtain complete inputs from
the SiemCore provisioning workflow. Inputs contain `application` and `release`.
The signed release receipt requires `version`, `sha256`, `public_key`, `signature`,
and `channel`. Application inputs include cluster identity, a unique updater
identity per host, and product-required topology, network, secrets and prerequisites.
A/B also require the logical application identity and database name. Keep input
files root-owned and private; never publish credentials in examples or logs.
A/B identify nodes, not permanently fixed active/passive assignments.

## Fresh installation command

Run from an extracted kit that includes the SiemCore greenfield provisioning hook.
Set `INPUT` to the appropriate file from the table and supply provisioning values:

```sh
INPUT=/root/provisioning/pod-a.json
sudo ./install.sh --clean \
  --greenfield-input "$INPUT" \
  --instance-id "$UPDATER_INSTANCE_ID" \
  --parent-url "$PARENT_RELAY_URL" \
  --parent-id "$PARENT_INSTANCE_ID" \
  --customer-id "$CUSTOMER_ID" \
  --customer-name "$CUSTOMER_NAME" \
  --license-key "$ENROLLMENT_CREDENTIAL" \
  --signing-key "$SIGNING_PUBLIC_KEY" \
  --self-update-channel stable
```

For testing, the parent URL is `https://testing.mysoc.ai:18443` and parent ID is
`mysoc-testing-mysoc-ai`. Add `--ca-file /root/provisioning/mysoc-relay-ca.pem`
when that relay uses its approved private CA. Where existing relay TLS is supplied,
add both `--relay-cert-file /path/fullchain.pem` and `--relay-key-file /path/privkey.pem`.
The current kit requires the enrollment and signing inputs shown above; this is
not yet a URL-only unattended installer.

Host prerequisites: Linux/systemd, root provisioning, Python >=3.9, OpenSSL with
Ed25519 support, sudo/visudo, coreutils and useradd. The kit does not install OS
packages; the verified SiemCore installer owns controlled product prerequisites.

## Normal alpha updater behavior

Testing standalone and pod nodes use updater channel `stable`, fleet group `alpha`,
and automatic signed self-updates. Set/verify alpha in Updates fleet management;
`--self-update-channel stable` does not itself assign the alpha ring.
Do not introduce a special pod updater channel unless explicitly requested.
Product release channel in the input receipt is separate; changing updater channel
must not silently change product release targeting. Preserve explicit user holds.

Existing enrolled nodes update automatically. Do not rerun `--clean` to upgrade them.
`--update --current-version VERSION` is for installing/configuring the updater
around an already installed product, not the normal application upgrade command.
Repeated greenfield bootstrap does not rewrite an existing channel setting.

Verify installed updater version, service restart, successful cascade heartbeat,
application health, and appropriate pod node roles. Publication alone is not proof
of installation. Product DR/rollback qualification remains a separate acceptance gate.
