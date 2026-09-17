# Observer store source review

2026-09-17. Source-only review of SiemCore `internal/pod/observer_maintenance.go`; no live changes or independent rerun of product native tests.

1. **Completed status must survive later legitimate operations.** ObserverStatus currently applies observerBaselineCurrent and observerChecks even after completed. Completion releases maintenance/update-prepared, allowing legitimate data updates or assignment history to change. An Updates process that lost the completion response then cannot read its completed operation because the frozen baseline no longer matches. Persist preservation/completion evidence atomically at completion, and make terminal read-back prove that historical result without requiring all current history to remain frozen. Fresh service acceptance is a separate check. Add lost-completion followed by another legitimate pod operation, then old-operation status/reconciliation.
2. **Bound actual transaction size.** Prepare/observerChecks add roughly three comparisons per baseline entry plus fixed checks. The 1024-entry/256-KiB record bounds do not by themselves prove the transaction fits the configured etcd transaction-operation limit. Qualify the deployed limit, enforce a compatible bound with an explicit prerequisite error, or adopt a safe revision-check strategy that handles complete history. This is an availability/qualification concern; refusal must retain protection and never reset history.

The separate witness binding, no-TTL shared prepared guard, prepare-before-start ordering, baseline digest, original assignment/history comparisons and expiry retention otherwise match the proposed updater boundary. Product's tests are reported as passing with real etcd; full local service executor and signed runtime qualification remain pending.

## Follow-up source verification

SiemCore addressed both findings. Reinspection confirms completion now atomically
stores AuthorityPreserved and completed ObserverStatus returns that historical
receipt without demanding unchanged live history. Baseline entry limit is now 24
with the existing byte bound. Product added later-assignment/history reconciliation
and boundary tests; their final native results remain product-reported evidence.
No remaining blocker identified in this limited store review. Local adapter,
service executor and joint runtime qualification are still required.
