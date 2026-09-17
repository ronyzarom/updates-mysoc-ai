# Installation identity and drain archival fixes

Passed:
- Five Python installation identity/CLI tests: all types, immutable identity,
  role/topology conflicts, A/B identity distinct from primary, old binary rejection,
  capable binary rendering, normal compatibility and unchanged relay configuration.
- Four existing greenfield bootstrap tests.
- `bash -n kits/siemcore/install.sh` and `git diff --check`.
- `go test -race ./pkg/podmaintenance ./pkg/updatersim ./cmd/updater-simulator/cmd ./cmd/pod-maintenance-qualification -parallel 4 -timeout 180s`.

Installer validates identity and bundled binary support before changing host files.
Existing untyped invocations keep the legacy path, without requiring the new probe.
The probe explicitly reports identity support, not qualified lifecycle readiness.
The next-operation transition now archives drain evidence/ledger byte-for-byte,
verifies hashes, and safely retries after interruption before removing the live
sidecar. Existing normal and relay compatibility regression suites pass.

No deployment, package publication, capability enablement or live IP movement.
Private .25 is unchanged. SiemCore confirms observer-specific update lifecycle and
relay shared-endpoint failover adapter are not qualified; they remain dependencies.
The legacy witness etcd/sentinel updater cannot substitute for allocation-observer
lifecycle. Missing qualified pod adapters remain explicit errors, never normal
executor fallback.
