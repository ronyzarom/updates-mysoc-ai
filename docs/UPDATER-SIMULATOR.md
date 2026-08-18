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

- [MySoc](../examples/updater-simulator/mysoc.yaml) - root of the product hierarchy
- [SiemCore](../examples/updater-simulator/siemcore.yaml) - middle tier (parent = a mysoc)
- [SWF](../examples/updater-simulator/swf.yaml) - leaf tier (parent = a siemcore)
- [Local filesystem install](../examples/updater-simulator/filesystem-local.yaml) - real install/update against a local server

The tier examples:

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

## Product Hierarchy

The fleet is a three-tier tree owned by a single customer:

```text
mysoc  (root, no parent)
  └── siemcore   (parent = a mysoc)
        └── swf  (parent = a siemcore)
```

A customer owns **one license** that spans the whole tree. Every node — mysoc,
siemcore, and swf — presents the **same** `license_key`. The server binds each
node's `license_id` from that key and groups the fleet by customer, so set the
same value (ideally via `UPDATER_SIM_LICENSE_KEY`) on all three configs.

Each agent **self-reports** its place in the tree via two `instance` fields:

| Field          | Meaning                                                        | mysoc        | siemcore            | swf                 |
| -------------- | -------------------------------------------------------------- | ------------ | ------------------- | ------------------- |
| `product_tier` | Canonical tier: `mysoc`, `siemcore`, or `swf`                  | `mysoc`      | `siemcore`          | `swf`               |
| `parent_id`    | The **parent node's `instance.id`** (its `instance_id` on the wire) | *(omit)* | a mysoc instance id | a siemcore instance id |

`instance.type` stays the OS/sub-type (e.g. `swf-windows`); `product_tier` is the
canonical tier. Rules enforced on load:

- `product_tier` must be one of `mysoc`, `siemcore`, `swf` (case-insensitive).
- `mysoc` is the root and must **not** set `parent_id`.
- `siemcore` and `swf` **must** set `parent_id`.

The tier and parent are sent on every heartbeat and update-check as
`product_tier` and `parent_instance_id`. A child may enroll before its parent;
the server stores the link and surfaces it as an *orphan* in the tree until the
parent appears (no error). If the declared parent already exists with a
mismatched tier, the server rejects the report with `400`.

View the assembled tree via `GET /api/v1/instances/tree` or the dashboard's
Instances → Tree view.

**Credential hygiene.** The server matches credentials **exactly**, so the
simulator normalizes them on load: surrounding whitespace (including a trailing
newline from `$(cat key)`) is trimmed, and a value **wrapped in literal quotes**
is rejected with a clear error rather than silently stripped — fix the source
instead. Note that shell quotes in `export KEY='value'` are removed by the shell
and never reach the value; the guard only trips when quote characters are part
of the actual string (e.g. `license_key: "\"SIEM-…\""` in YAML). Remember the two
credentials are different: `X-License-Key` (device license) does **not**
authorize `POST /releases`, which requires an admin/`msk_` key in `X-API-Key`.

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
interval with jitter. On `SIGINT` or `SIGTERM` the simulator stops scheduling new
work, gives any in-flight cycle a bounded drain window (`simulation.drain_timeout`,
default `30s`), and then exits.

`run --download` and `run --simulate` override the configured mode.

### Reconcile a Desired-State Manifest

Reconcile drives the staged, health-gated pipeline (Section 8.7-8.13 of the
[Updater Guidelines](UPDATER-GUIDELINES.md)) toward a `system-template.json`
manifest instead of a single artifact. It covers the eight updater
responsibilities in one flow: code change, database migration, multi-container
orchestration, self-update, boot/signal handling, configuration render, health
and security gates, and per-stage monitoring.

Set `simulation.manifest_file` (see
[examples/updater-simulator/system-template.json](../examples/updater-simulator/system-template.json)),
then:

```bash
./bin/updater-simulator \
  --config examples/updater-simulator/siemcore.yaml \
  reconcile --simulate
```

The pipeline runs `prepare -> migrate -> containers -> config -> self-update ->
health -> security -> commit`. Any stage failure rolls back the prior stages and
reports the failing stage. `observe` computes and logs the plan only; `download`
runs the stages without committing state or reporting success; `simulate` commits
the simulated state and reports the result. Every stage is delegated to the
`ReconcilingExecutor` seam, which defaults to a no-op, so nothing on the host is
changed.

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
  product_tier: siemcore          # optional: mysoc | siemcore | swf
  parent_id: sim-mysoc-dev-01     # required for siemcore/swf; omit for mysoc

heartbeat:
  interval: 60s
  jitter: 5s

simulation:
  mode: observe
  artifact_dir: ./artifacts
  state_file: ./.updater-simulator-state.json
  manifest_file: ./system-template.json   # optional; required by `reconcile`
  max_download_bytes: 1073741824
  legacy_fallback: false
  drain_timeout: 30s                       # graceful shutdown window for `run`

products:
  - name: siemcore
    current_version: 0.0.0
    channel: stable
```

### Modes

- `observe`: heartbeat and policy check only
- `download`: download and verify only
- `simulate`: run the configured executor lifecycle, report, and update state

### Executors

The `simulation.executor` field selects what happens after an artifact is
downloaded and verified in `simulate` mode:

- `noop` (default): simulates apply/validate/rollback latency without changing
  the machine. Safe for protocol testing against any server.
- `filesystem`: a **real** installer that writes to disk. Use it to exercise a
  genuine install/update on a local machine.

The filesystem executor installs each version into its own directory and
activates it by atomically flipping a `current` symlink, so upgrades and
rollbacks are safe:

```text
<install_root>/<product>/releases/<version>/   extracted artifact per version
<install_root>/<product>/current               symlink -> releases/<version>
<install_root>/<product>/.previous             prior target, used by rollback
```

Behavior:

- A gzip'd tar artifact is extracted (with a zip-slip guard); any other content
  is copied as a single file.
- After the symlink swap, `restart_command` runs with `PRODUCT`, `VERSION`,
  `FROM_VERSION`, `CURRENT_DIR`, and `INSTALL_ROOT` in its environment.
- `Validate` confirms the live version and runs `health_command`; a failure
  triggers rollback to `.previous` (or removal of the symlink for a fresh
  install), then restart.
- `keep_releases` bounds retained version directories (current and previous are
  always kept).

```yaml
simulation:
  mode: simulate
  executor: filesystem
  artifact_dir: ./artifacts
  state_file: ./state.json
  filesystem:
    install_root: ./install
    restart_command: ["sh", "-c", "\"$CURRENT_DIR/app/run.sh\""]
    health_command: []            # optional; non-zero exit triggers rollback
    keep_releases: 3
    command_timeout: 30s
```

Because the filesystem executor changes the machine, run it only against paths
you control (a scratch `install_root`), and pair it with a local server when you
want a fully local end-to-end.

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

Product teams replace `NoopExecutor` with an implementation of the
single-artifact seam:

```go
type Executor interface {
    Apply(context.Context, Update) error
    Validate(context.Context, Update) error
    Rollback(context.Context, Update) error
}
```

and, for desired-state reconciliation, the staged seam:

```go
type ReconcilingExecutor interface {
    Prepare(context.Context, Plan) error           // guard: backups, DB snapshot/expand, migration lock
    Migrate(context.Context, Plan) error           // database migrations (expand/contract, locked)
    ApplyContainers(context.Context, Plan) error   // ordered container roll with health gates
    RenderConfig(context.Context, Plan) error       // render manifest configuration
    SelfUpdate(context.Context, Plan) error         // replace the running updater
    HealthCheck(context.Context, Plan) error        // commit gate
    SecurityCheck(context.Context, Plan) error      // commit gate
    RollbackReconcile(context.Context, Plan) error  // restore the Prepare restore point
}
```

`NoopExecutor` satisfies both seams so the simulator can exercise the full
reconcile pipeline safely by default.

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

