# Live alpha Observer automatic upgrade

Signed Updates maintenance kit **1.16.1.27-r1**, clean commit
8e9a211c3fbfa260f4c97a65a8a195357682405f, archive SHA256
6023cc9781e3e7dda2a4ed8c1caeee71d1eab7467207e90e7853219575000393.
Both updater binary and outer kit manifest verified under the existing fleet key.
The kit was delivered only to the isolated Observer using approved root host
preparation. No global updater release or fleet reassignment was performed.

## Live execution

- Maintenance installed root component and updater 1.16.1.27, preserving product
  process and original bootstrap pins. Separate existing bcrypt credential was
  provisioned root:root 0600; no plaintext credential entered kit or logs.
- New updater started 06:46:59Z with self-update channel stable. Existing alpha
  assignment and automatic updates remained enabled, product obs-test-20260918.
- 06:47:00Z updater received .37 -> .38 offer; 06:47:04Z it completed download and
  signature/checksum verification of 65,296,083 bytes. Root application ran only
  through the service's fixed sudo boundary and signed private staging.
- Operation 5b9fce01-a90d-4c25-977f-ea6bffd999ea, binding hash
  59956d4a016e6b1acb8680627d8f12c15d8cfbe9708407f7d242908a051e5099:
  root and product both accepted; root timestamp 06:47:13.816818Z.
- 06:47:15Z updater activated matching local receipt/pointer and reported success.
- Exact HTTPS health: product 3.3.152.38; binary digest
  f73f01a0b75e0182d92d420024760ae69598544b81d6da26a280a22b469855d6;
  installed-unlinked; management ready; authority, processing and pod readiness
  remain false. Installation/updater IDs unchanged.
- Certificate-validated UI checks: no credentials 401, wrong password 401, actual
  separately held operator credential 200. Root readiness's own login_verified=false
  means that root probe does not use a password; external authenticated check is
  recorded separately in auth.json.
- Original bootstrap journal hash remains
  7d2d73e5c50ed7ef0d340e425a89a5832618498484d06cd5ee736edf9bb58499.
- Explicit updater-only restart: PID9993 ->10338. Product PID10169 unchanged.
  Readiness still advertises observer-unlinked-update-v1 after accepted upgrade.

This qualifies the isolated independent Observer update. It does not activate a
linked POD/authority, publish the updater fleet-wide, or run destructive recovery
against the live Observer. Failed-target isolation/recovery and maintenance rollback
were exercised in fixtures; current product recovery conservatively blocks instead
of claiming restoration of an unsafe predecessor.

## Final stability and cascade result

Seven samples at 0/50/100/150/200/250/300 seconds passed after updater restart.
Product PID10169 and updater PID10338 stayed unchanged, active, with NRestarts=0.
Exact product .38 health, unauthenticated401, accepted updater state and all
recorded bootstrap/operation journal hashes remained unchanged. See stability.jsonl.

Origin cascade heartbeat 06:49:07.033603Z reports updater1.16.1.27 and product
3.3.152.38/running, alpha, auto-update=true, online. The .37 -> .38 success is
recorded at06:47:15.263476Z. Benchmark remains alpha/auto=false. See alpha-rollup.json
and benchmark-hold.json. No other release target or assignment was changed.
