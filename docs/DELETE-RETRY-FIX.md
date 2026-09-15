# Deleted instances remained in cached fleet lists

Server logs confirmed that instance 24b702a7-b3bf-437d-8a28-f64f1021a032 was deleted successfully at 10:31:32 UTC on 2026-09-15, followed by a repeated DELETE at 10:31:36 that returned 404.

The detail page invalidated `instances`, while the paged fleet table uses `instances-list` and the tree uses `instance-tree`. The fix refreshes all instance/fleet query families, evicts the deleted detail, closes the confirmation, and returns to the fleet without waiting for background requests. A route change closes the confirmation; deletion is bound to the displayed record ID. Bulk cleanup uses the same cache refresh helper.

DELETE now succeeds if the record is already absent. Other database errors remain errors. Existing authorization is unchanged.

Validation: API tests cover success, missing record, wrapped missing-record error and database failure. A real QueryClient regression test verifies detail eviction and invalidation of the actual list/tree/hierarchy/count keys. All 30 dashboard tests and the production build pass. On live server 1.16.1.16, two retries against the verified-absent ID both returned 200; no existing record was deleted during verification. Fleet updater/relay remains 1.16.1.14.
