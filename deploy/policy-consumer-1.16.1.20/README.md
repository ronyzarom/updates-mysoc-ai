# Updates policy consumer 1.16.1.20 — local candidate, NOT deployed

Owner: Updates. This package is separate from the SiemCore universal application bundle. It is a root-consumer provisioning candidate, not a published updater self-update. No live host, sudo rule, application database or hold is changed by building/testing this package.

## Scope

One-time installer is pinned to testing hostname ip-172-26-13-54, its exact policy SHA ed4622f1cacb6aecac49687a87b30ba2dee3e3d7ba4c9627d55d7cd3290105a2 and the inspected original wrapper/runtime hashes. Requires updater already inactive and no pending recovery transaction. Keeps runtime1.0.0.4 and policy unchanged, retains original wrapper and replaces only the fixed root entrypoint with a delegate to this consumer. Existing sudo entries stay byte-for-byte unchanged. Installer does not stop/start services or release central holds.

Consumer accepts only apply/rollback. Rollback always invokes the existing runtime. Apply without a policy-request.json invokes existing runtime. Apply with a request performs ONLY policy authorization and returns; it never applies an application in that call. The future maintenance client must serialize this operation against product application, observe successful authorization, remove its request and then perform a separate normal product apply only when central policy permits. Do not configure current product apply to use a pending policy request: the old client does not distinguish the maintenance result from application success.

## Signed request

Fixed input /var/lib/siemcore-cascade-updater/policy-request.json, at most64KiB, regular file/no symlink, JSON duplicate/unknown fields rejected. Envelope contains payload and base64 signature. Signature uses existing root-policy Ed25519 public key and exact bytes:

`mysoc-policy-authorization-v1\n` + ASCII canonical JSON (sorted keys, compact separators) of payload.

Payload fields: protocol=mysoc-policy-authorization-v1, hostname, product=siemcore, old_policy_sha256, policy_revision, from_version, target_version, artifact_sha256, artifact_signature, source_commit (full40hex), issued_at/expires_at (Unix seconds, maximum1hour).

Origin policy authorization must be a distinct explicitly approved signing operation; ordinary artifact signature cannot substitute. No signing endpoint or automatic grant publication is included here. No new trust root.

Consumer builds the policy itself: next revision, one unambiguous forward edge, one fixed-path signed release entry, and target mounts copied from predecessor. Cannot change arbitrary topology, trust, host identities or retained rollback entries. Locks existing recovery journal, rejects pending transactions, authenticates origin grant then both artifacts using existing runtime preflight, checks live predecessor/configuration/storage, stores history and atomically swaps policy. Prepared receipt permits retry around an interrupted atomic policy replace. Neither root consumer nor grant clears holds. A request remains staged until its unprivileged maintenance owner removes it after success.

## Qualification evidence and remaining work

Local tests use real Ed25519/OpenSSL for authorization signatures and isolated temporary fixtures for candidate and transaction behavior. Seven test methods include11 invalid-field subcases and5 interruption points: additive scope, wrong host/product/policy/revision/predecessor/version/checksum/signature/commit/expiry/future time, unknown fields, cryptographic tamper/wrong key, duplicate/oversize/symlink input, idempotent retry and pending transaction. Runtime preflight is mocked in transaction tests; these DO NOT prove full native SiemCore application or root installation behavior. Existing runtime payload verification is reused with exact module hashes.

Isolated Linux installer fixture additionally passed first/repeated install, exact policy/runtime/sudo preservation, active-service refusal, pending-transaction refusal and runtime-drift refusal. The fixture uses a stub systemctl and contains no application/database; it is not live service-manager or real product qualification.

Still required: real service-manager/product consumer qualification; daemon-maintenance integration and origin authorization endpoint with signing scope/replay controls; updater signed self-update qualification; authorized non-SSH bootstrap route; actual signed .22 grant and retained-artifact preflight. Therefore NOT ready to unblock .22 and NOT authorized for installation.

## Exact one-time operator action (only after native qualification and delivery authorization)

Testing currently has no active privileged management agent; SSM absent and Salt inactive/unconfigured. No supported non-SSH operator-console/root provisioning route has been verified. Do not use SSH to run these commands, install an agent, or widen sudo. If an authorized native root console or host provisioning channel is supplied, the reviewed operation is:

1. Verify testing and bench holds still false, preserve Bezeq holds; stop testing updater through that authorized channel and verify no active executor.
2. Verify the delivered package SHA256 against the separately approved SHA256SUMS; extract as root into a root-owned0700 directory with `tar --no-same-owner`.
3. Run `python3 /root/updates-policy-consumer/install.py` from that approved channel.
4. Compare policy/runtime/sudo hashes with baseline; verify installed consumer and wrapper match files.json; preserve original wrapper history. Leave service stopped and holds in place until maintenance integration is qualified and explicitly enabled.

These are provisioning instructions for review, not a claimed available live route. If the host has no authorized non-SSH privileged entrypoint, this bootstrap is impossible with current unprivileged updater authority. Publishing a new binary cannot manufacture root permission.
