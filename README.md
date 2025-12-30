# MySoc Updates Platform

A complete update ecosystem for MySoc and SIEMCore instances, consisting of:

- **Update Server** (`updates.mysoc.ai`) - Hosts releases, manages licenses, receives heartbeats
- **Updater Agent** (`mysoc-updater`) - Bootstraps, updates, monitors, and secures instances
- **Admin Dashboard** - Fleet management UI

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              MYSOC.AI ECOSYSTEM                                  │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│                              ┌──────────────────┐                                │
│                              │     CI/CD        │                                │
│                              └────────┬─────────┘                                │
│                                       │                                          │
│                                       ▼                                          │
│                         ┌────────────────────────────┐                          │
│                         │      updates.mysoc.ai      │                          │
│                         │      (Update Server)       │                          │
│                         └─────────────┬──────────────┘                          │
│                                       │                                          │
│                                       ▼                                          │
│                         ┌────────────────────────────┐                          │
│                         │      cloud.mysoc.ai        │                          │
│                         │    (MySoc Production)      │                          │
│                         └─────────────┬──────────────┘                          │
│                                       │                                          │
│              ┌────────────────────────┼────────────────────────┐                │
│              ▼                        ▼                        ▼                │
│   ┌───────────────────┐   ┌───────────────────┐   ┌───────────────────┐        │
│   │  abc.siemcore.ai  │   │ acme.siemcore.ai  │   │ corp.siemcore.ai  │        │
│   └───────────────────┘   └───────────────────┘   └───────────────────┘        │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

## Quick Start

### Prerequisites

- Go 1.22+
- PostgreSQL 15+
- Node.js 20+ (for dashboard)

### Setup

1. Clone the repository:
```bash
git clone https://github.com/cyfox-labs/updates-mysoc-ai.git
cd updates-mysoc-ai
```

2. Install dependencies:
```bash
go mod download
```

3. Create database:
```bash
createdb mysoc_updates
psql -d mysoc_updates -f migrations/001_initial.up.sql
```

4. Configure environment:
```bash
export DB_HOST=localhost
export DB_PORT=5432
export DB_NAME=mysoc_updates
export DB_USER=postgres
export DB_PASSWORD=yourpassword
export ADMIN_API_KEY=your-admin-api-key
```

5. Run the server:
```bash
make run-server
```

### Building

```bash
# Build both server and updater
make build

# Build for Linux (cross-compile)
make build-updater-linux

# Run tests
make test
```

## API Endpoints

### License Management
- `POST /api/v1/license/activate` - Activate a license
- `POST /api/v1/license/validate` - Validate a license

### Releases
- `GET /api/v1/releases` - List all releases
- `POST /api/v1/releases` - Upload a release (admin)
- `GET /api/v1/releases/{product}/latest` - Get latest release
- `GET /api/v1/releases/{product}/{version}/download` - Download release

### Heartbeat
- `POST /api/v1/heartbeat` - Receive instance heartbeat

### Admin
- `GET /api/v1/instances` - List all instances
- `GET /api/v1/admin/licenses` - List all licenses

## Updater Agent

The `mysoc-updater` is a single binary that runs on each MySoc/SIEMCore instance.

### Installation

```bash
# One-liner installation
curl -sSL https://updates.mysoc.ai/install.sh | sudo bash

# Initialize with license
sudo mysoc-updater init --license YOUR-LICENSE-KEY
```

### Commands

```bash
mysoc-updater init --license XXX   # Bootstrap installation
mysoc-updater daemon               # Run as background service
mysoc-updater status               # Show current status
mysoc-updater update [product]     # Force update check
mysoc-updater rollback [product]   # Rollback to previous version
```

## Project Structure

```
updates-mysoc-ai/
├── cmd/
│   ├── update-server/       # Server entrypoint
│   └── mysoc-updater/       # Updater entrypoint
├── internal/
│   ├── server/              # Server internals
│   │   ├── api/             # HTTP handlers
│   │   ├── licensing/       # License management
│   │   ├── releases/        # Release management
│   │   └── storage/         # Artifact storage
│   └── updater/             # Updater internals
│       ├── bootstrap/       # Installation
│       ├── update/          # Update logic
│       ├── service/         # Service monitoring
│       └── security/        # Security hardening
├── pkg/                     # Shared packages
├── dashboard/               # Next.js admin UI
├── migrations/              # Database migrations
└── scripts/                 # Install scripts
```

## License

Copyright (c) 2024 CyFox Labs. All rights reserved.

