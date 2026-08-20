# MySoc Platform Updater — Production Kit (@VERSION@)

The cascade tier-1 updater. It is the **only** component that talks to
`updates.mysoc.ai`: it authenticates with the operator's single platform key,
keeps the mysoc platform updated, and acts as a relay that serves updates and
aggregates telemetry for every siemcore server (and their swf agents) in the
operator's fleet.

```
updates.mysoc.ai  ◀── heartbeat + rollup ── THIS NODE (mysoc-updater, relay :8443)
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

The default executor is a no-op (the updater downloads, verifies, and reports
but does not touch the host). To perform real versioned installs with atomic
activation and automatic rollback, uncomment the `executor: filesystem` block
in `config.yaml` and set the restart/health commands for this host. When you
do, also add the install root to the systemd sandbox:

```
# /etc/systemd/system/mysoc-updater.service
ReadWritePaths=/var/lib/mysoc-updater /opt/mysoc
```

## Network

- Outbound HTTPS to `updates.mysoc.ai` (the only internet dependency).
- Inbound from siemcore servers on the relay port (default `:8443`). Put a
  TLS-terminating proxy in front of it or restrict it to the management
  network; the relay itself speaks plain HTTP.

## Security properties

- Every artifact is verified twice before it can reach a child: SHA-256
  checksum and the ed25519 release signature minted by updates.mysoc.ai.
  With `signing.require: true` an unsigned or tampered release is rejected.
- Children are bound to a `relay_token` issued at first contact; a different
  machine presenting a known instance id without its token is rejected.
- Rotating or deactivating the operator key on the dashboard cuts this node
  (and therefore the whole cascade) off within one heartbeat interval.
