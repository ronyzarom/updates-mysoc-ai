# Hierarchy cleanup

The instance detail page provides an admin-only **Delete inactive entries** action. It previews up to 1,000 eligible direct children and allows select-all or individual selection before confirmation.

Eligibility is rechecked server-side at confirmation: offline, last heartbeat (or creation if never seen) more than one hour ago, automatic updates enabled (explicit holds excluded), and no non-decommissioned children. IDs from another parent are never affected. Nodes that reconnect between preview and confirmation are skipped.

Removal retains the existing record/history as a decommission tombstone. The hierarchy uses the existing tree endpoint, which hides those records. A stale or unstamped cached relay report cannot revive a removed entry; a genuine later heartbeat can. No uninstall is performed and no database migration is required.

Validation: all Go packages with disposable PostgreSQL fixtures, dashboard build and 29 tests. The fixture covers preview eligibility, hold/active/parent exclusions, cross-parent input, reconnect races, stale report suppression, unstamped report suppression and genuine revival.

Server/dashboard release: 1.16.1.15. This change does not publish a new fleet updater; alpha updater/relay remains 1.16.1.14. Deployment verification opens and cancels the review dialog; no customer records are removed automatically.

Live API verification on 2026-09-15: server health reports 1.16.1.15. Admin preview on testing.mysoc.ai returns HTTP 200 and 37 candidates; the same unauthenticated request returns 401. No cleanup POST was executed against the live fleet.
