# Self-Service Installation — New SiemCore Server

A customer installs the cascade updater on a fresh siemcore server entirely by
themselves. Nothing is pre-provisioned on the update infrastructure: the node
registers itself through the operator's relay on its first heartbeat.

Time required: about 10 minutes.

## 1. What your operator gives you

| Item | Example |
| ---- | ------- |
| Updater kit (zip, tailored to your fleet) | `siemcore-updater-kit-1.9.0.1.zip` |
| Relay address (the operator's mysoc node) | `https://relay.operator.example:8443` |
| Relay certificate to pin | `mysoc-relay-ca.pem` (skip if the relay serves a public certificate) |
| Enrollment credential | any stable secret string agreed with the operator |
| Release-signing public key (hex) | pre-filled in tailored kits |

## 2. Prerequisites

- Linux server (amd64 or arm64) with systemd, root access.
- Outbound HTTPS reachability to the relay address above. **No internet
  access is required** — this node never contacts updates.mysoc.ai.
- If SWF agents will report to this server: an inbound port for them
  (default `:18443`, LAN only).

## 3. Install and configure (one command)

```bash
unzip siemcore-updater-kit-*.zip && cd siemcore-updater-kit-*
sudo ./install.sh --update \
  --license-key <credential> --parent-url <relay address> \
  --instance-id siemcore-<customer>-01 --parent-id <operator's mysoc id> \
  --customer-id <customer> --customer-name "<Customer Name>" \
  --signing-key <hex from operator> --current-version <installed version> \
  --ca-file ./mysoc-relay-ca.pem     # omit if the relay uses a public cert
```

The installer creates the service user, binary, and systemd unit, and renders
the configuration from the flags — any missing value is prompted for
interactively. (Prefer editing by hand? Run `sudo ./install.sh` with no flags
and fill the placeholders it prints in
`/etc/siemcore-cascade-updater/config.yaml`.)

## 4. Start and verify

```bash
sudo systemctl start siemcore-cascade-updater
journalctl -u siemcore-cascade-updater -f
```

Within one minute you should see `heartbeat accepted` and
`product is up to date`. Done — your operator now sees this node on their
dashboard, reported through the relay.

## 5. What happens next (defaults are deliberately safe)

- Your node enrolls with **auto-update OFF** and in the `stable` update
  group. Nothing installs until your operator enables it — ask them to flip
  auto-update on when you're ready.
- Every artifact is independently verified on this host (SHA-256 + ed25519
  against the pinned signing key). A compromised relay cannot feed this node
  a tampered release.
- The updater keeps **itself** updated automatically through the relay; the
  siemcore application is only updated once real installs are enabled for
  your host (executor configuration — coordinate with the siemcore team).

## Troubleshooting

| Symptom | Cause / fix |
| ------- | ----------- |
| `certificate signed by unknown authority` | `server.ca_file` missing or wrong — re-copy the relay's `cert.pem`. |
| `connection refused` to the relay | Relay port not reachable from your network — check firewall/routing with the operator. |
| Node re-enrollment rejected after reinstall | The relay holds your previous enrollment — ask the operator to clear it, then restart the updater. |

The same pattern applies to SWF agents enrolling under this server's relay
port — see `docs/SWF-WINDOWS-UPDATER-GUIDE.md`.
