# SiemCore Cascade Updater — Production Kit (@VERSION@)

The cascade tier-2 updater. It never talks to `updates.mysoc.ai`: it
heartbeats to the operator's **mysoc relay**, installs siemcore releases
served from the relay's signature-verified cache, and acts as a relay itself
for the customer's swf agents.

```
mysoc relay  ◀── heartbeat + rollup ── THIS NODE (siemcore-cascade-updater, relay :18443)
                                            ▲
                                 swf updaters heartbeat here
```

## Naming and ports (important on v3 hosts)

- The unit is `siemcore-cascade-updater`, **not** `siemcore-updater` — the
  latter is the retired v2 daemon and v3 posture audits flag it. Never
  `systemctl mask` either name to silence an audit.
- The relay listens on `:18443` by default because v3 siemcore hosts publish
  the app itself on `:8443`; the port binds at updater start, even in
  simulate mode.
- The `executor: filesystem` block in `config.yaml` is a **v3
  compose-contract DRAFT** — the siemcore team must confirm the exact
  deploy/health commands before anyone enables it. Until then this tier is
  simulate-only.

## Kit contents

| Path | Purpose |
| ---- | ------- |
| `bin/siemcore-cascade-updater-linux-{amd64,arm64}` | Static updater binary (relay-capable). |
| `config.yaml` | Production configuration template. |
| `siemcore-cascade-updater.service` | Hardened systemd unit. |
| `install.sh` | Installs binary, user, config, and unit. |
| `docs/RELAY-DEPLOYMENT.md` | Full cascade deployment guide. |
| `docs/UPDATER-GUIDELINES.md` | Operating model and safety requirements. |

## Install

```bash
sudo ./install.sh
sudo vi /etc/siemcore-cascade-updater/config.yaml   # fill the printed placeholders
sudo systemctl start siemcore-cascade-updater
journalctl -u siemcore-cascade-updater -f
```

Required config values (tailored kits pre-fill some — the installer prints
exactly what remains):

1. `server.url` — the mysoc relay's child-facing address
   (the `relay.listen` port on the operator's mysoc node).
2. `server.license_key` — the enrollment credential agreed with the operator
   (bound to a relay_token at first contact).
3. `instance.id`, `instance.parent_id` (the mysoc node's instance id),
   `instance.customer_id` / `customer_name` (the end customer this siemcore
   serves — this is what groups the fleet view on the dashboard).
4. `signing.public_key` — the same pinned key as the whole fleet:
   `curl -s https://updates.mysoc.ai/api/v1/signing-key`
5. `products[0].current_version` — the siemcore version currently installed.

On first heartbeat the mysoc relay issues this node a `relay_token`; it is
persisted in the state file automatically. If the state file is lost, the
relay operator may need to clear the stale enrollment before the node can
re-enroll under the same instance id.

## Enabling real installs

The default executor is a no-op (download, verify, report — no host changes).
The commented `executor: filesystem` block in `config.yaml` is a **v3
compose-contract draft** — confirm the deploy and health commands with the
siemcore team before enabling it, then widen the systemd sandbox:

```
# /etc/systemd/system/siemcore-cascade-updater.service
ReadWritePaths=/var/lib/siemcore-cascade-updater /opt/siemcore
```

## Self-update

The updater keeps itself updated (on by default). `install.sh` places the
binary in a versioned layout under
`/var/lib/siemcore-cascade-updater/self-update/` and runs it through symlinks;
when a release for the product `updater-<os>-<arch>` reaches the mysoc relay,
this updater verifies its signature, stages and validates the new binary,
atomically retargets the `current` symlink, and exits — systemd relaunches it
as the new version, which confirms the handoff (a watchdog restores the
previous binary if the wrong version comes up). This is independent of
`simulation.mode`: the updater manages its own binary even while siemcore
installs stay simulated. Opt out with `self_update: { disabled: true }`.

## Network

- Outbound to the mysoc relay only. No internet access required.
- Inbound from swf agents on the relay port (default `:18443` — chosen to
  avoid the app's own `:8443` on v3 siemcore hosts), typically the
  customer's LAN. Plain HTTP: keep it on a management network or front it
  with TLS.

## Security properties

- The updater independently re-verifies the SHA-256 checksum and the ed25519
  release signature of every artifact — a compromised relay cannot substitute
  a payload.
- swf children are bound to relay tokens issued at first contact.
- Update results (success/failure) flow up the cascade automatically and are
  visible on the operator dashboard within one heartbeat interval per hop.
