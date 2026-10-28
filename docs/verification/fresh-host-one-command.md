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

## Second run — reset host + explicit break/fix (same host, reused)

The me-west1 4-external-IP quota (all in use) blocks a dedicated second public VM,
so this was run by **resetting `drtest` to a fresh-Ubuntu-like state** (purged
mysoc user/`/opt`/updater/PostgreSQL/nginx; kept `/etc/letsencrypt`) and re-running
the identical one-command. `instance_id` = the domain (`drtest.mysoc.ai`), so the
re-enrol reused the same instance and stayed on `alpha` (no DB change).

Break → fix → resume (`19:55`–`20:02` UTC):

| Step | Observation |
|---|---|
| hold port 80 (`python3 -m http.server 80`) + run one-command | `first_install` ran; **preflight reported OK** (see finding below), apt installed pg16/nginx, then failed at the known `useradd rc=12` bug → **no churn**, staged release cleaned, classified back-off `5m` |
| free port 80, clear back-off, retry | `useradd` **skipped** (user existed); role `mysoc_app` + DB `mysoc_production` created, `.env` written, LE cert reused, nginx + units scaffolded, migrations from baseline |
| resume to healthy | `/health` healthy `1.3.49.3`, `dr.role=primary`; `needs_setup:true`; `/setup`→`200`; valid LE cert (`notAfter Jan 26 2027`) |
| one-command wrapper | returned **`DONE  https://drtest.mysoc.ai  version=1.3.49.3`** (exit 0) — it absorbed the failure/back-off/resume transparently within its own wait loop |

This confirms the **identical command** brings up a host (normal server,
`dr.role=primary`); DR would differ only at the dashboard role step.

### Finding — port 80 is not a preflight-fatal condition (report to mysoc)

With port 80 **held**, `1.3.49.3` preflight still reported `preflight OK`; the apt
step installed nginx which merely logged "Not attempting to start NGINX, port 80
is already in use," and the run proceeded until it hit the `useradd` bug. The
entrypoint contract classifies port conflicts under exit `13` (DNS/ports) and the
preflight is meant to catch them read-only **before** mutating the host. Here the
port conflict was not classified and mutation (apt installs) happened first. Two
items for mysoc:
- Make preflight treat a held required port (80/443) as a classified `13` refusal
  **before** any apt/package mutation.
- Fix `useradd --create-home --home-dir /home/mysoc` (`rc=12`) so a single-shot
  `first_install` succeeds without needing the self-healing second pass.

(One intermediate failure — `signal: killed` during migration — was operator
interference: a `systemctl restart` issued while an install was in-flight. Not a
product issue.)
