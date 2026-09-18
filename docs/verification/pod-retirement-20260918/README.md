# Old Bezeq POD retirement — scoped delivery preparation

User-authorized retirement scope communicated by SiemCore is exactly eight old
VM names in osherad-graylog: pod-test A/B/witness, rehearsal-02 A/B/witness,
updater-clean A/B. Cloud deletion/inventory remains owned by SiemCore. Historical
inventory is not proof of current VM state or completed deletion.

Confirmed enrollment checkpoints in the SiemCore runbook match exact central
registration UUIDs and immutable-ID-suffixed instance IDs for pod-test A/B/witness.
On2026-09-18, central product updates were disabled for A and B; witness was
already disabled. `product-holds.json` records exact matches and before/after.
A transaction checked every fleet control row and preserved all unrelated flags,
rings, status/tombstones, customer and parent identities. No release target changed.

This toggle gates product updates only. Updater self-update deliberately bypasses
it; do not claim a complete delivery freeze. SiemCore will stop positively
identified VMs before deletion, which also stops their updater processes.

No registration was newly decommissioned or deleted by this action. Rehearsal
registrations and cleanB were already tombstoned; cleanA was absent from the
candidate-name query. Other historical pod-test IDs were not conflated with the
current matched installation IDs. Fresh cloud confirmation is required before
final retirement matching/decommission, especially the clean VM identities that
lack immutable VM IDs in their registered updater names. Preserve all history,
logical POD/MySoc identity, customers, testing and benchmark assignments/holds.
