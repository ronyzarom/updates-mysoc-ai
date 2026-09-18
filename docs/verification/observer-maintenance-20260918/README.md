# Observer maintenance candidate 1.16.1.27

Extends the earlier default-off executor checkpoint with root component delivery
and accepted-operation chaining. Original 1.16.1.26-r1 kit remains immutable.

Fixture checks passed:
- Root maintenance success and forced installed-readiness failure/rollback retain
  the product PID and original protected bootstrap/configuration. Updater restarts.
- Installed CLI applies two consecutive signed synthetic services (.37 -> .38 -> .39),
  with exact accepted predecessor binding and preserved original bootstrap inputs.
- Cross-compiled Go updater test runs as the real unprivileged service account;
  actual sudo allowlist, root CLI, signed staging, exact SiemCore worker and real
  systemd execute apply/status successfully. No injected Go adapter transport.

These are isolated Linux arm64 systemd containers, no runtime network or host
mounts. Updater installer version fixture uses a synthetic CLI binary; the Go wire
fixture is an actual compiled Go test executable. Product executable is synthetic.
The test image adds sudo to the existing local systemd fixture image during test
infrastructure provisioning; installed code has no package/download fallback.

Live qualification and signed package identities are recorded in LIVE-RESULT.md,
live-receipt.json, auth.json and kit-manifest.json. The source-only checkpoint
in ../observer-security-update-20260918/ is historical; this qualification
supersedes its first-hop-only and not-yet-deployed limitations.
The maintenance kit permits only the known .26 binary/bootstrap hook predecessor;
never run on Normal or linked POD hosts. Original bootstrap pins are not changed.
