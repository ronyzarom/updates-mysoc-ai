# Independent Observer security update: implementation checkpoint

This is **not a signed or deployed maintenance kit**. Live remains updater
1.16.1.26 / SiemCore 3.3.152.37. Initial bootstrap was qualified separately
with 1.16.1.26-r1; that immutable kit is unchanged.

Implemented, default off:

- Protected root CLI, strict immutable operation binding, root-private signed
  archive staging, independent health/UI checks, retained journal and supervised
  product worker under the shared lifecycle lock.
- Updater opt-in `simulation.filesystem.observer_unlinked_update`, restricted to
  independent Observer. Pending transaction reconciliation runs before network
  checks, retaining operation/target identity and never invoking generic rollback.
- Acceptance alone permits updater-owned pointer publication. Product execution
  is through the verified root-private worker, never updater-writable extraction.
- First security hop only: completed acceptance withdraws new-operation capability.
  Failed target cannot restore the original open-UI predecessor. Product worker
  durably stops management and independent root observation confirms isolation.

Evidence:

- 15 Python coordinator/transport tests passed.
- Go updater and capability regression packages passed, including Normal isolation,
  strict privileged-response parsing, durable uncertainty, retained recovery and
  accepted pointer publication without mutable product hooks.
- Exact SiemCore worker/systemd fixtures previously passed success, target-health
  failure -> blocked/isolation, and SIGKILL -> retained reconciliation. The additional
  installed CLI fixture passes signed staging, operation execution, read-only retry
  and post-acceptance readiness. Component tamper, Normal role and linking reject
  admission without changing product PID.
- Container fixtures have no network, host mounts or customer credentials. Signed
  executable is a synthetic fixture, **not a qualified published SiemCore binary**.
  Component hashes record the tested local source; they are not a signature.

Remaining deployment gates:

1. Signed Updates maintenance kit and validated installed-runtime migration/rollback
   without product execution. No installer or activation policy is deployed yet.
2. Complete interruption matrix against the final installed kit and product build.
3. Signed isolated alpha cascade, authenticated operator UI acceptance and stability.

Do not enable the configuration flag on live hosts from this checkpoint. New
password-protected *fresh bootstrap* publication is a separate SiemCore operation
and does not qualify ordinary upgrades of the existing .37 Observer.
