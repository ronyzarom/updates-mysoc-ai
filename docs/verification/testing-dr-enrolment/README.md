# DR-lab enrolment — testing-dr-a / testing-dr-b (alpha ring)

Enrol the two new MySoc DR-lab hosts in GCP project `mysoc-dr-lab` (me-west1)
into updates.mysoc.ai, update_group **alpha** (same ring as testing.mysoc.ai),
so the cascade installs the same alpha mysoc artifact on both. Coordinated with
the mysoc-ai project (host prerequisites + artifact entrypoint) over the hub.

| Host | Zone | Internal / public | Role | instance.id |
|---|---|---|---|---|
| testing-dr-a | me-west1-a | 10.208.0.2 / 34.165.207.15 | MySoc primary (nominal), `testing-dr.mysoc.ai` | `mysoc-testing-dr-a` |
| testing-dr-b | me-west1-b | 10.208.0.3 / 34.165.254.130 | MySoc streaming standby (read-only) | `mysoc-testing-dr-b` |

- Operator: `testing-dr-mysoc-ai` (one platform key for both hosts; stored in
  `keys/testing-dr-mysoc-ai-key.txt`, gitignored, never on the hub).
- Signing key pinned: ed25519 `1f1aa11a80d6ac549a26bb25daac4798c42dd469138680e68f89832bd32e7f57`
  (key_id `bbc4c0dbdcedc37e`).
- Kit: `mysoc-updater-kit-1.16.0.1`; configs `deploy/testing-dr/config-a.yaml`
  and `config-b.yaml`; wiring `deploy/enroll-testing-dr.sh`; root wrapper
  `deploy/mysoc-apply-update.sh` (unchanged).

## Host inventory (read-only, 2026-10-26)

Both: Ubuntu 24.04.5 x86_64, PostgreSQL 16.15, service user `mysoc` (uid 999,
nologin, home /opt/mysoc), `rsync curl jq psql pgbackrest` present, egress to
updates.mysoc.ai returns 200. `/opt/mysoc/{backend,frontend,migrations}` exist
(empty but for `backend/.env`); DR tooling (`/opt/mysoc/dr`) and DR units are
already installed. `-a`: `pg_is_in_recovery()=f`, nginx TLS for
`testing-dr.mysoc.ai`. `-b`: `pg_is_in_recovery()=t`, nginx `server_name _`.

Blocker handed to mysoc-ai before switch-to-real (hub reply
`20261026T131301Z-39b4b8`):
- A. No `mysoc-backend.service` / `mysoc-frontend.service` on either host.
- B. Entrypoint fresh-host bug: `deploy/updater/apply` falls back to owner
  `bitnami` when no backend binary exists yet; no `bitnami` user here.
- C. Confirm alpha `1.3.42.2` ships `updater/apply` (recovery skip) +
  `updater/mysoc-migrate` + `dr/`, and the standby runs `mysoc-frontend`.
- D. Relay port for `testing-dr-siemcore` and cross-failover relay trust.

## Enrolment procedure (per host)

Stage on the host (via `gcloud compute scp --project mysoc-dr-lab`):
`dist/updater-kits/mysoc-updater-kit-1.16.0.1` -> `/tmp/mysoc-updater-kit`,
`deploy/testing-dr/config-<a|b>.yaml` -> `/tmp/testing-dr-config.yaml` (with the
platform key filled), `deploy/mysoc-apply-update.sh` -> `/tmp/mysoc-apply-update.sh`.

```bash
sudo bash /tmp/enroll-testing-dr.sh /tmp/testing-dr-config.yaml   # installs updater in OBSERVE
# server side (admin API key), once the instance appears:
curl -fsS -X PUT https://updates.mysoc.ai/api/v1/instances/<id>/update-group \
  -H "X-API-Key: $ADMIN_API_KEY" -H 'Content-Type: application/json' -d '{"group":"alpha"}'
curl -fsS -X PUT https://updates.mysoc.ai/api/v1/instances/<id>/auto-update \
  -H "X-API-Key: $ADMIN_API_KEY" -H 'Content-Type: application/json' -d '{"enabled":true}'
# host: flip to real and restart
sudo sed -i 's/^  mode: observe/  mode: real/' /etc/mysoc-updater/config.yaml
sudo systemctl restart mysoc-updater
```

## Evidence

_(filled during execution)_

- Operator created:
- testing-dr-a observe heartbeat / instance id / group set:
- testing-dr-a real apply (version, dr.role, timings):
- testing-dr-b observe heartbeat / instance id / group set:
- testing-dr-b real apply (migrations skipped, dr.role=standby):
