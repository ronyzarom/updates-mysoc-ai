# Fleet retirement marker fix — 1.16.1.18

The four retired Bezeq test VMs were correctly marked decommissioned at 13:43 UTC on September 15. Dashboard DELETE requests at 13:46–13:47 physically removed these records. The parent relay then recreated the same logical identities with new UUIDs using frozen pre-shutdown heartbeat timestamps. This was a reporting bug, not VM restart or duplicate updater execution.

Individual DELETE now marks the instance decommissioned and retains its identity, settings and history. Repeated deletion preserves the original retirement timestamp. Existing relay freshness validation rejects cached or undated reports; a genuinely newer host heartbeat can reactivate the instance. No migration is required. The confirmation dialog describes the retained record accurately. All-status fleet views can still show decommissioned history.

Validation: disposable PostgreSQL regression covers individual retirement, repeated deletion, stable identity/timestamp, stale and undated relay reports, and genuine fresh heartbeat recovery. Licensing and API tests pass; all 31 dashboard tests pass.

Live API 1.16.1.18, source 4b41541, binary SHA256 0807662649972c35f37b575fb39c8f47a007d07e432d97ad68b8ef9af624c28e. The four exact recreated identities were retired using two DELETE requests each at 13:58 UTC, after verifying their unchanged source heartbeat times. All eight calls returned 200 and all four rows remained decommissioned. No VM, disk, application database, release targeting or updater binary was changed.

The stopped pod B inventory placeholder had separately been explicitly deleted at 13:47 and was not recreated again. Active pod A and witness remain unchanged.

Post-deployment: after the 13:58:41 UTC parent rollup, all four retirement identities/timestamps were unchanged and decommissioned; A and witness were online. Dashboard production build passed and version 1.16.1.18 was activated successfully. Its served detail-page chunk page-a8bd1985d404055e.js contains the corrected dialog. Interactive browser verification was unavailable because the existing login session expired; HTTP health and served bundle verification passed.
