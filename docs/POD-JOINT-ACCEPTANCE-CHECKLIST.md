# Joint pod acceptance evidence checklist

Owners: Updates (coordinator/delivery/receipts), SiemCore (observer, privileged
adapter, product lifecycle, measured installed health). No deployment is approved
by a passing fixture, a version string, or a tier label.

Evidence levels, recorded per test:
- **component**: synthetic host/adapter protocol or in-memory unit evidence.
- **native-with-synthetic-host**: real process, etcd and/or mTLS with synthetic host evidence.
- **executable-real-host**: disposable real host/runtime with signed product lifecycle and measured health.

Current Updates harness: component only. SiemCore separately reports native etcd/
mTLS tests; attach those receipts as their actual tier, never upgrade their label.

| ID | Required evidence | Owner | Acceptance gate |
|---|---|---|---|
| WIRE-1 | Literal strict request/response parity, duplicate/unknown/trailing/oversize rejection, unsupported/not-ready refusal | Both | Real adapter against coordinator |
| ID-1 | Independently provisioned exact pod/node/updater registry, pinned mTLS, least-privilege observer key; wrong identity denied | SiemCore | Real authenticated endpoint |
| READY-1 | <=60s proof with real credentials and complete lifecycle; absent capability defaults off; witness/unknown not Normal | Both | Real readiness + origin/client gates |
| BEGIN-1 | Durable begin before restart, same operation on lost reply, no nested maintenance | Both | Crash before/after acknowledgement |
| DISCOVER-1 | Lost-begin generation=0 read-only exact-binding lookup; positive durable generation; zero mutation rejected | Both | Observer reconstruction + unchanged store proof |
| DRAIN-1 | Processing/ingress stopped/quiescent while DB/Redis preserved; evidence <=5s; exact captured generations | SiemCore | Measured host evidence |
| WATCH-1 | Retirement receipt and actual exit before captured-owner CAS; newer owner/watchdog untouched; emergency isolation preserved | SiemCore | Crash/revocation/unknown-exit native tests |
| APPLY-1 | Release signature/hash plus signed manifest product/version/architecture/commit/runtime_image_id; actual app and archiver image IDs match | Both | Real signed bundle + Docker measurements |
| APPLY-2 | Parent/worker death and lost replies at each boundary; no second apply after completion; descendants safely supervised/drained | Both | Native child/service crash cases |
| HEALTH-1 | Paused health separately from accepted role; ACTIVE fresh authority/IP/service; STBY management, Mirror-STBY and processing=false/quiescent=true | SiemCore | Measured host + observer evidence |
| ROLLBACK-1 | Exact retained predecessor signature/digest and role acceptance; report failed target with actual predecessor version; no DB restore | Both | Real retained-artifact recovery |
| EXPIRY-1 | Original deadline unchanged; distinct observer scoped grant; expiry/revocation/replay/hash-history survives restart | Both | Executable recovery-v2 + observer |
| DRAIN-2 | Separate drain grant/domain only to paused; stale evidence, different owner, missing capture, expiry and unknown termination retain barrier | Both | Executable drain-v1 (not in .25) |
| ARCHIVE-1 | Exact original evidence archived before next intent; signed next scope; crash prepared/unlinked/new-intent cases; valid ACTIVE lease allowed until next drain | Both | Real coordinator + observer authorization |
| NORMAL-1 | Normal server lifecycle unchanged, same binary/artifact, product prerequisites still enforced | Both | Normal install/upgrade regression |
| RECOVERY-1 | Startup/cycles resume retained verified operation without new offer or reachable origin; no substituted artifact | Updates | Real process restart + outage |
| REPORT-1 | Actual installed version/restart/heartbeat/role reports preserved end-to-end; rollback never target success | Both | Cascade read-back |
| POD-1 | Controlled handoff, fencing, no automatic failback, restart paused, selective replication/RPO and degraded-state honesty | SiemCore | Full disposable pod acceptance |

Harness instructions: `scripts/qualification/pod-maintenance/README.md`.
Build driver from current source, configure product adapter absolute argv in
fault_proxy.json, then give run_suite.py product-owned reset/launch/ready/verify/
stop argv. Do not reset state between a crash and retry. Persist raw protocol
receipts and installed/runtime measurements with hashes. Keep test keys/grants
separate from live credentials. No production DB copies/backups/restores.

Run summary always states deployment_qualified=false: both teams review the above
receipts before separately authorizing a signed publication/host rollout. Candidate
1.16.1.25 remains the unchanged unsigned private artifact from 040eb9d; the later
harness/contract work is not evidence that this candidate executes drain recovery.

Concrete missing joint input: full SiemCore executable adapter and its isolated
launch/config/certificate/artifact/health fixtures. Restricted drain observer and
runtime provenance inspection alone do not satisfy the full lifecycle contract.
