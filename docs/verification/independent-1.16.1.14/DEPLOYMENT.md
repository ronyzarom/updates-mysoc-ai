# Alpha deployment — 1.16.1.14

2026-09-15, source cc128ee1d6b1851b45b61c217ee4e19ab0915e10.

- SiemCore released the API restart hold after .21 upload, readback and signature verification completed. Its release and host settings were preserved.
- First server activation failed because the service DB role did not own the releases table. Health rollback restored .13. Migration 016 was then applied in one transaction using the database owner, including the exact migration checksum in the ledger. No database was copied or restored, no release rows rewritten, and no ownership grants changed.
- API healthy on .14; server binary SHA-256 f62bb5d3d72db9a0dccfb373f2d82be6c497b3ba9aaa7e45e4310a0cd8a7d3d2.
- Dashboard built on server, activated with prior build retained. Browser verified Bootstrap and Update selections each show one artifact file plus metadata, without a companion requirement.
- Updater product updater-linux-amd64, version 1.16.1.14, channel stable, target_groups=[alpha]. Release ID f69d8b72-6955-4d0f-a05e-ef7d0fa31543. Published 10:19:25 UTC. Size 10191032; SHA-256 82e0ff5aae76b1d8db292b6e71f4b12373a49b74be1f02d0001df65b59d85c8b. Readback signature independently verified using the existing Ed25519 trust root.
- testing.mysoc.ai automatically detected .14 at 10:20:08 UTC, downloaded and verified it, restarted at 10:20:20 UTC, and reported .14 by central heartbeat. Running process SHA-256 matches the publication. Relay TLS and signature verification remain enabled. MySoc product remains 1.3.19.2; HTTPS returns 200.
- Existing product releases, channel assignments and maintenance holds were not changed. Independent product qualification is still alpha/dedicated-channel scoped. Real clean/existing-host installer acceptance remains owned by the individual product teams.

Remote rollback files: qualification/1.16.1.14/previous-update-server and previous-dashboard.conf under /home/bitnami/updates-mysoc-ai. The additive migration can remain during application rollback; do not restore the old unique constraint after independent records exist.
