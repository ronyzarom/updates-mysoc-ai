# SiemCore Cluster Update Server Spec

| | |
|---|---|
| **Document Version** | 1.0.0 |
| **Last Updated** | May 3, 2026 |
| **Status** | Draft |
| **Maintained By** | SiemCore Platform Team |

---

## Purpose

For SiemCore v3 cluster deployments, the updates server becomes:

```text
release distribution + topology registry + rollout policy engine
```

It must not become a runtime orchestrator. It does not SSH, provision cloud resources, promote Postgres, or edit Redis directly.

---

## 1. Concepts

### Cluster

A cluster is a logical SiemCore deployment.

Required fields:

```json
{
  "cluster_id": "customer-prod",
  "topology": "cluster",
  "template": "ha-40k-eps",
  "provider": "gcp",
  "profile": "gcp-ha",
  "eventbus": "redis",
  "desired_version": "3.1.0",
  "desired_schema_version": 136,
  "status": "healthy"
}
```

### Node

Each VM registers as one immutable role.

```json
{
  "node_id": "app-a",
  "siemcore_instance_id": "app-a",
  "cluster_id": "customer-prod",
  "role": "app",
  "current_version": "3.0.0",
  "desired_version": "3.1.0",
  "updater_version": "1.15.0",
  "health": "ready",
  "last_seen": "2026-05-02T20:50:00Z"
}
```

The updates server may call the node identifier `node_id`, but it maps to the runtime `SIEMCORE_INSTANCE_ID` used by SiemCore on the VM. `siemcore_instance_id` must be reported when the values differ or when compatibility with existing runtime tooling requires the explicit field.

`SIEMCORE_INSTANCE_ID` is per-node. `SIEMCORE_SSO_INSTANCE_ID` is shared across app nodes and must not be used as the node identity in the updates server registry.

Allowed roles:

```text
app
db
witness
loadgen
```

Role is selected at install time and should not be changed automatically.

---

## 2. Topology Templates

Support templates:

```text
single-node
ha-minimum
ha-40k-eps
ha-80k-eps
```

Example:

```json
{
  "template": "ha-40k-eps",
  "provider": "gcp",
  "profile": "gcp-ha",
  "eventbus": "redis",
  "required_roles": {
    "witness": 1,
    "db": 2,
    "app": 4
  },
  "optional_roles": {
    "loadgen": 2
  },
  "load_balancers": {
    "https": "cloud-managed",
    "syslog": "cloud-managed-network"
  }
}
```

---

## 3. Release Bundle

Cluster releases are universal bundles.

```text
siemcore-universal-3.1.0.tar.gz
  MANIFEST.json
  images/siemcore-3.1.0.tar
  roles/app/*
  roles/db/*
  roles/witness/*
  roles/loadgen/*
  migrations/*
```

Manifest:

```json
{
  "product": "siemcore",
  "bundle_type": "universal-cluster",
  "version": "3.1.0",
  "topology": "cluster",
  "eventbus": "redis",
  "roles": ["app", "db", "witness", "loadgen"],
  "schema": {
    "from": 135,
    "to": 136,
    "migrationMode": "online"
  },
  "compatibility": {
    "minUpdater": "1.15.0",
    "allowedFrom": ["3.0.x"]
  },
  "artifacts": {
    "image": "images/siemcore-3.1.0.tar"
  }
}
```

---

## 4. Release Channels

Minimum channels:

```text
siemcore-single
siemcore-cluster-redis
siemcore-cluster-kafka
```

Rules:

```text
single-node cannot receive cluster bundle
cluster redis cannot receive kafka bundle unless migration explicitly enabled
A node may only execute the update path for its immutable installed role
node updater version must meet manifest minUpdater
```

---

## 5. Node Registration

Installer registers with token:

```bash
./install-siemcore-node.sh --token <registration-token>
```

Token resolves to:

```json
{
  "cluster_id": "customer-prod",
  "node_id": "app-a",
  "role": "app",
  "profile": "gcp-ha",
  "eventbus": "redis",
  "target_version": "3.0.0"
}
```

The node then downloads the universal bundle and runs:

```text
roles/<role>/install.sh
```

---

## 6. Update Flow

Node checks in:

```http
POST /api/v1/update/checkin
```

Request:

```json
{
  "cluster_id": "customer-prod",
  "node_id": "app-a",
  "siemcore_instance_id": "app-a",
  "role": "app",
  "topology": "cluster",
  "eventbus": "redis",
  "current_version": "3.0.0",
  "current_schema_version": 135,
  "updater_version": "1.15.0",
  "health": "ready"
}
```

`current_schema_version` is the schema version currently observed or applied by the node. The updates server compares it with the cluster `desired_schema_version` to detect drift before granting update or migration work.

Response:

```json
{
  "can_update": true,
  "target_version": "3.1.0",
  "bundle_url": "https://updates.mysoc.ai/releases/siemcore-universal-3.1.0.tar.gz",
  "checksum": "sha256:...",
  "signature": "...",
  "strategy": "rolling-app"
}
```

If blocked:

```json
{
  "can_update": false,
  "reason": "cluster_not_healthy"
}
```

---

## 7. Rollout Policy

### App Nodes

Automated rolling update:

```text
one app node at a time
download bundle
docker load image
docker compose up -d
wait /health/live
wait /health/ready
verify version
continue to next app node
```

The previous image/tag must remain available locally during rollout. If an app node fails its health or version checks, rollback is performed on that node only before the rollout proceeds or is blocked.

App rollout must not start until required schema migrations have been applied once for the cluster. Schema migrations are cluster-level. The updates server grants exactly one migration lease for the target schema version. The selected node runs the migration bundle once, reports success, and only then may rolling app rollout begin.

### DB Nodes

V1: manual approval only.

Required gates:

```text
recent backup exists
standby healthy
replication lag acceptable
maintenance mode enabled
explicit approval
```

Schema migrations are not a per-node DB update step. DB node updates remain manual-approval operations separate from the single migration lease used to advance the cluster schema.

### Witness

Semi-automated, but never during DB failover or degraded quorum.

---

## 8. Cluster Health Gates

Before allowing app rollout:

```text
minimum app nodes healthy
DB primary reported
DB standby healthy
Redis master reported
Sentinel quorum healthy
witness reachable
app nodes report compatible JWT keyset / kid set
no active failover
schema version compatible
no other rollout active
```

For 40K EPS template:

```text
minimum 4 app nodes required for this template
2 DB/Redis nodes required
1 witness required
```

---

## 9. Validation Rules

The updates server must reject:

```text
wrong topology
wrong eventbus
wrong role
unsupported version jump
missing required node roles
cluster unhealthy
all app nodes updating at once
DB update without manual approval
role mutation after install
```

Example:

```json
{
  "can_update": false,
  "reason": "redis cluster cannot receive kafka bundle without migration flag"
}
```

---

## 10. Responsibilities Split

Updates server owns:

```text
topology templates
node registry
release channels
bundle distribution
checksums/signatures
compatibility validation
rolling app rollout coordination
audit log
```

Node updater owns:

```text
local install/update
role-specific scripts
docker load
docker compose
health checks
rollback to previous image
status reporting
```

Terraform/cloud owns:

```text
VPC
GCP HTTPS LB
GCP Network LB
firewall rules
disks
DNS
VM creation
```

Runtime HA tools own:

```text
pg_auto_failover
Redis Sentinel
HAProxy local DB routing
```

---

## One-Line Requirement

SiemCore v3 cluster updates must be delivered as signed universal container bundles. The updates server must track topology, enforce role/eventbus compatibility, and coordinate safe rolling app updates without directly orchestrating infrastructure or runtime failover.
