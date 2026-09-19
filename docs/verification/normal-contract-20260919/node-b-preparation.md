# Node B baseline preparation — no installation authorization

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
