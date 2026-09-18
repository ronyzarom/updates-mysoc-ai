# Normal clean native fixture result

Kit1.16.1.33-r1, updater1.16.1.33, unmodified common SiemCore3.3.152.44,
artifact SHA d64f270af60b756cf0062cef897c56d5fecfecff25c1a83323453655d5d8d22f.
Actual Linux amd64 systemd/nested Docker. Synthetic identities and credentials;
no host mounts/socket, published ports, customer endpoints or application data.

Passed: real kit installation, normal persisted installation type, relay heartbeat,
signed artifact download/checksum, protected product hook invocation, exact-input
installer retry, changed-input refusal, and no false application success/version.

Failed product bootstrap:
- First clean fixture: unprovided `shasum` dependency in DB role installer.
- Second fresh fixture with that utility in test infrastructure: attempted Patroni
  build and public postgres base-image pull, stopped by network isolation.

The second failure is **not** a verified prerequisite guard. SiemCore has the
exact evidence and owns the preloaded-dependency admission fix in a new artifact.
Immutable .44 remains unchanged and is not published for Normal installation.

Schema1 baseline only; schema3 GCS/native full UI/ingestion tests remain pending
an installable product candidate. The host cleanup record confirms the new real
A host remains empty. No customer credentials appear in these sanitized reports.
