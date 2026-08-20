# Updater kit templates

Source-of-truth templates for the production updater kits. `scripts/build-kits.sh`
renders these into `dist/updater-kits/<kit>-<VERSION>/`, stamping every
`@VERSION@` placeholder — and the updater binary itself via ldflags — from the
repo's single `VERSION` anchor, so the binary, config, README, and bundled docs
can never drift apart.

| Template dir | Kit | Unit name |
| ------------ | --- | --------- |
| `mysoc/` | Tier 1 — mysoc platform updater (talks to updates.mysoc.ai, relays to siemcore) | `mysoc-updater` |
| `siemcore/` | Tier 2 — siemcore cascade updater (talks to the mysoc relay, relays to swf) | `siemcore-cascade-updater` (renamed from `siemcore-updater` to avoid the retired v2 daemon's name in posture audits) |

Rules:

- Never edit rendered kits under `dist/` by hand; change the template and rebuild.
- Never rebuild into an existing versioned kit directory — bump `VERSION` first
  (the build script enforces this).
- Customer/tier tailoring (pre-filling `parent_id`, signing key, instance id)
  happens on a rendered copy and is a build target, not a version: a tailored
  kit keeps the version of the binary it wraps.

See `docs/UPDATER-RELEASE.md` for the release process.
