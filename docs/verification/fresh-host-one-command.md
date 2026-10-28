# Verification: one-command fresh host install (empty DB → healthy)

Live end-to-end validation of the one-command fresh install on a genuinely empty
VM, through the real published path. 2026-10-28.

## Host

| | |
|---|---|
| VM | `mysoc-fresh-test` (GCP `mysoc-dr-lab`, me-west1-a, e2-standard-2, 30 GB) |
| OS | Ubuntu 24.04.5 LTS, amd64 — no PostgreSQL, no `mysoc` user, no `/opt/mysoc` at start |
| FQDN | `drtest.mysoc.ai` → `34.165.183.239` (GoDaddy A record) |
| TLS | `--tls letsencrypt --email ops@mysoc.ai` |
| Ring | instance auto-created `update_group=stable`; moved to `alpha` for testing |

## Command (exactly what an admin runs)

```bash
curl -fsSL https://updates.mysoc.ai/install/mysoc-enroll.sh | sudo bash -s -- \
  --license-key <KEY> --domain drtest.mysoc.ai \
  --tls letsencrypt --email ops@mysoc.ai --admin-email ops@mysoc.ai
```

## Result: PASS

- `https://drtest.mysoc.ai/health` → `{"dr":{"role":"primary"},"environment":"production","service":"mysoc-ai-backend","status":"healthy","version":"1.3.49.3"}`
- Public HTTPS `200` with a valid **Let's Encrypt** cert (`CN=drtest.mysoc.ai`, verify ok).
- `GET /api/v1/auth/setup-status` → `{"needs_setup":true}`; `GET /setup` → `200`.
- Listeners: `80`, `443`, `8080` (backend), `3000` (frontend).
- First administrator created at `/setup` and signed in (confirmed by Rony).

## Versions

| Component | Version |
|---|---|
| Bootstrap + kit (ours) | `1.17.0.2` (commit `4b19ca4`), published to `/install/` |
| Artifact that provisioned the host | mysoc `1.3.49.3` (alpha) |

## Timeline / timings (UTC)

| Phase | Detail |
|---|---|
| enroll steps 1–4 | rsync/curl → checksum-verified kit fetch → seed `0600` → `install.sh --clean --executor filesystem` (seconds) |
| `first_install` attempt (1.3.49.3) | `11:58:04` preflight **OK** → apt `postgresql-16`/`nginx`/`certbot` + Node 20 → failed `11:59:11` (`useradd rc=12`), backed off |
| `first_install` resume | `12:00:30` → baseline migrate (`cmd/migrate -baseline`, app-role `mysoc_app`) + certbot + units + health gate → **healthy ~12:03:00** (~2.5 min) |

## Refuse → back-off → resume (no churn) — proven 3×

| Target | Outcome | Updater action |
|---|---|---|
| `1.3.36.6` (stale `stable` head, no `first_install`) | shim: `unknown phase: first_install` | no rollback, back-off `5m`→`15m` |
| `1.3.49.2` | preflight bug (silent `exit 1`) | no rollback, back-off, `CleanupStaged` |
| `1.3.49.3` | `useradd rc=12` (classified ERR trap) | no rollback, back-off `1h` → **resumed next cycle → healthy** |

Each failure cleaned up the staged release and backed off; none left a
partially-mutated host in a rollback loop. The resume path brought the host to
healthy with no manual reinstall.

## Bugs found and dispositions

1. **Ours — FIXED (`1.17.0.2`):** `/etc/mysoc-updater` created `0700 root:root`
   (leaked `umask` from the seed write) → service user could not traverse to read
   its config → crash loop. Fix: explicit `install -d -m 0750 root:<name>` for the
   config dir + scoped `umask` in `mysoc-enroll.sh`.
2. **Ring gap — FIXED (Rony):** `1.3.49.2` was not on the default `stable` ring, so
   fresh hosts pulled `1.3.36.6`. Promoted `1.3.49.2`/`1.3.49.3` to include `stable`.
3. **mysoc preflight — FIXED (`1.3.49.3`):** `set -euo pipefail` + a `grep` on a
   free port 80 (no match → exit 1) aborted the preflight with a bare `exit 1`
   whenever port 80 was free. Fixed with `|| true` at 7 sites + a classified ERR
   trap (new `PF_INTERNAL=17`).
4. **mysoc `useradd` — REPORTED (self-heals on resume):** `useradd --create-home
   --home-dir /home/mysoc` failed (`rc=12`); the user was created but `/home/mysoc`
   was not. A single-shot install fails once then resumes to success. Flagged to
   mysoc to make first-pass clean.

## Not run (optional)

- Standalone intentional break/fix on a dedicated second VM (hold port 80 →
  classified `13` → free → resume): blocked by the me-west1 4-external-IP quota
  (all in use). Equivalent refuse/back-off/resume behavior is proven above on real
  bytes. "Same command for normal vs DR" is evidenced by `drtest` coming up as a
  normal server (`dr.role=primary`); DR differs only at the dashboard role step.
