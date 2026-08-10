# Updater Simulator

The updater simulator is a safe Go client for testing the current
`updates.mysoc.ai` updater protocol. It can also be used as a skeleton by the
SiemCore and SWF teams.

The simulator:

- Enrolls with a dedicated test license when explicitly requested
- Sends realistic heartbeats
- Parses update hints returned by heartbeat
- Uses the group-aware update-check endpoint before accepting an offer
- Optionally downloads and verifies artifacts
- Simulates apply, validation, rollback, and result reporting
- Never executes a downloaded artifact
- Exposes an `Executor` interface for product-specific agent implementations

## Safety

The default mode is `observe`. It sends heartbeats and checks policy, but does
not download, execute, or report a successful update.

Requests to `updates.mysoc.ai` are not read-only:

- Heartbeats and update checks create or update instance records.
- Enrolling binds a license to `machine_id`.
- Re-enrolling may rotate an existing instance API key.
- Simulate mode sends an update-result report.

Use a dedicated test license and synthetic IDs beginning with `sim-`. Never
enroll using a customer or production license.

The current server does not consistently enforce updater credentials. The
simulator still sends `X-License-Key` and `X-API-Key`, but these headers are not
a security boundary until server-side enforcement is implemented.

## Build

```bash
make build-simulator
./bin/updater-simulator version
```

Or run directly:

```bash
go run ./cmd/updater-simulator version
```

## Example Configurations

- [SiemCore](../examples/updater-simulator/siemcore.yaml)
- [SWF](../examples/updater-simulator/swf.yaml)

Both examples:

- Use `https://updates.mysoc.ai`
- Use synthetic simulator identities
- Disable group-unaware legacy fallback
- Run in observe-only mode
- Leave credentials empty

Set credentials through environment variables:

```bash
export UPDATER_SIM_LICENSE_KEY='DEDICATED-TEST-LICENSE'
export UPDATER_SIM_API_KEY='OPTIONAL-ENROLLED-INSTANCE-KEY'
```

`UPDATER_SIM_LICENSE_KEY` and `UPDATER_SIM_API_KEY` override values in YAML.
State and artifact paths are resolved relative to the configuration file.

## Commands

### Enroll

Enrollment is the only command that activates and binds a license. It requires
an explicit confirmation flag:

```bash
./bin/updater-simulator \
  --config examples/updater-simulator/siemcore.yaml \
  enroll \
  --confirm-license-binding
```

The returned instance ID and API key are written to the configured state file
with `0600` permissions. Full credentials are not printed.

### Send One Heartbeat

```bash
./bin/updater-simulator \
  --config examples/updater-simulator/siemcore.yaml \
  heartbeat
```

Heartbeat update hints are logged without artifact URLs or credentials.

### Check Update Policy

Check every configured product:

```bash
./bin/updater-simulator \
  --config examples/updater-simulator/siemcore.yaml \
  check
```

Check one product:

```bash
./bin/updater-simulator \
  --config examples/updater-simulator/swf.yaml \
  check swf
```

The simulator prefers:

```text
POST /api/v1/updates/{product}/check
```

This endpoint honors instance auto-update and update-group policy. Legacy
fallback to `GET /api/v1/releases/{product}/latest` is disabled in the examples
because that path bypasses those controls.

### Run One Cycle

Observe only:

```bash
./bin/updater-simulator \
  --config examples/updater-simulator/siemcore.yaml \
  once
```

Download and verify without reporting success:

```bash
./bin/updater-simulator \
  --config examples/updater-simulator/siemcore.yaml \
  once --download
```

Simulate a successful update:

```bash
./bin/updater-simulator \
  --config examples/updater-simulator/siemcore.yaml \
  once --simulate
```

Simulate mode:

1. Sends a heartbeat.
2. Checks group-aware policy.
3. Downloads the artifact.
4. Verifies SHA-256 metadata and the download response header.
5. Calls the no-op executor.
6. Reports simulated success.
7. Advances the version only in the simulator state file.

It never starts or executes the artifact.

### Run Continuously

```bash
./bin/updater-simulator \
  --config examples/updater-simulator/siemcore.yaml \
  run
```

The first cycle runs immediately. Later cycles use the configured heartbeat
interval with jitter. `SIGINT` and `SIGTERM` stop the process cleanly.

`run --download` and `run --simulate` override the configured mode.

## Configuration

```yaml
server:
  url: https://updates.mysoc.ai
  license_key: ""
  api_key: ""
  timeout: 30s
  max_response_bytes: 1048576
  allow_external_downloads: false

instance:
  id: sim-product-dev-01
  type: simulator
  hostname: sim-product-dev-01
  machine_id: sim-product-dev-01
  updater_version: updater-simulator/1.1.0.3
  os: linux
  arch: amd64

heartbeat:
  interval: 60s
  jitter: 5s

simulation:
  mode: observe
  artifact_dir: ./artifacts
  state_file: ./.updater-simulator-state.json
  max_download_bytes: 1073741824
  legacy_fallback: false

products:
  - name: siemcore
    current_version: 0.0.0
    channel: stable
```

### Modes

- `observe`: heartbeat and policy check only
- `download`: download and verify only
- `simulate`: run the no-op lifecycle, report, and update simulator state

### Download Controls

- Artifact names are sanitized before writing.
- Downloads use a temporary file and are committed with rename after
  verification.
- A checksum from update metadata or `X-Checksum-SHA256` is required.
- If both checksums exist, they must agree.
- Maximum artifact size is enforced.
- Cross-origin artifact URLs are blocked by default.
- Credentials are stripped from cross-origin redirects.

Only enable `allow_external_downloads` for a trusted artifact CDN. Credentials
are never added to an external download request.

## API Compatibility

The simulator handles current API differences:

- Heartbeat and legacy release responses use `checksum`.
- Group-aware update checks use `sha256`.
- Download responses may use `X-Checksum-SHA256`.
- Group-aware update URLs are absolute.
- Heartbeat and legacy update URLs may be relative.
- Update checks may return `download_url` or `update_url`.

Heartbeat hints are treated as informational. The simulator always performs the
group-aware policy check before download because the current heartbeat handler
does not enforce update groups.

If `legacy_fallback` is enabled, fallback occurs only when the policy endpoint
returns HTTP 404 or 405. Authorization and server errors never fall back to the
less restrictive endpoint.

## Agent Skeleton

Reusable client and orchestration code lives in:

```text
pkg/updatersim
```

Product teams replace `NoopExecutor` with an implementation of:

```go
type Executor interface {
    Apply(context.Context, Update) error
    Validate(context.Context, Update) error
    Rollback(context.Context, Update) error
}
```

SiemCore and SWF executors must follow
[Update Server and Agent Guidelines](UPDATER-GUIDELINES.md), including:

- Single-instance and update-operation locks
- Manifest and signature verification
- Maintenance windows
- Atomic installation
- Health validation
- Durable state
- Automatic rollback
- Secret redaction

The simulator's SHA-256 verification is a transport-integrity check. A
production executor must additionally verify a trusted publisher signature.

## Tests

The package uses `httptest` and does not contact the live service:

```bash
go test ./pkg/updatersim
```

Tests cover:

- Heartbeat update parsing
- Group-aware update checks
- Relative download URLs
- SHA-256 verification
- Typed server errors
- Observe-only behavior
- Simulated reporting and version persistence
- Private state-file permissions

## Current Server Limitations

- Device credentials are not enforced consistently.
- Heartbeat update hints are not group-aware.
- The bundled updater ignores heartbeat response bodies.
- Update reports are acknowledged but not persisted.
- Signed grants and cluster leases are target behavior, not current endpoints.

Use the simulator to exercise current compatibility and as a foundation for
those stronger contracts; do not treat it as proof that the server already
enforces them.

