# Real-product harness preparation

Implemented:
- Required binary SHA256 provenance and per-step verification for product plans.
- Required isolated product reset/ready/verify/stop command interface; unsupported
  modes, unpinned commands and known reference responder cannot qualify as product.
- Real coordinator driver and suite now retain drain journal and archive evidence.
- Exact wire/action/fixture contract in scripts/qualification/pod-maintenance/
  PRODUCT-FIXTURE-CONTRACT.md, with separate drain and full maintenance interfaces.
- Drain resume response rejects node evidence later than response observation,
  matching the SiemCore protected transport contract.

Passed: four Python fixture validator tests; Python compile; Go race coordinator
and executable qualification tests; separate real-process negative response test;
git diff --check. Source-only local driver built at /tmp/updates-p1-joint-coordinator
SHA256 18c96a75f657e54f00342f543bf3475fba2342570b4b5147b2171f151232cc7a.

Evidence tier: component testing of harness/coordinator. No product fixture has
been run here yet. SiemCore must supply the real executable adapter argv, protected
fixture config, disposable signed artifacts and fixture lifecycle commands. Full
signed product apply/recovery remains unqualified. suite_passed cannot imply
readiness or deployment approval; deployment_qualified remains false.

No live host changes, release, publication, enablement, DB copies/backups/restores.
Private .25 unchanged. Normal standalone remains on its existing updater path.
