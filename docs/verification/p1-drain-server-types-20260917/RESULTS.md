# P1 drain coordinator and remembered installation types

Evidence tier: component, with real coordinator/adapter subprocesses and synthetic
observer responses. Not native service or real-host qualification.

Passed:
- `go test -race ./pkg/podmaintenance ./pkg/updatersim ./cmd/pod-maintenance-qualification -parallel 4 -timeout 180s`
- `go test -race ./pkg/podmaintenance -run TestDrainToRecoveryRequiresSeparateGrantAndPausedProof -count=1`
- `git diff --check`

The process cases kill the coordinator on lost status/authorization/resume replies
and at five durable checkpoints, then restart it. Original operation binding and
deadline remain unchanged; generation becomes the authenticated retained value.
Discovery-only performs no mutation. Expired grants and observer refusal preserve
the barrier. Invalid scope, freshness, phase, capture, zero generation and ordinary
v1 fallback are refused. Only paused is drain terminal success, never application
success. V2 requires its own signed authorization and paused proof.

All four requested SiemCore server types are persisted/restored. Pod identities
cannot silently change to normal or another pod/node. Pod data nodes require a
coordinated executor; the observer refuses the data-node executor. Startup drain
reconciliation requires neither an origin connection, a new offer, nor access to
the target artifact. No automatic grant is generated.

Not qualified: real protected adapter integration against native service, live
host lifecycle, observer-specific executor, and next-operation drain-ledger archival.
No publication/deployment/capability enablement. Private 1.16.1.25 unchanged.
