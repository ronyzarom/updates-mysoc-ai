# Scoped Node A production-signature preparation

Update at 2026-09-19 06:36:45 UTC: after SiemCore confirmed the user's isolated
Node A security acceptance and authorized scoped publication, exact `.47` was
published on `normal-a-20260919`, with explicit target groups `['alpha']`.
Read-back signature and digest passed. All existing fleet controls and other
release metadata were unchanged. See `product47-publication.json`. Host
installation remains SiemCore-owned; publication alone does not prove install.

The following records the earlier preparation and checks:

Prepared at SiemCore's user-authorized request to finish fresh Normal Node A.
No host enrollment, release catalog mutation, publication or fleet change made.

Exact qualified kit `1.16.1.33-r3` retained:
`337e14b9c15b4c2d8d27af4992b9e47cdb71278f20203b21fcbddd8c7026b9f3`.
Local distribution folder: `/tmp/normal-production-admin-1.16.1.33-r3`.
Its new outer manifest contains only the SiemCore kit and is signed by the
existing origin key; no archive rebuild and no fixture key substitution.

Exact SiemCore `3.3.152.47` retained:
`7cf8b7b0011344ca8f568bb023d6bd69e0f01fc9dbd3e14271727da0b8172812`,
65,415,525 bytes, manifest `build.git_commit`
`c2fcb246c3aec96088d815531b85581587dff8b4`, capability
`normal-prerequisites-v1`. Origin-signed receipt:
`/tmp/siemcore-3.3.152.47-signed-receipt.json`.
Both signatures verified against independently pinned origin public key
`1f1aa11a80d6ac549a26bb25daac4798c42dd469138680e68f89832bd32e7f57`.
The signing seed stayed on the origin.

Read-only parent checks:

- `https://testing.mysoc.ai:18443` passes system TLS trust and hostname
  verification. SSL.com issuing CA; certificate expiry 2027-01-13.
- Parent service active, configured listener `:18443`, explicit SSL.com files,
  identity `mysoc-testing-mysoc-ai`, fresh central heartbeat, alpha/automatic.
- Unknown-source `/health` returns relay JSON `unknown source: enroll via
  heartbeat first`. This is the relay guard, not evidence of Cloudflare failure.
- Parent license `07c449bf-f963-4a45-9919-44123f71fcc0` is active through
  2027-08-19; operator/customer reference `testing-mysoc-ai`.
- New updater identity `siemcore-normal-db91d16e-97a5-452d-ae54-6db5c6d8f3bf`
  is absent from the central registry, as expected before installation.
  Do not copy the parent's credential: child enrollment uses its own protected
  local credential and receives a relay token at first heartbeat.
- Benchmark remains alpha with automatic updates disabled.

Publication hazard discovered before any upload: `Repository.Create` defaults
an empty target-group list to **all rings**. An empty-group POST is not safe
held staging. Do not create broadly and then race to clear targeting.

Existing scoped mechanism, only after SiemCore's live security/prerequisite gate:
explicit `alpha` target on the exclusive **product** channel
`normal-a-20260919`, with only fresh Node A configured to subscribe. No reported
subscribers were found at this check; recheck actual effective configurations
and current origin data before activation. Channel isolation is not a per-host
authorization mechanism and historical heartbeat data cannot prove the absence
of all dormant subscribers. Leave the global stable product releases and other
fleet settings unchanged. Updater self-update remains stable/alpha.

If held catalog publication is needed before those checks, implement and test
atomic held creation first. No such new server behavior is claimed deployed.

SiemCore still owns live prerequisite/security disposition, real SSL.com inputs,
schema3 settings under service-visible protected `/etc` paths, and final
application qualification. Local production signatures alone do not authorize
bypassing those gates or prove installation.
