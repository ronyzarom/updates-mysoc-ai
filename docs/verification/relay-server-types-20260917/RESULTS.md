# Relay host-type compatibility

Passed `go test -race ./pkg/updatersim -run 'TestRelay.*ServerType|TestRelayArtifactDeliveryAllServerTypes' -count=1`.

Matrix: legacy/default, normal, pod-active, pod-stby, pod-observer.
Two-hop artifact delivery: paired and independent variants, bootstrap/update cache
separation, existing signature verification and corrupt-cache recovery.
Child forwarding: child role/identity preserved, heartbeat rollup available.

Existing relay behavior needs no production-code change: role-specific local
application execution is separate from child relay delivery. Relay remains
explicitly enabled. Component HTTP fixtures only; no live shared-endpoint failover
qualification, deployment, or application traffic/authority change.
