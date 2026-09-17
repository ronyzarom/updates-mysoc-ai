# Standalone SiemCore isolation regression

Scope: fixture-based automatic updater cycles, both legacy unspecified standalone
configuration and explicit `server_type: normal`. No deployment or live-host test.

Eight cycle cases cover successful signed application, invalid signature, corrupt
artifact checksum, and failed application health for each configuration type.
Assertions cover ordinary executor apply/health/rollback, unchanged failed version,
truthful reports, heartbeat, updater self-check, and retained version after restart.
The explicit normal type survives restart when subsequently omitted from config.

Pod journals in an unconfigured directory are deliberately invalid and remain
untouched. No pod coordinator, discovery, drain, or recovery request is issued.
A pod-scoped capability requirement on a shared release does not gate an explicit
normal host. Conflicting normal plus pod-executor configuration is rejected before
adapter execution. No pod credentials or availability are required for standalone.

Commands:
- `go test ./pkg/updatersim -run 'TestStandaloneAutomatic|TestNormalRejects' -count=1`
- `go test -race ./pkg/updatersim -count=1 -timeout 180s`

Evidence does not replace deployment qualification. No binaries, release targets,
fleet assignments, capabilities, or host installations changed.
