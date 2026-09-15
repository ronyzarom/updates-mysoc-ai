# Shared updater clean-pod qualification candidate 1.16.1.21

Scope: replacement Bezeq test pod A/B/witness only. SiemCore owns cloud replacement and inputs. Preserve A reserved IP 34.165.133.248, direct endpoint and SSL.com TLS. No bench/production changes or database copying. This document does not report live qualification or publication.

The kit combines existing greenfield installer from updates-greenfield commit 4ff1dbc with the current shared updater. Product root hook must be from SiemCore commit adc1e494b99128033b346aad0b589f42ee1c41d4, SHA256 221fe046129211fcd8b421e495134e5bf5054b9934c74f0029a2e49667b95a88.

Updater changes retain origin signature in the filesystem receipt, reserve retry state before execution, exponentially delay the same product/version/digest/build from 60 seconds to 15 minutes, and retain heartbeat/self-update operation during delay. Changed target/digest/updater build gets a new attempt. RetryProduct is an in-process authorized-call API, not a CLI or permission to edit a running daemon state file. Retry deadline/count are optional heartbeat fields. Unknown/partial native failures still roll back.

Only apply exit 78 and a sole canonical MYSOC_APPLY_RESULT_V1 JSON line with phase apply, mutation none and allowlisted code prerequisite_failed, signature_invalid or receipt_invalid skip native rollback. Trusted product root hook must generate it before invoking product code; child output cannot impersonate it. Local current and original previous pointers are restored. This is not a crash-durable product transaction journal.

## Fresh host requirements and invocation

Use Linux systemd hosts with root provisioning, Python >=3.9, OpenSSL with Ed25519 pkeyutl support, sudo/visudo, coreutils and useradd. Kit installs no OS packages itself. The verified signed SiemCore bootstrap owns Docker and controlled application prerequisites. Provide parent connectivity and product-approved dependency sources. No application installer replaces the updater binary.

Root-owned private input JSON must contain application and release. Application is schema2/topology pod, pod_role a/b/witness, cluster_id and unique updater_instance_id; a/b also require logical instance_id and database_name. SiemCore owns remaining product fields and new machine identity; helper binds local machine-id. Release requires version, sha256, public_key, signature and channel.

Use isolated release.channel `pod-qualified-20260915` and independent updater channel `pod-updater-20260915`. No release has been published on either channel by this preparation. Explicitly enroll/reconcile just new nodes to alpha before publishing alpha-only; never omit target groups. Product/version identities cannot be republished on another channel through current API, so qualification versions are dedicated to this channel.

```sh
sudo ./install.sh --clean --greenfield-input /root/pod-input.json \
  --instance-id "$UPDATER_NODE_ID" --parent-url "$PARENT_URL" \
  --parent-id mysoc-testing-mysoc-ai --customer-id "$CUSTOMER_ID" \
  --customer-name "$CUSTOMER_NAME" --license-key "$ENROLLMENT_CREDENTIAL" \
  --signing-key "$PUBLIC_SIGNING_KEY" --self-update-channel pod-updater-20260915
```

Resolve credentials through existing protected provisioning; do not paste them into logs. Add --ca-file only if relay uses the approved private CA. Installer starts service after root hook/input/receipt configuration. Repeat identical bootstrap preserves installed-version/config; channel flag applies on first installation, and repeat bootstrap does not rewrite it.

Qualification: signed .25 fresh bootstrap A/B/witness; verify receipt signatures and health. Publish fixed .26 to same isolated application channel only after product readiness; routine upgrade plus controlled post-mutation failure and rollback to fixed .25; exact pre-mutation rejection must preserve both managed pointers without native rollback. Verify failure/restart retry delays and continued heartbeats, then same-channel signed updater self-update/restart. No wider promotion until real-host evidence is recorded.
