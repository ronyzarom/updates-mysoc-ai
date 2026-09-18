# Withdrawn updater 1.16.1.29 packaging incident

The first binary was incorrectly built from `cmd/mysoc-updater` rather than
`cmd/updater-simulator`, the cascade service entrypoint. Signature and version
checks passed, but the management CLI does not support `relay`.

Published stable/alpha at13:13:24 UTC; MySoc testing relay accepted it and began
restarting with `unknown command relay`. Updates withdrew the release through
DELETE API and restored the existing `.previous` updater1.16.1.24 pointer.
The service restarted at13:16:10 UTC, its existing watchdog reconciled the failed
self-update, and heartbeat was accepted at13:16:10.963 UTC. Product MySoc remained
1.3.19.2. No product installation or database action occurred.

A stayed on updater1.16.1.28 and SiemCore3.3.152.40; Normal stayed on updater1.16.1.24,
Observer1.16.1.27; both services were active. Relay downtime temporarily interrupted
child heartbeat transport. Benchmark remained held/stopped. SiemCore.41 was not
published or applied during this incident.

Corrected unique version1.16.1.30 comes from clean commita04556f and the cascade
entrypoint. Validation now requires exact `updater-simulator <version>` identity
and successful run/relay help including --config before future activation.
Regression tests reject management CLI, version-only binaries and substring
version matches. Native AMD64 command and self-update tests passed before its
publication. Version.29 must never be republished or reoffered.
