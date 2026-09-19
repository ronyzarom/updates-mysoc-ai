# Setup UI alpha eligibility — 2026-09-19

Read-only origin and Tailscale host checks. Candidate `.49` reserved with
SiemCore; source preparation `7ac88b3`. No release or fleet mutation yet.

| Host | Installed product | Product channel | Local installation path |
| --- | --- | --- | --- |
| A / siemcore-normal-db91d16e-97a5-452d-ae54-6db5c6d8f3bf | .47 | normal-a-20260919 | Normal completed greenfield hook |
| B / siemcore-normal-b-90881d89-d9b0-4e41-8643-3cf758903cc1 | .47 | normal-a-20260919 | Normal completed greenfield hook |
| testing / siemcore-testing-01 | .39 | stable | Legacy Normal recovery 1.0.0.4 |
| Observer / bezeq-pod-test-observer-8079452056575878166 | .48 | obs-test-20260918 | observer-unlinked-update-v1 enabled |

All four central alpha/automatic=true; updater 1.16.1.33. Actual local updater
self-update channel is stable on all four. Preserve benchmark automatic=false.
Other stale legacy POD entries remain alpha/auto=true/product stable; publishing
to stable/alpha would not provide the intended four-host scope.

A/B original bootstrap receipt hashes, respectively:
9aca575e64599e13e12e56fcc80673165fbc4797b7e3b3b2bd78c1bf971b5e52
and 3df55dc3a1f8e11376fea33e07a221988592f1d2db6ad02c67855312f1efea2b.
The installed root greenfield-hook.py prepares an exact predecessor/target
recovery policy from retained signed release receipts after completed bootstrap.
It does not require rewriting the original .47 bootstrap pin. Actual .47 -> .49
preflight, preserved mounts and rollback compatibility still need qualification.

Testing protected recovery policy is revision 8, SHA256
9cd577e406de989e0f35a7483fde902f0ed4f225f17cf2c22d0416a0b6fda6f1.
Its latest authorized transition is .36 -> .39. A .39 -> .49 addition must retain
all prior releases, trust, expected mounts and transitions, bind exact .49
signature/archive, and pass the installed native preflight before enablement.
No existing policy was edited. Exact candidate generation awaits the .49 signed
receipt and product compatibility evidence; a placeholder is not a valid policy.

Observer protected update policy remains enabled on obs-test-20260918, with
original bootstrap/component digests. It has no target-version update pin.
The signed target must preserve both independent Observer capabilities and pass
product state-preservation and service-write-path tests.

Agreed route: publish .49 on existing obs-test-20260918, explicit alpha, verify
Observer automatic apply and setup persistence first. Then migrate only A/B and
testing product channel to that existing scoped channel after each exact native
policy/preflight gate passes. No updater self-update channel changes, catalog
relabeling or wider target groups. Same artifact/version has one catalog channel;
duplicate versions/artifact kinds are not a channel workaround. POD adoption and
activation remain disabled. Follow-up must verify actual versions, restart,
health, TLS, archive/runtime settings, cascade reports and preserved receipts.
