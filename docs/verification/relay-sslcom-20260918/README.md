# Testing MySoc relay SSL.com migration — 2026-09-18

Authorized scope: updater trust/TLS only; preserve product processes, versions,
fleet assignments and explicit holds. Endpoint testing.mysoc.ai:18443.

Old relay self-signed CA SHA256:
4ee3ec7c63af477fe91ad33967baeb9d38a6eac6263fde0dca1c3d49eddc2d46.
Validated local SSL.com wildcard chain ~/certs/mysoc/fullchain.pem, SAN*.mysoc.ai
and mysoc.ai, matching root-private key, expiry2027-01-13. Chain SHA256:
648d0f37a9eac7dd102bd94114e30aaa95e73a7c4f37cc9741d86e43ae955087.

Observer and Normal testing explicit CA pools replace public system roots.
Added the verified SSL.com public chain alongside the old certificate, then
restarted only their updater services. Prospective bundle verified the old
listener before replacing files; application processes/containers and product
versions remained unchanged. Dual-trust bundle SHA256:
c88f8822233e75ae5829be7e8c21d3c7d8b6ec059096148405759a5d55c5bfc5.
Root-private original CA backups/receipts are under
/var/lib/updates-tls-migration-20260918 on each affected child.

Parent configured explicit cert/key under /etc/mysoc-updater/tls-sslcom,
root:mysoc-updater0640, directory0750. OpenSSL verified system trust, hostname,
SSL.com issuer and key match before changing configuration. Restarted only
mysoc-updater; application process IDs and MySoc1.3.19.2 unchanged.
Original config retained root0600 at
/var/lib/updates-tls-migration-20260918/mysoc-config.original.yaml.
An initial preflight assumed Docker was installed on MySoc and stopped before
any config/certificate change; corrected baseline checks use native process IDs.
No self-signed listener fallback is configured after migration. Old trust anchor
retention on clients is transitional trust only, not a self-signed server.

## Mandatory benchmark resume prerequisite

GCP bench-siemcore-ai / me-west1-b / immutableVM2021894211683199098 was TERMINATED.
It was NOT started or changed. Its origin instance siemcore-bench-20260912-01 has
a stale Sept15 heartbeat; the UI online label is not proof it is running.
Before any future authorized resume, refresh its parent trust for the SSL.com
relay and verify TLS. Keep its existing alpha/automatic=false hold intact.
Do not start it merely to finish this TLS migration.

Old bezeq-pod-test-a-6560860442296916583 and
bezeq-pod-test-witness-8675763460526905958 registrations are stale/offline; do not
re-enroll or restart retired VMs as part of migration. Any future restoration
requires identity reconciliation and updated TLS trust first.

## Post-migration checks

Native TLS verification using each host's effective trust passed from Normal,
Observer and new node A. Public system trust also passed externally. Fresh
origin cascade reports after the parent restart confirm Observer/Normal online,
both SiemCore3.3.152.39, parentMySoc1.3.19.2 and unchanged alpha/automatic settings.
Benchmark remains alpha/automatic=false. See cascade-after.json and per-host
nonsecret receipts. New node A uses public system trust; no old private CA copied.
