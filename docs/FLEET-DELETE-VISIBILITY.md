# Deleted fleet visibility — 1.16.1.19

The previous retirement fix prevented stale reports from recreating identities but left deleted rows visible in All statuses. Decommissioned status and deletion now have distinct meanings: optional instances.deleted_at records explicit deletion, while status continues to represent lifecycle.

Delete and inactive-child cleanup retain the identity and mark deleted_at. Fleet list, totals, customer directory, tree/subtree and security views exclude these rows even when decommissioned entries are included. Detail lookup remains available for retained history. Stale reports cannot revive the row; a verified newer online/degraded report or direct host heartbeat can reactivate it. Relay freshness compares against both retirement and deletion timestamps. Repeated deletion preserves both markers. Migration 017 adds one nullable column without rewriting existing records.

Validation: PostgreSQL fixture tests cover list/tree/count exclusion, all-status/decommissioned filtering, repeated deletion, stale/undated reports and genuine heartbeat reactivation. API, licensing and migration tests pass; 31 dashboard tests pass. Sources c294bb8 and 2a35e76. Final server binary SHA256 30cb46c2e861f2a02119d3c07cdbc49ee40029cc50a4265bbf335b6a183dcf62.

Live API 1.16.1.19 is healthy. The exact DANAGISDC3 and DANAGISDC4 identities from the screenshot were verified to retain their September 10 source heartbeat times, then deleted twice via the existing authenticated API. All requests returned 200. Their internal markers remain, with zero visible records. Browser verification on All statuses showed total 21 (previously 23), decommissioned 6 (previously 8), online 15 and offline 0; neither deleted identity remained in the displayed fleet. Other instances and host services were unchanged.

Browser testing caught an existing empty-list response issue: a nil Go slice serialized as null, which broke infinite pagination and left the search loading. Empty list responses now serialize items as [], with a PostgreSQL regression assertion. Dashboard production build passed and 1.16.1.19 was deployed.

Final browser verification after the empty-array fix: searching DANAGIS with All statuses selected displayed "No matching instances". Search was cleared and the fleet page left open.
