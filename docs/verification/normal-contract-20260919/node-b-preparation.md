# Node B preparation and enrollment record

## Final acceptance — 2026-09-19

B successfully installed SiemCore `3.3.152.47` using origin-signed kit
`1.16.1.33-r3` and updater `1.16.1.33`. Updates independently read local state:
Normal installation, stable self-update channel, successful apply at
09:19:24 UTC with the expected `7cf8b7b0011344ca8f568bb023d6bd69e0f01fc9dbd3e14271727da0b8172812`
artifact digest, no pending retry products, empty operation state and active service.

SiemCore performed an orderly B-only GCP stop/start after correcting the known
`.47` archive default through the supported configuration API. It reported all
six containers healthy, persisted Normal receipt, no repeated apply, successful
login and exact SSL.com certificate/trust checks on 443, 6514 and 18443. GCS
provider initialization and consumer startup succeeded after the automatic DB
startup retry. Archive upload/retrieval has not been qualified.

Updates independently verified the postboot origin heartbeat at 09:23:47 UTC:
`.47` running, updater `.33`, alpha, automatic updates enabled, expected parent
and customer, successful apply and no error. See
[node-b-postboot-rollup.json](node-b-postboot-rollup.json).
The origin still stores `installation: null`; local Normal identity is verified,
but end-to-end central installation-type reporting remains an unresolved gap.

B remains Normal and unlinked. Access is through Tailscale at
`https://bezeq-pod-test-b.siemcore.ai` (`100.81.8.61`). A, Observer, benchmark
hold and release metadata were not changed for B. Product evidence is in the
SiemCore repository under `docs/operations/evidence/node-b-normal-create-2026-09-19/`.

## Historical preparation and launch notes

Launch update 2026-09-19: SiemCore confirmed user-approved test-only residual-CVE
acceptance for B, final prerequisite PASS and exact r3 kit/envelope verification,
then launched the installer. Updates observed first registration and assigned
only UUID `a1d98dfb-69ae-4ed0-91b6-1f936abf6922` to alpha/automatic via the supported
API. All other fleet controls and the benchmark hold were unchanged. No release
metadata was modified. First heartbeat reports updater `1.16.1.33`, product
channel `normal-a-20260919`; application completion is pending at this entry.

B prerequisite evidence supplied by SiemCore:
machine SHA `6d28e11dce6ed709fc4f72860c6f33f96e89e64f676b0ab4e8dd37410c982e59`,
manifest SHA `470b7da94f259d56b4b304172b4ec10d4203e955d9f69dd2ca068638139f5c29`.
Explicit customer mapping is `testing-mysoc-ai` / `Testing`; B retains its own
logical application identity and no A application/customer assignments.

Earlier preparation history:

Subsequent authorization: SiemCore relayed the user's go for B preparation and
installation, conditional on B's own security gate. Shared use of the existing
`.47` product channel by A+B is explicitly accepted; no release metadata change
or broader ring targeting is needed. Tailscale address: `100.81.8.61`.

Allocated updater identity (not enrolled):
`siemcore-normal-b-90881d89-d9b0-4e41-8643-3cf758903cc1`.
Planned logical application `siemcore-bezeq-normal-b`, cluster `bezeq-normal-b`,
frontend `https://bezeq-pod-test-b.siemcore.ai`, server type Normal, parent
`mysoc-testing-mysoc-ai`, updater self-update stable and planned central alpha.
B customer mapping must be explicitly supplied. Existing production-signed r3
kit and `.47` receipt signatures/checksum were reverified locally. No B
enrollment or live mutation has occurred in this task.

Original baseline record:

SiemCore reported creation of the dedicated B VM:

| Field | Value |
|---|---|
| VM name | `bezeq-pod-test-b` |
| Immutable VM ID | `8452243806140755915` |
| Dedicated disk ID | `2558450062049144779` |
| Zone | `me-west1-b` |
| Private IP | `10.89.0.3` |
| External IP | None |
| Baseline image ID | `4611828751800710765` |

Status reported by SiemCore: OS preparation in progress; no updater or
application installed. Updates has not independently checked this VM and has
not enrolled it, published a release for it, or changed its fleet settings.

Preparation scope only. B starts as an independent Normal installation;
future POD linking is separate. It needs unique updater and logical application
identities, explicitly prepared customer/MySoc/TLS/archive inputs and its own
security disposition. The Node A residual-CVE exception does not cover B.

Existing `.47/r3` bytes can be reused unchanged, but the current `.47` release
is on `normal-a-20260919` targeting alpha. Using it for B would intentionally
share the channel with A, not establish B-exclusive eligibility. No channel
change or B subscription has been performed. Preserve A, Observer and all holds.
