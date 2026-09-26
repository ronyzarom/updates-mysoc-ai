# MySoc Updates Platform

The update and fleet-monitoring platform for MySoc, SiemCore and the SiemCore
Windows Forwarder (SWF):

- **Update server** (`updates.mysoc.ai`) — hosts and signs releases, manages
  licenses and operators, and receives the fleet's heartbeats.
- **Cascade updater** — one updater binary on every node. It keeps the node's
  products (and itself) updated, and on mysoc and siemcore nodes also runs as a
  **relay** for the tier below.
- **Admin dashboard** — fleet, releases, licenses and rollout management.

## Architecture

Nodes never talk to more than one URL. Only the mysoc tier reaches
`updates.mysoc.ai`; every other node talks only to its parent relay, so customer
sites need no route to the internet or to the update server.

```text
                  Release uploads (admin or releases-scoped API keys)
                                        │
                                        ▼
                             updates.mysoc.ai
                  update server + dashboard, one platform license per operator
                                        │
               ┌────────────────────────┼────────────────────────┐
               ▼                        ▼                        ▼
        cloud.mysoc.ai          operator B mysoc          operator C mysoc      tier 1: mysoc
               │                        │                        │              updater + relay
               ▼                 ┌──────┴──────┐                 ▼
         siemcore host       siemcore ... siemcore          siemcore ...        tier 2: siemcore
                                 │                               │              updater + relay (:18443)
                                 ▼                               ▼
                            swf  swf  swf                    swf  swf           tier 3: swf
                                                                                updater (leaf)
```

- **Tiers** are enforced by the server (`internal/server/catalog/tiers.go`):
  mysoc is the root, siemcore's parent must be a mysoc node, swf's parent must
  be a siemcore node.
- **Operators.** Each SOC operator runs its own mysoc node with its own
  platform license. There is no single central mysoc.
- **Relays** (`pkg/updatersim/relay.go`) serve their children heartbeat,
  update check, report, decommission, release metadata and artifact download on
  one TLS port (`18443`). Update checks are forwarded upstream with the relay's
  **own** license, download URLs are rewritten to the relay, and artifacts come
  from a pull-through cache.
- **Trust.** Releases are ed25519-signed when `RELEASE_SIGNING_SEED` is set (it
  is in production); the public key is published at
  `GET /api/v1/signing-key` and pinned in every updater config. SHA-256 and
  signature are verified at every hop, so a compromised relay cannot inject an
  artifact. A child is authenticated to its relay by the relay token issued on
  first contact.
- **Visibility.** Each relay rolls its subtree up in its own heartbeat, so the
  server and dashboard see the whole fleet without any node below mysoc
  connecting to them.

Details: [Relay Deployment Guide](docs/RELAY-DEPLOYMENT.md),
[API Contract §9](docs/API-CONTRACT.md#9-cascade-distribution-added-in-180) and
[Updater Guidelines §14](docs/UPDATER-GUIDELINES.md#14-cascaded-distribution-180).

## Quick Start (development)

### Prerequisites

- Go 1.22+
- PostgreSQL 15+
- Node.js 20+ (dashboard)

### Setup

```bash
git clone https://github.com/ronyzarom/updates-mysoc-ai.git
cd updates-mysoc-ai
go mod download

# Database. Schema migrations run automatically when the server starts.
createdb mysoc_updates
psql -d mysoc_updates -c 'CREATE EXTENSION IF NOT EXISTS "uuid-ossp"'

export DB_HOST=localhost DB_PORT=5432 DB_NAME=mysoc_updates
export DB_USER=postgres DB_PASSWORD=yourpassword
export ADMIN_API_KEY=your-admin-api-key
export RELEASE_SIGNING_SEED=$(openssl rand -hex 32)   # optional: enables release signing

make run-server                          # API on :8080
cd dashboard && npm ci && NEXT_PUBLIC_API_URL=http://localhost:8080 npm run dev   # dashboard on :3001
```

### Build and test

```bash
make build-server                  # bin/update-server
make build-simulator               # bin/updater-simulator (the cascade updater)
make test                          # Go tests
cd dashboard && npm test           # dashboard unit tests
scripts/build-kits.sh              # updater kits into dist/updater-kits/<kit>-<VERSION>/
```

Builds are stamped from the repo `VERSION` file and the short commit; a build
from an uncommitted tree is stamped `-dirty` and is diagnostic only. Releases
follow `.cursor/rules/development-cycle.mdc`; server deployment is described in
[DEPLOYMENT.md](DEPLOYMENT.md) and `scripts/deploy.sh`.

## API Overview

Authoritative reference: [API Contract](docs/API-CONTRACT.md).

| Area | Endpoints | Auth |
| ---- | --------- | ---- |
| Health | `GET /health` — `status`, `version`, `commit` | public |
| Signing key | `GET /api/v1/signing-key` | public |
| Updater data plane | `POST /api/v1/heartbeat`, `POST /api/v1/updates/{product}/check`, `POST /api/v1/updates/{product}/report`, `GET /api/v1/releases/{product}/{version}/download` | `X-License-Key` (the same paths are served by relays) |
| Releases | `GET /api/v1/releases`; `POST /api/v1/releases` (upload); `PUT …/{version}/target-groups` | upload and changes: admin or `releases`-scoped API key |
| Instances | `GET /api/v1/instances`, `/paged`, `/tree`, `/stats` | dashboard JWT |
| Admin | `/api/v1/admin/licenses`, `/operators`, `/trusted-keys`, `/api-keys` | admin |

Published releases are immutable: uploading an existing product and version, or
overwriting its file, returns `409`. Uploads may carry an optional issuer seal
(`issuer_signature`, `issuer_key_id`); the server records `seal_status` and never
rejects an upload because of it ([API Contract §9.6](docs/API-CONTRACT.md#96-issuer-sealing-added-in-1162)).

## Cascade Updater

The updater is built from `cmd/updater-simulator` and shipped as a per-tier kit
from `kits/` by `scripts/build-kits.sh`. Kits run it in relay mode:

| Tier | Kit | Unit | Upstream (`server.url`) |
| ---- | --- | ---- | ----------------------- |
| mysoc | `kits/mysoc` | `mysoc-updater` | `https://updates.mysoc.ai` |
| siemcore | `kits/siemcore` | `siemcore-cascade-updater` | the operator's mysoc relay, `https://<mysoc-host>:18443` |
| swf | Windows service, see [SWF Windows Updater Guide](docs/SWF-WINDOWS-UPDATER-GUIDE.md) | — | the customer's siemcore relay |

Install from an unpacked kit (flags are prompted for when missing):

```bash
sudo ./install.sh --update --license-key <credential> --instance-id <id> ...
sudo systemctl start mysoc-updater        # or siemcore-cascade-updater
```

New siemcore servers can be installed by the customer alone; see
[Self-Service Installation](docs/SELF-SERVICE-INSTALL.md). The updater updates
itself through the same cascade: publish `updater-linux-amd64` /
`updater-linux-arm64` releases ([Updater Kit Release Process](docs/UPDATER-RELEASE.md)).

`cmd/mysoc-updater` (`init`, `daemon`, `status`, …) is the earlier standalone
agent. It is not part of the kits and is not what the fleet runs.

## Documentation

Cascade and updater:

- [Relay Deployment Guide](docs/RELAY-DEPLOYMENT.md) — running relays at each tier, port protection, scaling
- [Self-Service Installation](docs/SELF-SERVICE-INSTALL.md) — customer-installed siemcore node
- [SWF Windows Updater Guide](docs/SWF-WINDOWS-UPDATER-GUIDE.md) — the leaf tier
- [Updater Kit Release Process](docs/UPDATER-RELEASE.md) — building, tailoring and self-update releases
- [Update Entrypoint Contract](docs/UPDATE-ENTRYPOINT-CONTRACT.md) — the host `updater/apply` contract products implement
- [Updater Simulator](docs/UPDATER-SIMULATOR.md) — protocol testing harness and the updater's code base

Server and fleet:

- [API Contract](docs/API-CONTRACT.md) — endpoints, auth model, cascade wire format
- [Update Server and Agent Guidelines](docs/UPDATER-GUIDELINES.md) — release lifecycle, agent safety, cascade rules
- [License Ownership Guide](docs/LICENSE-OWNERSHIP-GUIDE.md) — operator, reseller and customer model
- [MySoc Admin Guide](docs/MYSOC_ADMIN_GUIDE.md) — dashboard and release administration
- [SiemCore Deployment Guide](docs/SIEMCORE_DEPLOYMENT_GUIDE.md) — SiemCore rollout operations
- [SiemCore Cluster Update Server Spec](docs/SIEMCORE-CLUSTER-UPDATE-SERVER-SPEC.md) — cluster registry and rollout policy (draft)
- [Release Notes](docs/RELEASE-NOTES.md) — what shipped in each version

The MySoc Admin Guide, SiemCore Deployment Guide and the client section of
`DEPLOYMENT.md` predate the cascade in places and still show siemcore nodes
contacting `updates.mysoc.ai` directly; where they disagree with the documents
above, the cascade documents are correct.

## Project Structure

```
updates-mysoc-ai/
├── cmd/
│   ├── update-server/       # Server entrypoint
│   ├── updater-simulator/   # Cascade updater (relay/run/loadgen); shipped in the kits
│   └── mysoc-updater/       # Earlier standalone agent (not shipped)
├── internal/
│   ├── server/              # API, auth, catalog (tiers), licensing, releases, storage, migrations runner
│   └── updater/             # Internals of the standalone agent (cmd/mysoc-updater)
├── pkg/
│   ├── updatersim/          # Updater and relay implementation
│   ├── signing/             # ed25519 release signing
│   ├── artifactprotocol/    # Independent artifact metadata
│   └── types/               # Shared wire types
├── kits/                    # Updater kit templates (mysoc, siemcore)
├── dashboard/               # Next.js admin UI
├── migrations/              # Embedded, auto-applied schema migrations
├── deployments/             # Docker Compose and systemd definitions
├── examples/                # Simulator configurations
└── scripts/                 # Build, kit, deploy and upload scripts
```

## License

Copyright (c) 2024 CyFox Labs. All rights reserved.
