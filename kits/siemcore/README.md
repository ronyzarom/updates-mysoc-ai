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
- The relay listens on `:18443` — the ONE relay port across the whole
  cascade (also chosen because v3 siemcore hosts publish the app itself on
  `:8443`); the port binds at updater start, even in simulate mode. The
  listener protects its own port (1.10.0+): unknown sources are restricted
  and rate-limited, repeated auth failures earn escalating temp-bans, and
  authenticated children are auto-learned — it can be opened broadly, and
  children never configure firewalls.
- The `executor: filesystem` block in `config.yaml` is the **confirmed-final
  v3 compose contract** (proven on production-track releases 3.3.132.1 and
  3.3.133.1); canonical copies live in the siemcore repo under
  `deploy/cascade/`.

## Kit contents

| Path | Purpose |
| ---- | ------- |
| `bin/siemcore-cascade-updater-linux-{amd64,arm64}` | Static updater binary (relay-capable). |
| `config.yaml` | Production configuration template. |
| `siemcore-cascade-updater.service` | Hardened systemd unit. |
| `siemcore-cascade-updater-restore.service` / `-self-restore.sh` | Self-update restore backstop (invariant 4). |
| `install.sh` | Installs binary, user, config, and unit (`--executor filesystem` also wires real installs). |
| `siemcore-apply-update.sh`, `siemcore-cascade-updater.sudoers`, `override.conf`, `executor-block.yaml` | The four SiemCore-owned host files, installed/spliced byte-for-byte by `--executor filesystem`. |
| `siemcore-host-files.sha256` | Pins the four host files above; the installer refuses to install any whose bytes drift. |
| `docs/RELAY-DEPLOYMENT.md` | Full cascade deployment guide. |
| `docs/UPDATER-GUIDELINES.md` | Operating model and safety requirements. |
| `docs/UPDATE-ENTRYPOINT-CONTRACT.md` | How the updater calls `apply` / `first_install` / `rollback`. |

## Install

One command (flag-driven; missing values are prompted for on a terminal):

```bash
sudo ./install.sh --update \
  --license-key <credential> --parent-url https://relay.op.example:18443 \
  --instance-id siemcore-acme-01 --parent-id mysoc-op-01 \
  --customer-id acme --customer-name "Acme Corp" \
  --signing-key <hex> --current-version 3.3.133.1 \
  --ca-file ./mysoc-relay-ca.pem     # omit when the relay serves a public cert
sudo systemctl start siemcore-cascade-updater
```

Use `--clean` instead of `--update` on a host with no siemcore installed yet
(enrolls at 0.0.0; the installer prints the fresh-install checklist).

Or the manual flow:

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

## Fresh install in one command

A brand-new SiemCore server is installed the way a fresh MySoc server is: one
line from the updates server enrols the updater at `0.0.0`, and the release
installs itself through its `first_install` phase.

```bash
curl -fsSL https://updates.mysoc.ai/install/siemcore-enroll.sh | sudo bash -s -- \
  --license-key <credential> --config /root/server.conf
```

`server.conf` is a root-only `KEY=value` file (SiemCore's counterpart of
`/etc/mysoc/install-seed.env`). `siemcore-enroll.sh` copies it verbatim to
`/etc/siemcore/install-seed.env` and reads only the enrolment keys from it:

| Key | Meaning |
| --- | ------- |
| `UPDATES_PARENT_URL` | parent MySoc relay child-facing address (`https://…:18443`) |
| `UPDATES_PARENT_ID` | instance id of the parent MySoc node |
| `CUSTOMER_ID`, `CUSTOMER_NAME` | end customer (groups the fleet view) |
| `UPDATES_SIGNING_KEY` | pinned fleet signing key (64 hex); else fetched from the parent (TOFU) or `--signing-key` |
| `UPDATES_RELAY_CA_FILE` | relay `cert.pem` to pin (omit for a publicly trusted relay) |
| `TLS_CERT_FILE`, `TLS_KEY_FILE` | copied to `/etc/ssl/siemcore/` |
| `ARCHIVE_CREDENTIALS_FILE` | GCS archiver credential; copied out of `/root` to `/etc/siemcore/provisioning/` (the unit has `ProtectHome=yes`) |
| `DOMAIN` | the server's address; printed on success |
| `UPDATER_INSTANCE_ID` | optional; else a stable id is generated and persisted to `/etc/siemcore/instance-id` |
| `UPDATE_GROUP` | advisory only — the installer cannot set the ring (see below) |

The instance id is **never derived from the domain** and is persisted, so a
re-run never creates a second identity.

## Enabling real installs

The one-command path above passes `--executor filesystem`, which enables the
executor and installs everything it needs outside the config:

```bash
sudo ./install.sh --clean --executor filesystem \
  --license-key <credential> --parent-url https://relay.op.example:18443 \
  --instance-id siemcore-acme-01 --parent-id mysoc-op-01 \
  --customer-id acme --customer-name "Acme Corp" --signing-key <hex>
```

`--executor filesystem` installs the four SiemCore-owned host files
**byte-for-byte** from the kit (updates-mysoc never hand-writes their contents):

- the apply shim → `/usr/local/sbin/siemcore-apply-update`,
- the sudoers entry → `/etc/sudoers.d/siemcore-cascade-updater` (NOPASSWD only
  for the lifecycle invocations, no `SETENV` in the CURRENT bytes; the
  first_install-capable bytes from release/v3.3.198 add `SETENV` + `env_keep` of
  the contract vars),
- the unit drop-in → `/etc/systemd/system/siemcore-cascade-updater.service.d/override.conf`
  (`NoNewPrivileges=false`, `ProtectSystem=no`, `ReadWritePaths=/opt/siemcore-cascade`),
- the filesystem-executor block → spliced from `executor-block.yaml` into the
  `#BEGIN/#END-FS-EXECUTOR` region of `config.yaml` (`first_install_timeout: 30m`),

plus the `/opt/siemcore-cascade` staging root (the fleet path — `/opt/siemcore`
is the dashboard folder). All four files ship with a pinned SHA-256 in
`siemcore-host-files.sha256`; the installer refuses to install any whose bytes
do not match. These files are SiemCore-owned (canonical copies under the
siemcore repo `deploy/cascade/`); the kit carries the authoritative bytes.

## Re-enrolment (reinstalling an existing server)

Use `--clean` to re-enrol a host that already carries updater state. Unlike a
fresh host, a siemcore node is a relay child: the parent MySoc relay bound its
`instance_id` to a `relay_token` at first contact. `--clean` therefore does
**not** delete the state file; it rewrites it to reset the product version to
`0.0.0` (so `first_install` re-runs) while **preserving** `instance_id`,
`api_key` and `relay_token`, and saves the old state beside it as a
`state.json.bak-<ts>`. Reuse the **same** `--instance-id` so the server row
stays in its ring. This is how the test pods (`bezeq-pod-test-a` / `-b`) keep
their identities — and therefore their **alpha** ring — across a reinstall.

**Ring at enrolment.** A *new* instance id always enrols in `stable` (the
installer cannot set the ring, hence the `UPDATE_GROUP` warning). Reusing an
existing id keeps whatever ring that row already has. Offering a group at
enrolment (so a from-scratch host could land directly in, say, `alpha` with no
`first_install`) is a pending design decision for Rony, not something this kit
does today.

If the `relay_token` is lost (state file deleted, or a from-scratch wipe), the
parent relay will reject heartbeats (`401 relay_token_absent`) until its stale
binding is cleared. Each siemcore node is one binding at the parent, so wiping
both test pods means clearing **both** bindings: have the parent
`POST /api/v1/decommission` for each `instance_id`, or restart the parent relay
— either lets a node re-bind its token on the next honest heartbeat.

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

- Outbound to the mysoc relay only (TLS; pin its `cert.pem` via
  `server.ca_file`). No internet access required.
- Inbound from swf agents on the relay port (`:18443` — the ONE relay port
  cascade-wide, also avoiding the app's own `:8443` on v3 siemcore hosts).
  The listener protects its own port (1.10.0+; see "Naming and ports"), so
  it can be opened broadly. It is TLS-only (1.9.0+): it self-provisions a
  certificate under `relay.tls.dir` on first start. Give that directory's
  `cert.pem` to the swf team so their agents pin it, and list every address
  swf agents use to reach this host in `relay.tls.hosts`.

## Security properties

- The updater independently re-verifies the SHA-256 checksum and the ed25519
  release signature of every artifact — a compromised relay cannot substitute
  a payload.
- swf children are bound to relay tokens issued at first contact.
- Update results (success/failure) flow up the cascade automatically and are
  visible on the operator dashboard within one heartbeat interval per hop.
