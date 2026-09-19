# Scoped alpha automatic-update acceptance — 2026-09-19

SiemCore `3.3.152.50` was delivered through the running updater cascade to testing,
the independent Observer, then Normal B and Normal A in sequence.

| Host | From → installed | Installation identity | Fleet / updater self-update |
| --- | --- | --- | --- |
| testing.siemcore.ai | 3.3.152.49 → 3.3.152.50 | Existing legacy Normal preserved | alpha / stable |
| bezeq-pod-test-observer.siemcore.ai | 3.3.152.49 → 3.3.152.50 | observer-unlinked | alpha / stable |
| bezeq-pod-test-b.siemcore.ai | 3.3.152.47 → 3.3.152.50 | normal | alpha / stable |
| bezeq-pod-test.siemcore.ai | 3.3.152.47 → 3.3.152.50 | normal | alpha / stable |

Installed updater: `1.16.1.33`, automatic self-update enabled, effective local
channel `stable`. Tested bootstrap kit: `1.16.1.33-r3`. A/B additionally received
the scoped signed recovery component `1.0.0.5`; the kit alone does not include this
new corrective component. Product channel `obs-test-20260918` is separate from
updater self-update. Release target groups are exactly `alpha`.

Release ID: `9db7cf87-1b65-453e-a097-18669ee522db`.
Source: `7832d56a0b41bbca0bdeaf82d7137bca5a4b2b58`.
Artifact SHA256: `9167378baaf99483a0a72b4f896c9d969e70bfab978052f5f6b482f6b01203bc`.

## What was corrected and verified

The `.49` application port gate compared address-qualified expected ports against
bare actual ports and incorrectly rejected matching endpoints. Product `.50`
corrects that comparison. Scoped recovery authorization pins each host's existing
observed ports before updating, retaining exact endpoints through rollback.

Three separate fresh native fixtures passed: direct upgrade; interruption after
pinning and recovery; and real systemd updater → root hook → recovery execution.
Tests include signed delivery, retry, repeated rollback, a real port collision,
failed-closed behavior, exact ingress retention, and final health. No initialized
database disks were copied to create fixtures.

Live A/B verification confirms six healthy containers, signed successful update
reports, preserved Normal identity and protected bootstrap records, exact original
ports, and applied recovery journals. Testing and Observer passed authenticated UI,
health and readiness checks. Observer remains unlinked, with authority and processing
disabled. No registration, adoption, or role activation was performed.

Canaries passed at least five minutes of continuous target health before B; B
passed at least five minutes plus authenticated UI before A. Final A soak passed at20:43:41 UTC with307 seconds of continuous healthy50;
all four central heartbeat/report records confirm50/success/alpha/automatic. A cached browser shell on B was resolved by a
normal browser reload; server files already contained the correct dashboard.

The benchmark automatic-update hold is preserved. No wider release promotion,
production database copy, database backup, or database restore was performed.

Detailed signed receipts, fixture results, per-host authorizations and live
verification are retained in [the evidence directory](../setup49-eligibility-20260919/README.md).
