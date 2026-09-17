# Archive readiness input review — 2026-09-17

Reviewed product contract `pod-archive-readiness-v1` and its Go parser. Exact
original archive-policy bytes and selected file-credential bytes are digest-bound;
file authentication never falls back to ADC, explicit ADC rejects file overrides,
and production refuses the storage emulator override. Scope buckets must match
installed policy. These rules align with the Updates installation boundary.

Independently ran the focused product test
`go test ./internal/podcontroller -run '^TestArchiveReadinessBindsInstalledPolicy$' -count=1`:
PASS. This is policy binding coverage, not real GCP or full listener acceptance.

Integration remains pending: accepted application module config currently lacks
the new readiness-file path/hash. Its configuration digest binds archive policy
and credentials but not this new scope file. The product must define a provisioning
entry point/extension binding exact readiness bytes and approved tenant scopes to
the original node operation before execution. Do not rewrite already registered
inputs or silently extend retained journal identity. A new candidate fixture can
include the extension from the outset.

The listener must strictly decode the exact schema with bounded file/scope size,
reject unknown/duplicate keys, require root-owned0600 with protected paths, and
fail on changed/missing selected file credentials. Scope approval comes from
product tenant layout, not observations or arbitrary request assertions. Each
data node uses its installed identity; Observer receives bounded observations
and no credentials. Provisioning and actual GCP runtime acceptance remain gates.

## Opt-in product installer v2 review

Reviewed `pod-application-install-v2`: v1 fields plus explicit protected profile
path and digest; exact profile bytes join the configuration's hashed assets
before journal creation, and the installed root0600 copy must remain identical
on retry. Existing v1 field/digest paths are unchanged. Independently passed all
six product Python module tests with real Compose and focused Go archive readiness
tests. A1201-byte UTF-8 prefix was correctly rejected by the current source
(the product fixed byte-count validation during review).

Compatible for a fresh candidate; not qualified by the existing v1 native run.
Updates runner remains v1 until original-registration fields for the fresh v2
candidate are agreed and bound. No retrofit of existing inputs/journals and no
live/schema4 enablement. Native v2 installation, retry and installed listener
integration remain required.
