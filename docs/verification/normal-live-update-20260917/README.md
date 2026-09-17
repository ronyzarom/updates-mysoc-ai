# Normal live-update preflight — 2026-09-17

User requested a live server update test after local regression tests passed.
Read-only checks used existing SSH over Tailscale; no application was deployed
through SSH and no host/fleet/release configuration was modified.

## Verified target

- testing.siemcore.ai, Tailscale 100.118.106.93, hostname ip-172-26-13-54.
- Updater identity siemcore-testing-01, parent mysoc-testing-mysoc-ai.
- Origin reports online, alpha, auto-update enabled.
- Effective product state 3.3.152.26; static enrollment baseline is older and
  must not be mistaken for actual installed version.
- Installed updater 1.16.1.24, source 7ef793a23712b2392ca18a8daa4c996d8cda7dcc.
- Updater service active; regular accepted heartbeat and up-to-date checks.
- Product channel stable, no self-update override (stable default), no POD
  maintenance/Observer configuration. Host uses standalone recovery runtime1.0.0.4.
- SiemCore independently checked public health/liveness: .26, source8004291,
  PostgreSQL and Redis healthy.

## Blocker to actual live upgrade

Origin latest signed stable Normal candidate is3.3.152.26, already installed.
Newer .29/.30/.32/.33 are pod-app-20260915; .31 is an Observer-only exception.
They are not eligible Normal test targets. Host recovery policy ends with the
approved .23->.26 transition, with no newer transition.

No upgrade, downgrade, reinstallation, policy bypass, fleet assignment change,
release promotion or database copy occurred. Benchmark and POD hosts were untouched.
A compatible signed Normal target and its upgrade/retained-artifact rollback
prerequisites are required to complete the requested actual update test.
SiemCore has been asked to prepare/identify that candidate.

At this initial checkpoint only preflight had passed. The execution results below
supersede that initial status.

## Candidate and live execution

The initial missing-candidate blocker was resolved by SiemCore building3.3.152.36
from clean commit2af0cda57013bec0e8c1d5226e51b504568f9777, direct parent
80042915ee8ac15b8ab8477c86a8b750feb40304. Independent git diff confirms only
VERSION and CHANGELOG differ. Application, schema, apply and rollback source are
unchanged. This is a version-only Normal update qualification candidate, not a
POD-feature release or deployment of the private updater candidate.

- Artifact:64,008,177 bytes, SHA256
  231bf4006d5f96c62843db1c8bbf7cfef2b33684af6fbffe0edccbc18507eb8b.
- Release ID:f2a54c7d-6b89-4443-9a78-bc63d054120b.
- Published only on product channel normal-test-20260917, target group alpha.
  Stable release targeting and all other existing releases were unchanged.
- Origin signature independently verified against testing's existing trust pin.
- Testing alone temporarily selected that product channel in download mode.
  The live updater fetched and verified the artifact through testing.mysoc.ai.
- Existing root runtime1.0.0.4 preflight passed .26->.36; active recovery journal
  unchanged. Exact additive policy revision6->7 adds one transition and retained
  signed target, preserving previous entries/trust/mounts. Policy SHA256
  61ce8758c2d361f29aa9483c6d2defc5b3d7cb26088e3d225d5f5af6ec435167.
- Policy/configuration setup used SSH over Tailscale under testing authorization.
  Application artifact download and apply were performed by the cascade updater,
  not manual SSH software delivery.
- Actual cascade success:2026-09-17T07:04:01.613759658Z, .26->.36, exact digest.
- App and archiver run image
  sha256:bd04a84ea6c2865305080a479db487d3a27be8463477525f5090f81cf4f71bdf.
  Both healthy; PostgreSQL/Redis healthy; migration summary applied0, failed0,
  skipped104. Infrastructure containers were not restarted.
- Retained .26 artifact checksum verified; recovery journal stage applied.
  No forced live rollback was performed, and no database copy/restore occurred.
- Original updater config restored byte-for-byte after success. Product stable,
  updater self-update default stable, alpha, auto-update true. Installed updater
  remains1.16.1.24. Origin confirms running.36 and successful .26->.36 report.
- All nine alpha SiemCore groups/auto-update flags match preflight; benchmark and
  current POD holds remain unchanged. See fleet-preservation.json.

Health observation uses app /health and archiver /health/live. The first monitor
attempt queried unsupported archiver /health (HTTP error); this was a probe-path
mistake, not archiver health failure. Corrected full-duration monitor evidence is
health-soak-correct-endpoints.jsonl. SiemCore independently verified public
/health, /health/live and /login after apply.

These results qualify the existing live Normal cascade application-update path.
They do not prove that the new private updater binary or POD lifecycle is ready.

## Final result

PASS: eleven successful app/archiver samples over five minutes,
2026-09-17T07:05:33Z through07:10:33Z, after restoring stable configuration.
Actual signed cascade update .26->.36 completed successfully on the Normal
testing server. Updater1.16.1.24 itself was not upgraded.
