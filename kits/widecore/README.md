# WideCore Cascade Updater — Production Kit (@VERSION@)

The cascade leaf updater for WideCore AI gateways. It never talks to
`updates.mysoc.ai` and relays to nothing: it heartbeats to the operator's
**mysoc relay** (`:18443`) and installs `widecore` releases served from the
relay's signature-verified cache.

```
mysoc relay (:18443) ◀── heartbeat + pull ── THIS NODE (widecore-updater, leaf)
```

## Naming and mode

- Canonical service / account / binary name: **`widecore-updater`**.
- Runs the updater binary in leaf mode (`widecore-updater run --config /etc/widecore-updater/config.yaml`, `relay.enabled: false`).
- The four WideCore host files (`widecore-apply-update`, `sudoers-widecore-updater`, `updater-override.conf`, `updater-executor.yaml`) live canonically in `widecore-ai/deploy/` and are bundled here SHA-256-pinned (`widecore-host-files.sha256`).

## Kit contents

| Path | Purpose |
| ---- | ------- |
| `bin/widecore-updater-linux-{amd64,arm64}` | Static updater binary. |
| `config.yaml` | Production configuration template (`product_tier: widecore`, `relay.enabled: false`). |
| `widecore-updater.service` | Hardened leaf systemd unit (`Type=notify`, `WatchdogSec=180`). |
| `widecore-updater-restore.service` / `-self-restore.sh` | Root-owned self-update restore backstop (invariant 4). |
| `install.sh` | Installs binary, user, config, and unit (`--executor filesystem` wires real installs). |
| `widecore-apply-update`, `sudoers-widecore-updater`, `updater-override.conf`, `updater-executor.yaml` | The four WideCore-owned host files, installed/spliced byte-for-byte by `--executor filesystem`. |
| `widecore-host-files.sha256` | Pins the four host files above; the installer refuses to install any whose bytes drift. |
| `docs/RELAY-DEPLOYMENT.md` | Full cascade deployment guide. |
| `docs/UPDATER-GUIDELINES.md` | Operating model and safety requirements. |
| `docs/UPDATE-ENTRYPOINT-CONTRACT.md` | How the updater calls `apply` / `first_install` / `rollback` / `health`. |

## Install

Flag-driven (missing values are prompted interactively on a terminal):

```bash
sudo ./install.sh --clean --executor filesystem \
  --license-key <credential> --parent-url https://relay.op.example:18443 \
  --instance-id widecore-acme-01 --parent-id mysoc-op-01 \
  --customer-id acme --customer-name "Acme Corp" \
  --signing-key <hex> \
  --update-group alpha \             # optional initial ring (defaults to stable)
  --ca-file ./mysoc-relay-ca.pem     # omit when the relay serves a public cert
sudo systemctl start widecore-updater
```

Use `--update --current-version <ver>` instead of `--clean` when WideCore is
already installed on the host.

For one-command fresh-host provisioning (which also stages `/etc/widecore/install-seed.env`
and copies `TLS_MODE=provided` certs out of `/root` or `/tmp` so systemd's
`ProtectHome=true` / `PrivateTmp=true` never hide them), use
`widecore-enroll.sh --license-key <KEY> --config /root/server.conf`.
