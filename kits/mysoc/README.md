# MySoc Platform Updater — Production Kit (@VERSION@)

The cascade tier-1 updater. It is the **only** component that talks to
`updates.mysoc.ai`: it authenticates with the operator's single platform key,
keeps the mysoc platform updated, and acts as a relay that serves updates and
aggregates telemetry for every siemcore server (and their swf agents) in the
operator's fleet.

```
updates.mysoc.ai  ◀── heartbeat + rollup ── THIS NODE (mysoc-updater, relay :18443)
                                                 ▲
                                    siemcore updaters heartbeat here
```

## Kit contents

| Path | Purpose |
| ---- | ------- |
| `bin/mysoc-updater-linux-{amd64,arm64}` | Static updater binary @VERSION@ (relay-capable). |
| `config.yaml` | Production configuration template. |
| `mysoc-updater.service` | Hardened systemd unit. |
| `install.sh` | Installs binary, user, config, and unit. |
| `docs/RELAY-DEPLOYMENT.md` | Full cascade deployment guide. |
| `docs/UPDATER-GUIDELINES.md` | Operating model and safety requirements. |

## Install

One command (flag-driven; missing values are prompted for on a terminal):

```bash
sudo ./install.sh --update \
  --license-key <platform-key> --instance-id mysoc-<operator> \
  # --current-version auto-detected from /opt/mysoc/VERSION or :8080/health
sudo systemctl start mysoc-updater
```

Use `--clean` instead of `--update` on a host with no app installed yet
(enrolls at 0.0.0; the installer prints the fresh-install checklist).

Or the manual flow:

```bash
sudo ./install.sh
sudo vi /etc/mysoc-updater/config.yaml   # fill the placeholders the installer prints
sudo systemctl start mysoc-updater
journalctl -u mysoc-updater -f
```

Required config values (tailored kits pre-fill some — the installer prints
exactly what remains):

1. `server.license_key` — the operator's platform key from the dashboard
   **Operators** page (shown once at creation/rotation).
2. `instance.id` — stable unique id for this node (e.g. `mysoc-<operator>`).
3. `signing.public_key` — pin it once:
   `curl -s https://updates.mysoc.ai/api/v1/signing-key`
4. `products[0].current_version` — the platform version currently installed
   (read it from the host's `VERSION`; do not leave `0.0.0`).

## Enabling real installs

Pass `--executor filesystem` to `install.sh` and it wires everything needed for
real versioned installs (atomic activation, automatic rollback, and fresh-host
`first_install`): it enables the `executor: filesystem` block in `config.yaml`,
installs the root apply wrapper (`/usr/local/sbin/mysoc-apply-update`, bundled in
this kit), writes the `NOPASSWD` sudoers entry and the unit drop-in
(`NoNewPrivileges=no`, `ProtectSystem=no`), and creates `/opt/mysoc-cascade`.

```bash
sudo ./install.sh --clean --executor filesystem \
  --license-key <platform-key> --instance-id mysoc-<operator>
```

(The one-command bootstrap `deploy/bootstrap/mysoc-enroll.sh` runs exactly this
for you — see "Fresh host enrolment" below.)

## Fresh host enrolment (no app installed yet)

### One command (recommended — self-provisions the whole host)

A brand-new host enrols at `current_version 0.0.0` and provisions itself through
the cascade `first_install` phase (deps, service user, DB + role, `.env`, TLS,
units, nginx, baseline DB, code, health gate — owned by the artifact). From a
bare VM:

```bash
curl -fsSL https://updates.mysoc.ai/install/mysoc-enroll.sh | sudo bash -s -- \
  --license-key <platform-key> --domain <fqdn> --tls wildcard \
  --cert /path/fullchain.pem --key /path/privkey.pem
```

The bootstrap fetches and checksum-verifies this kit, writes the install seed to
`/etc/mysoc/install-seed.env` (0600), runs `install.sh --clean --executor
filesystem`, starts the updater, and waits until `/health` is green — then prints
`DONE https://<fqdn> version=<x>`. It is the **same command for a normal server
and for DR**; DR differs only in the ring/role chosen at the dashboard (step 3).
See [`docs/FRESH-HOST-ONE-COMMAND.md`](../../docs/FRESH-HOST-ONE-COMMAND.md).

This installs the current **stable** ring head, which is what a production
normal/DR server wants. If the host must land on a non-stable ring (e.g. the
alpha DR lab), enrol observe-first (below) so you can set the ring before any
install runs.

### Observe-first (advanced — control the ring before installing)

The first heartbeat creates the instance server-side in `update_group=stable`
with auto-update ON, so a fresh node left in `real` mode installs the highest
*stable* release before you can move it. To pin a different ring first:

1. Install with the executor wired but `simulation.mode: observe` (heartbeat and
   policy check only — no download, no apply, no updater self-update).
2. Confirm the instance appears on updates.mysoc.ai, then set its ring and
   auto-update server-side (admin API key or admin JWT):

   ```bash
   curl -fsS -X PUT https://updates.mysoc.ai/api/v1/instances/<id>/update-group \
     -H "X-API-Key: $ADMIN_API_KEY" -H 'Content-Type: application/json' \
     -d '{"group":"alpha"}'
   curl -fsS -X PUT https://updates.mysoc.ai/api/v1/instances/<id>/auto-update \
     -H "X-API-Key: $ADMIN_API_KEY" -H 'Content-Type: application/json' \
     -d '{"enabled":true}'
   ```

3. Set `simulation.mode: real` and `systemctl restart mysoc-updater`. The next
   cycle takes the highest artifact for that ring through the executor.

Before step 3 the host must already have its app units (`mysoc-backend`,
`mysoc-frontend`), DB and `/opt/mysoc/backend/.env` in place — the updater
swaps binaries and restarts units; it does not provision them. The DR-lab
hosts `testing-dr-a` / `testing-dr-b` are enrolled exactly this way; their
tailored configs and one-shot wiring script live in
[`deploy/testing-dr/`](../../deploy/testing-dr/) and
[`deploy/enroll-testing-dr.sh`](../../deploy/enroll-testing-dr.sh).

## Self-update

The updater keeps itself updated (on by default). `install.sh` places the
binary in a versioned layout under `/var/lib/mysoc-updater/self-update/` and
runs it through symlinks; when the updates server publishes a release for the
product `updater-<os>-<arch>`, the updater verifies its signature, stages and
validates the new binary, atomically retargets the `current` symlink, and
exits — systemd relaunches it as the new version, which confirms the handoff
(a watchdog restores the previous binary if the wrong version comes up). This
is independent of `simulation.mode`: the updater manages its own binary even
while product installs stay simulated. Opt out with `self_update: { disabled:
true }`.

## Network

- Outbound HTTPS to `updates.mysoc.ai` (the only internet dependency).
- Inbound from siemcore servers on the relay port (`:18443` — the ONE relay
  port across the whole cascade). The listener protects its own port
  (1.10.0+): unknown sources are restricted to the enrollment path and
  strictly rate-limited, repeated auth failures earn escalating temp-bans,
  and authenticated children are auto-learned. It can be opened broadly;
  children never configure firewalls. The relay listener is TLS-only
  (1.9.0+): it self-provisions a certificate under `relay.tls.dir` on first
  start. Distribute that directory's `cert.pem` to each siemcore operator,
  who pins it via `server.ca_file`. List every address children use to
  reach this host in `relay.tls.hosts`.

## Security properties

- Every artifact is verified twice before it can reach a child: SHA-256
  checksum and the ed25519 release signature minted by updates.mysoc.ai.
  With `signing.require: true` an unsigned or tampered release is rejected.
- Children are bound to a `relay_token` issued at first contact; a different
  machine presenting a known instance id without its token is rejected.
- Rotating or deactivating the operator key on the dashboard cuts this node
  (and therefore the whole cascade) off within one heartbeat interval.
