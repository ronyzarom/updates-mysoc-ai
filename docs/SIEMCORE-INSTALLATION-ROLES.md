# SiemCore installation by server role

The same SiemCore updater kit handles standalone, pod A, pod B, and witness.
Installation identity is recorded through `--server-type` (`normal`, `pod-active`,
`pod-stby`, `pod-observer`) with `--pod-id` and `--node-id` for pod hosts. Clean
installation may derive these fields from the protected `--greenfield-input` JSON.
Bootstrap is an artifact type, not a pod role.

## Agreed role lifecycle contract (2026-09-17)

This is the required product behavior, not a claim that all executors are implemented
or deployed. Observer is the third POD role, with its own bootstrap and update
sequence. Normal remains independent of POD credentials, observer availability,
maintenance barriers and role transitions.

| Workflow | Required sequence |
| --- | --- |
| Bootstrap — Normal | Verify prerequisites → initialize fresh database/configuration → install → verify health → enable processing. |
| Bootstrap — Observer | Verify prerequisites → establish pod identity, quorum and credentials → initialize durable authority/maintenance state → verify authenticated access → make authority service ready. |
| Bootstrap — POD data nodes | Register immutable identities → start management with processing disabled → initialize independent databases → synchronize selected tables → verify readiness → observer assigns active → transfer role IPs → enable active processing. |
| Update — Normal | Verify signed artifact/prerequisites → drain → update → restart → verify health. |
| Update — POD Active | Obtain and durably record observer maintenance ACK → explicitly start drain → verify paused → update → verify health → complete maintenance → obtain fresh active permission → resume. |
| Update — POD STBY | Obtain and durably record observer maintenance ACK → verify processing disabled → update → verify standby health and replication compatibility → complete → remain standby. |
| Update — Observer | Verify compatibility → persist maintenance protection and updater operation → update/restart authority service → reconcile existing assignments, generations and barriers → verify quorum/authentication → complete maintenance. |

Observer UPDATE must never invoke bootstrap initialization, create a new assignment,
reset token generations, discard durable barriers, or replace uncertain state with
defaults. Missing/incompatible authority state is a recovery error, not an empty
installation. Its dedicated restart/reconciliation path must work across its own
authority-service outage; the data-node path cannot assume the observer remains
available during observer self-maintenance.

Serialize all updates within a pod, including Observer updates. Maintenance blocks
automatic takeover but never extends processing permission indefinitely. If the
observer cannot renew permission during its restart, the active pauses at expiry.
The initial implementation accepts this controlled interruption; it must not claim
uninterrupted processing. Resume requires fresh authority after reconciliation.

The current shared data-node maintenance implementation pauses both data nodes.
The STBY sequence above does not claim that the active can already remain processing
during a standby update. That requires separate product qualification. Observer
execution currently fails closed pending its qualified dedicated lifecycle.

These workflows do not authorize production database copying/restoration, release
publication, readiness enabling, or deployment. Fresh bootstrap and updating an
existing installation remain distinct operations.

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

## Persisted server type (source implementation; not deployed)

For an existing standalone host add `--server-type normal` to the `--update`
command. Old invocations without the new flags retain their existing behavior.
For a data node supply, for example, `--server-type pod-stby --pod-id example-pod
--node-id 2`. An observer uses `--server-type pod-observer --pod-id example-pod
--node-id witness`. These fields do not grant processing, quorum, or IP ownership.
A/B map to node IDs 1/2; neither implies current primary. Without an explicit
active/standby label, greenfield data nodes record standby intent.

The installer preserves an existing recorded identity and rejects conflicting
inputs. For pod identity it probes the bundled binary's `installation-types`
command before rendering: older binaries cannot silently ignore the new role
fields. The probe is metadata support, explicitly not lifecycle qualification.
The private .25 candidate does not gain these features retroactively.

All types retain relay delivery. Explicit pod types cannot use normal application
execution as a fallback. Recording a type does not install a privileged adapter,
enable capability advertisement, or make a blocked pod bootstrap ready. Until
SiemCore supplies and qualifies the role-specific lifecycle, data-node bootstrap
without that adapter and observer-owned application updates fail closed.

Remaining external gates: observer-specific signed lifecycle and shared relay
endpoint failover qualification. The existing witness etcd/sentinel update script
is not treated as an allocation-observer update implementation.

## Read-only Normal / POD indication

The updater reports `installation.kind` (`normal` or `pod`) from its persisted
installation identity, plus `pod_id`/`node_id` for POD. This is independent of the
active/standby role and is never inferred from directory names. The server retains
the first recorded identity across direct and relayed heartbeats, including old
clients omitting it later. The instance page renders a locked, read-only badge.
There is no dashboard conversion control. Legacy instances without a recorded
identity display **Not reported**; they are not silently classified as Normal.
This protects administrative classification, not against a privileged host attacker.
