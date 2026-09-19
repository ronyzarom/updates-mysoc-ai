# Fresh Normal Node A installation

User authorization relayed by SiemCore included the residual-CVE exception for
this test A only. SiemCore owned host installation, approved protected inputs,
SSL.com checks and public-IP attachment; Updates owned signing, publication,
new identity alpha assignment and cascade verification.

- Updater identity: `siemcore-normal-db91d16e-97a5-452d-ae54-6db5c6d8f3bf`.
- Registry UUID: `722fc999-b49e-4262-836b-806a46310b68`.
- Parent: `mysoc-testing-mysoc-ai`, customer: `testing-mysoc-ai`.
- Logical application: `siemcore-bezeq-pod-test`; immutable VM `1028971160940844512`.
- Kit: exact `1.16.1.33-r3`; installed updater `1.16.1.33`.
- Installed SiemCore: `3.3.152.47`, artifact digest
  `7cf8b7b0011344ca8f568bb023d6bd69e0f01fc9dbd3e14271727da0b8172812`.
- Central alpha, automatic enabled; effective local self-update channel stable.
- Product channel `normal-a-20260919`; no other channel/fleet targeting changed.

Actual cascade offer/download/signed-hook apply was observed by SiemCore. The
apply completed at 2026-09-19 06:38:44 UTC. Independent Updates read-only host
verification found active updater, `.47` persisted, Normal installation identity,
successful exact-digest last attempt and no pending product retry. Origin
confirmed `.47` running and success with empty error in the next rolled-up
heartbeat. See `node-a-final-rollup.json` and `node-a-alpha-assignment.json`.
SiemCore's subsequently reviewed `updater-restart.json` confirms an updater-only
restart, unchanged application containers, preserved Normal identity and `.47`,
stable/enabled self-update, and accepted heartbeat.

SiemCore confirmed all six containers healthy, actual SSL.com trust/hostname/
matching leaf on 443, 6514 and 18443, then attached reserved public IP
34.165.133.248. Public health and login return 200. Stable URL:
`https://bezeq-pod-test.siemcore.ai`. SSO redirect uses the correct logical
application and callback; authenticated completion still requires user login.

Outstanding product functionality: SiemCore's final inspection found GCS input
and credential mounting did not set database `archiver_defaults`; runtime still
used LOCAL storage. Successful bootstrap/healthy containers do not establish GCS
archival. SiemCore owns correcting configuration and verifying actual archive
operation. Updater `.33` has no ad-hoc per-service restart/force-reapply API:
use a product-owned supported reload if available, or a qualified signed
successor/implemented maintenance operation. Do not clear transaction state or
use direct SSH Compose to bypass signed execution.

Reporting limitation: local config/state correctly identify Normal, but the
origin heartbeat snapshot's `installation` field is null. Do not claim central
type metadata persistence passed. This is separate from successful application
delivery/version/health. No origin-server reporting deployment was made here.

All other fleet controls and release metadata were compared before/after
scoped publication and new-A alpha assignment; benchmark automatic=false hold
was preserved. No production database copy or restore was used.
