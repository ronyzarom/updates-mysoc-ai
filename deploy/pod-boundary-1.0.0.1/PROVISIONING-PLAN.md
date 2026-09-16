# Current A/B prerequisite plan — pending exact-artifact gates

This is a reviewable plan, not an executed deployment. Do not publish or offer
SiemCore 3.3.152.32 as part of this prerequisite installation.

## Scope and immutable inputs

Project: osherad-graylog. Provision only these exact existing instances:

| Node | Zone | GCP numeric instance ID |
| --- | --- | --- |
| A | me-west1-a | 7631443060152631202 |
| B | me-west1-b | 5012156962629255074 |

Re-read /etc/machine-id and the protected updater_instance_id immediately before
preparing each assignment. Require exact values in installer arguments; do not
infer either from the VM name. No wildcard or shared alpha label targeting.

Boundary 1.0.0.1 SHA-256 inputs:

- boundary.py: ce846a61a6252670cad87a77a0f67e4ec44476e847201af8665469d59a3523ac
- install.py: 727c12e89b8c0ab69359c997eeb44b239526bf6de0f30f2e719d59f7232e3b96
- wrapper: 81d88b4dd798ae1877d5bc84fd9656e712b0e8a6fd9ec7f6706a1c6d1c4ed0bd
- Required existing legacy hook: 221fe046129211fcd8b421e495134e5bf5054b9934c74f0029a2e49667b95a88

The provisioning package still requires an externally verified signature and
recorded archive digest; files.json alone is not a signing trust boundary.

## Gates before current-host execution

Native systemd and installer tests passed on the disposable OS Config VM.
Boundary adapter tests and the retained .30 extracted-function fixture pass.
These do not replace exact signed-artifact compensation/rollback qualification.
Complete the negative missing-transaction test and reviewed retained rollback
fixture, record scope and limitations, and review the resulting evidence first.

## Execution sequence

1. Confirm live owner and health with SiemCore. Requested product sequence is
   current owner A, then B, then witness; revalidate ownership before use. This
   prerequisite boundary only applies to A/B, not witness.
2. Enable OS Config at the individual VM metadata level only. Preserve all other
   metadata. Verify attached service account, agent connectivity and inventory.
   No project-wide enablement, new broad IAM roles, reboot or SSH host writes.
3. Prepare an assignment with a unique per-instance selector AND an execution
   guard checking numeric VM ID, machine ID and updater identity before mutation.
4. Confirm no update/apply/rollback is in flight. Through the approved non-SSH
   route, stop only siemcore-cascade-updater.service; verify inactive and no
   surviving product phase. Do not kill an in-flight application operation to
   satisfy this prerequisite. Record service state for restoration.
5. Verify the signed provisioning package privately as root. Run install.py with
   exact machine/updater IDs. Installer preserves legacy hook, application policy,
   sudo policy, application services, product data and fleet/channel assignments.
   Versioned implementation is installed before the atomic wrapper replacement.
6. Verify installed file hashes, ownership and modes. Verify protected policy and
   sudo files are unchanged without recording their contents. Restart the updater
   if it was running before this operation. Require fresh heartbeat and unchanged
   installed application version/health before proceeding to B.
7. Remove the enforcing assignment after verification so it cannot reinstall a
   boundary that an operator deliberately rolls back. Record metadata disposition.

## Failure and restoration

If prechecks fail, leave the host unchanged. Before any new boundary transaction,
restoration may atomically restore the exact pinned legacy wrapper after verifying
no phase is running, then restore the updater's original service state. Preserve
the legacy hook and versioned boundary implementation for audit. Never bypass or
delete an existing transaction to downgrade the wrapper: reconcile it first.
An enforcing OS Config policy must be removed before intentional restoration.

Do not change database contents, perform migrations/restores, restart application
containers, change product release offers or claim application rollback qualified
as part of this prerequisite. Product delivery remains a separate held operation.
