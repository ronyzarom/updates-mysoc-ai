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

## Execution progress

`.49` published 14:50:49 UTC, ID d2ec0089-0c91-48a2-bbdc-f53e688fd8f5,
exact alpha / obs-test-20260918. Observer automatically applied at 14:51:17;
product confirmed authenticated setup GET/UI, unchanged identity, healthy .49,
strict service protection with only setup directory writable, activation false.

Testing passed installed recovery preflight; additive revision 9 policy SHA
abcfd4db04599a5d202a69b981bf8b2854cafcac922d17717aa528d84a112bb3
preserves prior fields, trust, mounts and transitions. Only testing product
channel moved to obs-test-20260918. Actual daemon signed cascade .39 -> .49
succeeded at 14:53:42 UTC; fresh 14:55:54 heartbeat/up-to-date logged.

B preflight blocked on dynamic published ports without invoking apply. Product
committed c6ee8f762ae45e7a139b2202dabb0abdce72cd94: exact policy-generator flag
for the existing narrow archiver8444/tcp allowance, with 28 tests passing.
Updates verified the exact diff and signed a root component repair restricted
to A/B identities and old/new hook hashes. B installed hook SHA
329e855866399e59e62219fcbd7d8b59087b472f60253c6ae95fe43e8ccedea8;
original bootstrap/config inputs unchanged. Root repair receipt included here.
No application was applied through SSH.

B repeat preflight then correctly refused dynamically mapped application ingress
12201/udp,1514/tcp,1514/udp. A read-only inspection finds the same condition.
No generic port guard was loosened. Both remain .47 on normal-a-20260919;
A root hook is still original. Product-owned ingress preservation/correction
must be qualified before either application update. The attached operational
scripts are exact-candidate records, not general installers; reruns must honor
current state, expected baselines, and these unresolved preconditions.

Independent testing runtime probes subsequently confirmed .49/healthy with
PostgreSQL and Redis ready, and archiver status ok. SiemCore's browser acceptance
confirmed .49 footer, Observer connection/enrollment form, blocked role switching,
archive health/zero upload failures and SSL.com443 validation. No registration
or settings mutation was performed. Central snapshots are in current-rollup.jsonl.

A/B ingress correction contract agreed with SiemCore: separate scoped signed root
runtime/authorization prerequisite may be installed without touching application
containers. Only later updater-owned .47 -> .49 execution can pin the exact
measured existing published ports, with original and normalized predecessor
configuration durably recorded first. Update and rollback must preserve effective
ingress. No general dynamic-app exception or manual Compose execution. Product
implementation and native qualification remain pending. Runtime baselines copied
read-only for review: fresh Normal recovery SHA89107cacaa08daad28b30e4e62ddd6214dd1e1d312d7f7b16cf49c60c1c0622e;
legacy testing recovery SHA95b5c64cb8ccf32a449bca2bf7851118abb1d55742bd4513846487010d0eccea.

## Native corrective qualification pending

Product runtime 1.0.0.5 committed as 0eb2669; independent 41-test suite passed,
including signed authorization/tamper, runtime/identity binding, drift before
mutation without rollback, and pinned predecessor recovery. Product drill
53ff822 requires separate direct-normalization and interrupted-pin cases before
live A/B eligibility. No qualification pass has been claimed.

Updates built isolated native Docker-in-Docker test infrastructure on B: no host
socket/mounts, no published ports, network none, image-only dependency copies and
synthetic installation inputs. First fixture stopped at missing sysctl tooling;
Dockerfile now includes procps. Second fixture stopped before bootstrap because
its network namespace hides net.core.netdev_max_backlog required by the signed
product prerequisite checker. No checker was weakened or mocked. Both task
containers are stopped and preserved; real B application remains .47.

Dedicated native .47 qualification VM identity recorded separately:
bezeq-normal-qualify47-20260919, VM7541999129262795054,
disk4918847062956812590, me-west1-a, private10.89.0.14. GCP CLI access was restored with user reauthentication. Native testing now runs
through IAP; A/B migration remains gated on the qualification results below.


### Native direct case: failed safely

The dedicated synthetic VM passed signed preflight without installed mutation.
Direct `.47 -> .49` then failed in the immutable product `updater/apply` step.
Recovery restored healthy `.47` (PostgreSQL and Redis ready), with exact original
published ingress endpoints preserved. Original transaction:
`/etc/normal-qualification/ingress-retention/c5ca1e130dfc46a4b0b9d6495a6a8656`.
Original output: `/root/port49-direct.log` on the qualification guest.

The runtime discards failed child stdout/stderr, so a separate fixture-only
harness captured failed `updater/apply` output to a root-only file without
modifying the protected runtime. Diagnostic transaction:
`/etc/normal-qualification/ingress-retention-diagnostic/8a4865d225124227a86c34f5c1a4b2f0`.
That retry also failed and restored healthy `.47` with all endpoints preserved.
The fixture is now pinned; diagnostic retries cannot count as fresh direct
normalization acceptance. Fresh direct and interrupted-pin fixtures are required.
No real A/B application change or new release publication was performed.

The restricted diagnostic confirms Gate A's product comparison bug: expected
`0.0.0.0:32774/tcp`, `0.0.0.0:32778/udp`, `0.0.0.0:32779/udp`, while Docker
actual projection returns those same ports without host addresses. No port drift
exception was used. SiemCore helper fix `51f1ebe` has eight passing regression
tests. Corrected `.50` candidate is reserved but not published; immutable `.49`
is unchanged. New baseline-only fixtures are being prepared:
`bezeq-ingress-direct-20260919` (VM4385378830207112216) and
`bezeq-ingress-crash-20260919` (VM5426915765356025313), both private me-west1-a.

## Corrected .50 qualification and canary preparation

Product `.50`, source `7832d56a0b41bbca0bdeaf82d7137bca5a4b2b58`, SHA
`9167378baaf99483a0a72b4f896c9d969e70bfab978052f5f6b482f6b01203bc`, passes
both fresh native direct and interrupted-pin drills, including actual port
collision recovery, repeated rollback, exact endpoint retention, and final health.
A third fresh VM (8169611730880778800) passes actual systemd updater delivery
through the installed greenfield hook and recovery runtime. See the three JSON
receipts. The initial integration harness final assertion confused persisted
`server_type` with heartbeat `kind`; read-only verification confirms exact original
installation identity and complete original bootstrap-input binding. No identity
change or application retry was needed to resolve that assertion.

Detached `.50` and root runtime signatures were produced at the existing origin;
private signing material was not exported. Testing's recovery policy was extended
from revision9 to10 for the exact49→50 transition and signed artifact, retaining
prior entries and expected mounts. SHA
`056a330397a4134e7a8a478541cd0880be9b65603c0e1edfd68f40272a82eb6c`.
The active journal remained unchanged; the application stayed healthy49. Its
updater is intentionally stopped until scoped publication, signed download, and
read-only preflight succeed. Publication is owned by SiemCore coordination.

Latest eligibility snapshot: obs-test-20260918 contains only testing and Observer,
both alpha/auto=true/49. A/B remain normal-a-20260919, alpha/auto=true/47.
Benchmark remains stable/alpha/auto=false. Subsequent live A/B rollout must be B
first, with signed root prerequisite installation, exact final-config authorization,
preflight, native cascade, and at least five minutes' verified health before A.
No POD adoption or authority activation is included.

B automatically applied47→50 at20:31:42.885057016Z after signed root prerequisite,
per-host authorization, download and non-mutating snapshot preflight. Live verifier
passes all six container health checks, exact ingress endpoints, unchanged protected
bootstrap/config receipts, Normal identity, applied recovery journal, and installed
updater1.16.1.33/self-update stable. The root prerequisite did not apply the app;
the running cascade daemon performed delivery/application. B is under observation
before A eligibility changes. See ingress50-B-live.json.

B's apparent stale UI was a cached browser shell. Fresh server HTML and ordinary
browser reload served the correct `index-8wB7i4ln.js`; authenticated Observer
enrollment form and archive health passed without server changes. B exceeded
300 seconds of continuous healthy50 before any A eligibility change.

A automatically applied47→50 at20:38:31.883067716Z after the same signed prerequisite,
per-host authorization and preflight sequence. Live verification passes exact Normal
identity, all six healthy containers, original ingress map, unchanged protected
receipts, applied recovery journal, and installed updater1.16.1.33 on stable.
Final A soak and fleet heartbeat reconciliation are pending at this checkpoint.

Final acceptance: all four hosts exceeded300 seconds of healthy50 by20:43:41 UTC.
All central reports/heartbeats reconcile50/success/alpha/automatic; benchmarkhold
remainsfalse for auto-update. Standalone/unlinked identities preserved. Final
summary and bounded fleet/soak evidence: ../normal-alpha-20260919/README.md.
