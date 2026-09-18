# Normal .36 -> .39 test — application pass, TLS posture failure

**Not a full acceptance pass.** The application update and Normal UI succeeded,
but SiemCore's independent audit found a changed syslog TLS certificate on6514.
Wider promotion remains unchanged. No rollback or certificate modification was done.

## Isolated delivery

Instance siemcore-testing-01 / UUID6bec992a-2699-40c8-93db-6dffd17eaeaf,
testing.siemcore.ai, Tailscale100.118.106.93. Existing updater1.16.1.24 remained
on its Normal/standalone filesystem+protected recovery1.0.0.4 path; no Observer or
POD execution configuration was added. Centralalpha/auto=true was preserved.

Only testing temporarily selected existing productchannelobs-test-20260918 in
download mode. Its updater fetched signed .39, archive SHA256
87880575107db69282ee727bd61ef04a33de63196fe407a8de3fb216094522af,
releaseeef75af7-6b99-47c5-b978-108107b64fe9. No release/fleet targeting edits.

Native preflight passed before application execution. Existing archive compose
overlay was absent (no symlink), expected mounts reused from verified .36, recovery
runtime hashes matched. Exact additive policyrevision7->8 preserves old releases,
trust, mounts and transitions, adding only .36->.39 and the signed target.
Candidate SHA9cd577e406de989e0f35a7483fde902f0ed4f225f17cf2c22d0416a0b6fda6f1;
preflight left active transaction unchanged. SiemCore reviewed .36->.39 as Normal
compatible, unchanged volumes, additive SQL201 only and forward-compatible retained
.36 rollback. No direct database operation, backup, copy or restore was performed.

Daemon real-mode cascade reported success08:31:26.495550Z; nativejournal applied.
App and archiver run image425dcc172c1f63109a10be88c99df19c4a5da0b2ebec4474a37ccb3e17ae0f2e.
Original updater config restored byte-for-byte SHA
40f6414c51d61d06c2817d6d8050f18cad2dce73545b2e07ada1d8819711cde9,
productstable/self-updatestabledefault; updater-only restart did not restart app
containers. All captured fleet controls including benchmark hold are unchanged.

SiemCore independently confirmed existing signed-in Normal UI/SSO session retained,
Attack Shield connected/.39, Platform Manager and archive health OK, zero upload
failures. Observer login routes did not replace Normal authentication.

## TLS finding and rollback boundary

SiemCore reports baseline6514 SSL.com fingerprint matched canonical443/18443.
After restart6514 served fingerprint
FB:35:7D:B8:CA:EB:D3:0A:DA:50:0B:25:9E:7A:0A:88:55:16:F4:C0:E0:40:53:ED:5A:33:28:83:10:F3:E5:7B.
Host /opt/siemcore/tls/tls.crt already had that certificate with mtime06:37:20Z,
before08:31 update. Existing /opt/siemcore/tls->/etc/siemcore/tls read-only mount
and SYSLOG_TLS_CERT_FILE path were unchanged. SiemCore diagnosis: restart exposed
pre-existing on-disk certificate drift; no .39 certificate-path defect established.
TLS audit has two failures; app health alone cannot qualify the full deployment.

Exact signed .36 archive remains retained in cache and private transaction storage;
actual predecessor imagebd04a84... remains local. An additional archive-listed image
6ca2ec... is not loaded; native recovery reauthenticates/reloads the retained archive
and uses --pull never rather than depending on current tags. Original app config,
compose/override, node/VERSION and dashboard are retained root-private. Recovery
never restores a DB. No live rollback was attempted, and binary rollback would not
repair the external host certificate. Certificate provisioning requires separate
reviewed remediation; Updates made no certificate or renewal-job change.

## Final limited result

Seven samples over300seconds after config restoration passed application,
PostgreSQL, Redis and archiver checks. App/archiver containers and original updater
config bytes remained unchanged. Origin08:35:27.833980Z reports running.39 onstable,
updater1.16.1.24, alpha/auto=true and successful .36->.39 at08:31:26.646218Z.
All31 fleet control records match before/after, including benchmark auto=false.

**Application/Normal compatibility test: PASS. Full TLS deployment acceptance:
NOT PASSED**, due the independently diagnosed6514 certificate drift. No forced
rollback, certificate repair, DB copy/restore, or broader promotion was performed.
