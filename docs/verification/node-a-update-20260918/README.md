# Node A branding update and alpha updater rollout — 2026-09-18

**Completed:** node A automatically updated SiemCore **3.3.152.40 → 3.3.152.41**
at13:22:20 UTC. `/login` displays **Pod node 1**, without Observer branding or a
browser Basic challenge. HTTPS health reports .41, original machine/installation/
updater/node identities, management/data ready, and all processing/authority/POD/
link flags false. The node remains installed-unlinked.

## Delivered versions

- Cascade updater **1.16.1.30**, clean source `a04556f`, actual entrypoint
  `cmd/updater-simulator`. Published **stable / alpha only**. Artifact SHA
  `62219fa760704818fddf624aca22dcaa60feb6b83c225895a564effd54fef8f5`.
- Signed root maintenance kit **1.16.1.30-r1**, limited to A's exact identity and
  signed .40→.41 transition. No updater binary copied by this installer.
- Product .41 source `3e5b1f0d70893d64d3060ae6088726a9c977ec33`, archive SHA
  `5781c30da2cfc47220d8e116267f8cc9f5656c54139c95f6675385feee058ea7`.
  Product channel **node-a-20260918**, **alpha only**; fresh fleet query found
  only A reporting this channel. Published as the existing full single artifact.

## Tests and actual application

- Local Go updater regression suite and Node/Observer paths passed.
- Local30 Python node+Observer adapter/transport tests passed.
- Native AMD6415 node adapter/transport tests and node/self-update Go tests passed.
- Actual .30 CLI version, run, relay and installation-types commands passed.
- Isolated nested Docker29.1.3/Compose2.40.3 fixture used fresh synthetic databases,
  no host socket/data mounts, no external network. Actual .40/.41 images passed
  switch, failed-health recovery, interruption/reconciliation, accepted retry,
  data-container preservation, immutable bootstrap bytes and branded login.
  Fixture import/Compose/PATH issues were corrected as test infrastructure;
  production code and checks were not weakened. Fixture container removed afterward.
- Parent and A automatically downloaded, verified and restarted into .30;
  A finalized at13:19:33 UTC. Signed root component installer reported
  `product_execution=false`; actual service-user sudo readiness passed.
- A automatically downloaded and verified .41, invoked protected node adapter,
  and published filesystem pointers only after root acceptance.
- Root and product journals are **accepted**, operation
  `36636b19-cd40-4ee8-b64f-20ed09250ec9`. PostgreSQL/Redis container IDs/images and
  original bootstrap journal bytes are unchanged. Only management was replaced.
- Fresh central cascade heartbeat at13:24:20 UTC reports A .41 running, updater
  .30, alpha/automatic and successful application. All four active alpha Linux
  hosts report updater .30; benchmark remains held.
- SiemCore independently verified browser branding and HTTPS health; their receipt
  is `docs/operations/evidence/node-a-bootstrap-2026-09-18/product41-https-acceptance.json`
  in the SiemCore POD worktree.

## Fleet preservation

MySoc relay, Normal SiemCore, Observer and A all run .30 with alpha/automatic
self-updates. Normal and MySoc omit self_update configuration and therefore use
the verified stable default; A and Observer explicitly use stable. No special
self-update channel or per-host binary pin was added.

Normal health remains .39/alive; Observer remains .39/healthy/unlinked; MySoc
remains1.3.19.2 with HTTP200 and accepted heartbeats. Application versions/channels
were not changed for these hosts. Benchmark remains GCP TERMINATED and auto=false.

## Limits and incident

The server updater_requirements vocabulary still covers linked-POD maintenance,
not pod-node-update-v1. This first operation relies on the exclusive isolated
channel plus the installed updater/root signed exact-transition checks. It does
not claim broader server capability metadata or thin/dual publication qualification.
The initial update-kind upload was refused before persistence; readback showed
no .41 release before retrying the existing full single-artifact path.

A packaging error in withdrawn updater1.16.1.29 briefly interrupted the MySoc
relay updater; retained previous binary recovery restored it before the corrected
.30 rollout. See [incident details](PACKAGING-INCIDENT.md). Version.29 is not offered.

No production database copies, restores, migrations, initialization or role
activation occurred. Native fixture databases were generated only for testing.
