# Live independent Observer bootstrap — 2026-09-18

Updates kit 1.16.1.26-r1 installed on immutable VM 8079452056575878166.
Updater identity: bezeq-pod-test-observer-8079452056575878166.
Application installation: observer-8079452056575878166-v1.

## Delivery and targeting

- Protected input pins signed SiemCore 3.3.152.37, channel obs-test-20260918,
  SHA256 59daa737b0084ddce921aedbd67bf67994cd42f93eef28f5da9644b63838d9fd.
- Updater started 03:55:25Z, actual version 1.16.1.26 / commit 849a21f.
- Fresh enrollment initially stable: first check correctly produced no offer.
- Supported admin PUT changed only UUID bfcdd816-b683-40bc-ac82-38b00c70e3a2
  to alpha. Before/after comparison preserved every other fleet control row.
  Fresh auto_update_enabled=true; effective local self-update channel stable,
  enabled. The benchmark remained held (auto=false, alpha).
- The updater alone downloaded/verified/applied the product. No SSH product
  installer/hook invocation occurred. Root apply was invoked by updater service.
- 03:56:36Z alpha offer;03:56:42Z checksum/signature verified 65,290,566bytes;
  03:56:44Z root apply;03:56:48Z health;03:56:50Z successful verified application.
- Origin success report at 03:56:50.187129Z. Cascade rollup at 03:58:01.873832Z
  reports updater 1.16.1.26 and product 3.3.152.37/running, correct parent/customer.

## Measured health and retained state

HTTPS with ordinary certificate validation reports exact product version and
binary SHA 75db1e54f9b1066e5a66fbc41756f2d12f57bd58b64dd26d8fc64705ca056a32,
correct installation/updater identities, installed-unlinked, management_ready=true,
and pod_ready/authority_enabled/processing_enabled=false.

Root durable journal is complete at 3.3.152.37. Policy SHA
9313216ea69000830a06d07cad45bed0d681f251f6f40585d4bf92b6a09f91ef.
Journal SHA 7d2d73e5c50ed7ef0d340e425a89a5832618498484d06cd5ee736edf9bb58499.

Updater-only restart 03:57:28Z succeeded; new PID 6652, automatic restart count 0.
Product PID 6542/start03:56:46Z and journal bytes were unchanged; state retained
observer-unlinked and installed 3.3.152.37. Next heartbeat accepted and no reinstall
occurred (product up to date).

Normal hosts were untouched. This qualifies initial independent Observer bootstrap
and completed-state persistence, not linking, authority activation, ordinary
Observer upgrades/rollback or a live destructive interruption test. Interruption
and failed-health exact-retry coverage remains the documented fixture evidence.

## Five-minute stability acceptance

Seven HTTPS/service/journal samples at 0, 50, 100, 150, 200, 250 and 300 seconds
all passed. Updater PID6652 and product PID6542 remained unchanged, active, with
NRestarts=0; root journal bytes remained unchanged. Full measurements are in
`stability-evidence.json`. Final central rollup at04:03:02.897723Z confirms alpha,
automatic updates, updater1.16.1.26, product3.3.152.37/running, success=true, and
correct parent/customer. Benchmark hold remains false/alpha. SiemCore separately
confirmed normal DNS and certificate-validated HTTPS through its own five-minute
client observation. No linking or authority activation performed.
