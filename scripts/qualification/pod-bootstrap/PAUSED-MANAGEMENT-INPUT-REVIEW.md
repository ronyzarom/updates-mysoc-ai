# Paused management stage: production-kit input review

Source review only. No new wire schema or installer entrypoint is approved here.
Existing data and Observer receipts remain partial, not activation permission.

## Existing immutable inputs to reuse

- Original schema4 input bytes and SHA256; registry fingerprint, pod/installation/
  operation IDs; persisted Observer generation. Node IDs1/2/witness and machine,
  infrastructure and updater identities remain unchanged. Translate1/2 to legacy
  deployment names app-a/app-b only at the artifact's installer boundary.
- Same retained signed SiemCore release product/version/architecture/commit,
  bundle checksum/signature/pinned trust, common image identity and exact verified
  script/helper bytes. Do not fetch a new script, image or dependency during apply.
- Real pinned Observer transport, node credential, detached still-valid invitation
  and independent authorization-key pin; fresh checks before consequential work
  and before returning an accepted receipt.
- application.selective_sync source1/target2/policy2 remains data-copy direction,
  never desired processing ownership.

## Product configuration requiring an exact protected schema

- Node-local database endpoint/name/application role and protected credential file;
  local Redis endpoint and secret. Reuse independent runtime ownership/configuration,
  not Patroni/shared-Sentinel assumptions or database-role promotion.
- Existing application instance/SSO association, parent MySoc URL, frontend public
  URL, JWT and other configured secrets as protected file references. Preserve
  existing identities and credentials on retry; never generate replacements silently.
- Immutable private management listen/peer addresses, TLS CA/cert/key and expected
  peer certificate/identity; management/controller service definitions and local
  runtime identity. Route IPs may be declared but are not acquired at this stage.
- Optional configured archive/LLM integrations remain protected references. No
  database backup, copy, credential logging, paid AI or public-registry fallback.
- Explicit supervised-v1 mode, migration/bootstrap disabled because the bound
  database stage already completed, and both processing runtimes (app and archiver)
  starting paused. A script's exit0 or HTTP liveness is not paused-state evidence.

SiemCore must provide the exact artifact-owned command/config/receipt contract;
Updates will not infer one from legacy install flags or a/b names.

## Host prerequisites before quorum

Prepare and qualify Python/tool versions, local immutable images and runtime UIDs,
Docker/Compose, effective kernel UDP buffers/backlog/forwarding, Docker logging
and proxy configuration before quorum. The source read-only check currently asks:
net.core.rmem_max>=67108864, rmem_default>=33554432,
netdev_max_backlog>=10000, optmem_max>=25165824, ip_forward=1,
conf.all.forwarding=1, json-file logging with100m/5 rotation and userland-proxy=false.
These are observed product requirements, not new Normal-install requirements.

Persist a protected host acceptance receipt tied to machine/boot and dependency
versions with fresh effective runtime checks. Parsing daemon.json alone is not
proof a running Docker daemon applied it. No sysctl rewrite, Docker restart or
package download may occur after quorum during this stage. An unmet prerequisite
returns an explicit incomplete error rather than repairing the host mid-operation.

## Updates execution callback

The bounded runner must authenticate the retained bundle first, verify every
executed script/helper/image under that provenance, match original identities and
stage config hashes, and check current host/dependency/Observer authorization.
Persist intent before spawn. Pass secrets through protected inputs, not argv/logs.
Capture only bounded structured output. A timeout/nonzero/lost response preserves
operation, runtime state and barrier; use exact-stage reconciliation/idempotent
retry, never teardown or reinitialization. The product must return an exact bound
paused-management receipt proving both runtimes' immutable identities, installed
release, management readiness and processing=false, with freshness and no active
assignment. Requery actual management/controller state after restart; do not treat
saved receipt as permanent proof.

No route switch, role-IP claim, processing grant, barrier clear or installation
completion is permitted. Those require the later explicit product readiness and
activation contract.

## Compatibility finding sent to SiemCore

The current greenfield Normal path calls the newly strengthened host prerequisite
checker and sets SIEMCORE_MANAGED_PREREQUISITES=1 before app installation. The app
installer now skips host tuning for that flag. This may make a previously valid
Normal clean install fail on defaults. Product must separate the corrected POD
admission behavior or qualify/prove preserved Normal behavior before a candidate.

Compatibility follow-up: SiemCore removed the unconditional greenfield host gate
and scoped the read-only helper to canonical supervised nodes1/2 with managed
prerequisites. Updates independently ran the actual product shell regression suite:
5tests passed in9.347s, including Normal/legacy cases with no added host probes.
This resolves the identified source-level gate regression; it is not a new live
Normal-install or release qualification claim.

Host observation transport clarification: container `/run/bootstrap` file references
must be materialized into a separate protected host data-binding config. Preserve
all registry/node/generation/original-input/trust/endpoint fields; rebase only
reviewed file references to byte-identical protected original materials. Freeze
and verify both resulting host config files before/after execution. Do not alter
the original input or container-stage config, and do not mount a Docker socket
inside a customer app container. Native integration needs the materialization
mapping and independent rendered-environment expectations in its fixture handoff.
