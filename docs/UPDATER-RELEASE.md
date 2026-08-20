# Updater Kit Release Process

The cascade updater (all tiers) is developed, versioned, and released from
**this repository**. The `siemcore-ai` repo's `siemcore-updater/` directory is
the *retired v2 auto-updater* — a different lineage kept only as the migration
runner reference; nothing is built from it, and its old unit name is what v3
posture audits flag.

## Version anchor

There is exactly one version anchor: the repo root `VERSION` file
(`MAJOR.MINOR.PATCH.BUILD`, per the development cycle). The updater kits carry
the version of the build that produced them — the binary via ldflags
(`main.Version`, shown by `<updater> version`), the config
(`instance.updater_version`), the README title, and a stamp line in every
bundled doc all come from that single anchor at build time. There is no
separate updater version line, and no combined app×updater version: customer
or tier tailoring is a **build target**, never a version axis.

## Building kits

```bash
scripts/build-kits.sh            # both tiers
scripts/build-kits.sh siemcore   # one tier
```

Templates live in `kits/` (tracked); rendered kits land in
`dist/updater-kits/<kit>-<VERSION>/` (untracked artifacts). The script refuses
to rebuild into an existing versioned directory — bump `VERSION` first — and
stamps builds from a dirty tree `-dirty` (diagnostic only, never shippable or
tailorable).

## Tailoring for an operator/customer

Tailoring means pre-filling a rendered kit's `config.yaml` (instance id,
`parent_id`, pinned signing key, relay address) and adding a
`QUICKSTART-<target>.md`. Rules:

- Tailor a **copy** of a rendered kit; never edit templates with
  target-specific values.
- The kit version stays the version of the binary it wraps.
- Secrets (platform keys, enrollment credentials) never travel in a kit.
- After tailoring, regenerate `SHA256SUMS` and record the zip's SHA-256 in the
  handover message.
- The installer prints whichever `CHANGE-ME`/`PASTE-` placeholders remain, so
  a tailored kit automatically shows only what is left to fill.

## Unit naming

| Tier | Unit | Note |
| ---- | ---- | ---- |
| mysoc | `mysoc-updater` | |
| siemcore | `siemcore-cascade-updater` | Renamed in 1.8.1.1; `siemcore-updater` is the retired v2 daemon and is flagged by v3 posture audits. Never mask either unit to silence an audit. |

## Known issue: binaries labeled 1.8.0.1 may not roll up children

The first tailored kits (testing.mysoc.ai pilot) shipped binaries built
mid-development of 1.8.0 and stamped `1.8.0.1` before the relay rollup fixes
were committed. Symptom: children enroll and heartbeat to the relay, but never
appear on the dashboard — the relay's upward heartbeats silently omit them.
Fix: upgrade the relay binary; **minimum relay version for rollup is
1.8.1.1**. This is exactly the unanchored-artifact failure `build-kits.sh`
now prevents (clean-tree requirement + `-dirty` stamping + no rebuilds under
an existing version).

## Release checklist

1. Develop on `version/MAJOR.MINOR.PATCH`; bump `VERSION`.
2. Run the local gate (fmt, vet, tests, builds).
3. `scripts/build-kits.sh` from the clean committed tree; record the commit
   and version.
4. Tailor per target if needed; zip; record SHA-256.
5. Hand over via the approved channel; keys travel separately.
