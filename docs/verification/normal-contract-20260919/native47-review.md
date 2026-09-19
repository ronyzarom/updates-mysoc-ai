# Native Normal .47 evidence review

Reviewed SiemCore's sanitized `/tmp/normal-bound-native-20260919/kit-result47.json`
and its `normal_bound_native_runner.py` assertions. This is review of a
product-team execution, not an independently repeated live installation.

Confirmed report:

- SiemCore `3.3.152.47`, artifact SHA-256
  `7cf8b7b0011344ca8f568bb023d6bd69e0f01fc9dbd3e14271727da0b8172812`.
- Native AMD64, kit `1.16.1.33-r3`, original signed updater `1.16.1.33`.
- Successful first-install cascade report from `0.0.0`, Normal identity.
- Exact installer retry preserves containers; changed input rejected.
- Updater restart produces Normal heartbeat; service active, ProtectHome enabled.
- Self-update enabled on stable; alpha is **fixture-only**, not a real fleet assignment.
- TLS trust, hostname and exact served certificate checks pass on 443, 6514, 18443.

Six local cross-contract checks also pass against exact product source `c2fcb24`
(full resolved revision in `result-product47.json`). Hook and all three packaged
recovery files are unchanged from r3's pinned provisioning source `cd5f94f`.
No new kit revision is required solely for the product source change.

The original report does not contain the runner's subsequently added
`verified_runtime_dependency_images` field. Separate `runtime47.json`, committed
by SiemCore in `d5aa901` under
`docs/operations/evidence/normal-bound-native-2026-09-19/`, was subsequently
reviewed: all four runtime prerequisite IDs match the reviewed dependency set;
etcd, Patroni, Redis, nginx, application and archiver are healthy with zero
restarts, and login HTML is available. This is subsequent runtime evidence,
not an assertion executed by the original installer runner. It does not prove
authenticated login or archive retrieval.

Updates installer/capability implementation has no identified blocker for the
qualified fixture path. This does not make r3 a production-trusted download:
its outer repository signature uses an ephemeral fixture key. Before a real
Node A installation, use an origin-signed distribution manifest and product
release receipt with the independently pinned production trust root, preserving
the exact qualified bytes or retesting changed bytes. Do not reuse test pins.

Still required before live installation: approved product prerequisite/security
handoff; protected service-visible paths under `/etc`; actual SSL.com material
and endpoints; real host/application/updater identity and MySoc configuration;
and explicit execution/publication authorization. Preserve existing holds.
Verify central alpha assignment plus effective local stable self-update, actual
installed version, restart, heartbeat and application health after any eventual
authorized rollout. A fixture heartbeat does not prove live alpha targeting.

Archive/GCS, real MySoc/SSO onboarding, paid AI, load and full product update/
rollback acceptance are outside this bootstrap evidence. No production database
copy or live host change was performed for this review.
