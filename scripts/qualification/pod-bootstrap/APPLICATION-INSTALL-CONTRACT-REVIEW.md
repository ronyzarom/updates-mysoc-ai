# Application install contract review — 2026-09-17

Scope: product-owned `pod-application-install-v1`, reviewed from SiemCore's
`deploy/cluster/updater/pod_application_install.py` and matching contract.
This review does not enable execution, kit delivery, or cloud changes.

The API fits the existing bounded host-worker boundary: original operation,
generation, input/artifact digest, fresh Observer authorization, independently
verified extracted bundle, and a partial receipt followed by independent paused
management observation. Normal installation must remain outside this POD path.
Explicit port reset, fixed credentials, exact environment equality and retained
retry identity address the earlier review points.

Independent product unit run: four tests passed. These exercise helpers, not the
full installer. The existing ten-stage native fixture qualifies observation only.

Remaining data-evidence finding: `verify_data` trusts protected `compose.json` as
its image/mount plan without verifying its bytes against the prior authenticated
runtime plan. Reproduction using the product test fixture: change both planned
PostgreSQL image and inspected Config.Image while leaving the original journal
and data_configuration_sha256 unchanged; verification succeeds. Expected compose
must be derived from authenticated prior inputs or its exact digest must be
captured at the verified runtime stage and checked before parsing here.

An additional unplanned bind mount also passes; require the exact expected mount
set for the controlled data containers. Add negative coverage for both cases.
The matching immutable container and network IDs remain required.

Before execution integration: resolve this evidence binding, run the actual
signed installer fixture with interruption/retry, independently observe both
paused runtimes, and preserve all incomplete/processing-disabled flags. Full
bundle verification must cover every invoked shell/Python helper and template;
verifying only the selected module is insufficient.
