# Node B baseline preparation — no installation authorization

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
