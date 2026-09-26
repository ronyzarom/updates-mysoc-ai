# MySoc Updates Platform - Deployment Guide

## Quick Start

### Prerequisites

- Go 1.22+
- PostgreSQL 15+
- Node.js 20+ (for dashboard)
- Docker & Docker Compose (optional)

### 1. Database Setup

```bash
# Create database (the uuid-ossp extension needs a superuser once)
createdb mysoc_updates
psql -d mysoc_updates -c 'CREATE EXTENSION IF NOT EXISTS "uuid-ossp"'
```

Schema migrations are applied automatically by the server at startup
(embedded `migrations/*.up.sql`, recorded in `schema_migrations` under an
advisory lock). A database that predates the runner is baselined on first
start. Never edit an applied migration — add a new numbered one. Rollbacks
are an operator action using the on-disk `*.down.sql` files.

### 2. Build

```bash
# Build both server and updater
make build

# Or build separately
make build-server
make build-updater

# Cross-compile updater for Linux
make build-updater-linux
```

### 3. Configure

Set environment variables:

```bash
export SERVER_PORT=8080
export DB_HOST=localhost
export DB_PORT=5432
export DB_NAME=mysoc_updates
export DB_USER=postgres
export DB_PASSWORD=yourpassword
export DB_SSL_MODE=disable
export STORAGE_TYPE=local
export STORAGE_LOCAL_PATH=./artifacts
export ADMIN_API_KEY=your-secret-admin-key
```

### 4. Run

```bash
# Run the server
./bin/update-server

# Or use make
make run-server
```

---

## Production Deployment

### Using Docker Compose

```bash
cd deployments/docker

# Set environment
export DB_PASSWORD=securepassword
export ADMIN_API_KEY=your-admin-key

# Start services
docker-compose up -d

# View logs
docker-compose logs -f update-server
```

### Using Systemd

1. Create system user:
```bash
sudo useradd -r -s /bin/false mysoc-updates
```

2. Create directories:
```bash
sudo mkdir -p /opt/mysoc-updates/{bin,artifacts}
sudo chown -R mysoc-updates:mysoc-updates /opt/mysoc-updates
```

3. Copy binary:
```bash
sudo cp bin/update-server /opt/mysoc-updates/bin/
```

4. Install systemd service:
```bash
sudo cp deployments/systemd/update-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable update-server
sudo systemctl start update-server
```

5. Check status:
```bash
sudo systemctl status update-server
journalctl -u update-server -f
```

---

## Nginx Configuration

For production, use Nginx as a reverse proxy:

```nginx
server {
    listen 443 ssl http2;
    server_name updates.mysoc.ai;

    ssl_certificate /etc/letsencrypt/live/updates.mysoc.ai/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/updates.mysoc.ai/privkey.pem;

    # API
    location /api/ {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # Health check
    location /health {
        proxy_pass http://localhost:8080;
    }

    # Dashboard
    location / {
        proxy_pass http://localhost:3001;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

---

## Updater Deployment

Updaters are installed from the per-tier kits built by `scripts/build-kits.sh`
(see [Updater Kit Release Process](docs/UPDATER-RELEASE.md)). Each node talks
to exactly one upstream: only the mysoc tier reaches `updates.mysoc.ai`.

| Tier | Kit / unit | Upstream |
| ---- | ---------- | -------- |
| mysoc | `mysoc-updater` | `https://updates.mysoc.ai`, operator platform key |
| siemcore | `siemcore-cascade-updater` | the operator's mysoc relay (`https://<mysoc-host>:18443`) |
| swf | Windows service ([guide](docs/SWF-WINDOWS-UPDATER-GUIDE.md)) | the customer's siemcore relay |

From an unpacked kit:

```bash
sudo ./install.sh --update --license-key <credential> --instance-id <id> ...
sudo systemctl start mysoc-updater        # or siemcore-cascade-updater
```

Relay setup at each tier: [Relay Deployment Guide](docs/RELAY-DEPLOYMENT.md).
Customer-installed siemcore nodes: [Self-Service Installation](docs/SELF-SERVICE-INSTALL.md).

---

## Uploading Releases

### Using curl

```bash
curl -X POST https://updates.mysoc.ai/api/v1/releases \
  -H "X-API-Key: YOUR-ADMIN-KEY" \
  -F "product=siemcore-api" \
  -F "version=1.0.0" \
  -F "channel=stable" \
  -F "release_notes=Initial release" \
  -F "artifact=@siemcore-api-linux-amd64"
```

### Using the dashboard

1. Navigate to Releases page
2. Click "Upload Release"
3. Fill in product details
4. Upload the binary

---

## Monitoring

### Logs

```bash
# Server logs
journalctl -u update-server -f

# Updater logs (on instances)
journalctl -u mysoc-updater -f              # mysoc tier
journalctl -u siemcore-cascade-updater -f   # siemcore tier
```

### Health Check

```bash
curl https://updates.mysoc.ai/health
```

### API Status

```bash
# List instances (dashboard JWT from POST /api/v1/auth/login; the admin API key is not accepted here)
curl https://updates.mysoc.ai/api/v1/instances \
  -H "Authorization: Bearer $ACCESS_TOKEN"

# List releases
curl https://updates.mysoc.ai/api/v1/releases
```

---

## Backup

### Database

```bash
pg_dump mysoc_updates > backup_$(date +%Y%m%d).sql
```

### Artifacts

```bash
tar -czf artifacts_$(date +%Y%m%d).tar.gz /opt/mysoc-updates/artifacts
```

---

## Troubleshooting

### Server won't start

1. Check database connection:
```bash
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "SELECT 1"
```

2. Check logs:
```bash
journalctl -u update-server -n 100
```

### Updater can't connect

1. Check the node's **parent** is reachable (`server.url` in the updater
   config): `updates.mysoc.ai` only for mysoc nodes, the mysoc relay for
   siemcore nodes.
```bash
grep -A3 '^server:' /etc/siemcore-cascade-updater/config.yaml   # or /etc/mysoc-updater/config.yaml
curl --cacert <server.ca_file> https://<parent-host>:18443/health
```

2. Check the updater log for the rejection reason (for example
   `relay_token_mismatch` or `certificate signed by unknown authority`):
```bash
journalctl -u siemcore-cascade-updater -n 100
```

### License activation fails

1. Verify license key format
2. Check server logs for details
3. Ensure network connectivity

---

## Security Recommendations

1. **Use TLS**: Always use HTTPS in production
2. **Secure API keys**: Store admin keys securely, rotate regularly
3. **Firewall**: Restrict access to the update server
4. **Updates**: Keep the server and updater up to date
5. **Backups**: Regular database and artifact backups
6. **Monitoring**: Set up alerts for offline instances

