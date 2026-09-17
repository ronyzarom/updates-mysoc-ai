# Fresh ten-stage native qualification

2026-09-17 disposable local fixture. All ten requests returned exit0 on their first
attempt: two runtimes, two schemas, selected seed, data-preserving runtime retry,
two read-only data observations and two paused app/archiver observations.

Current full-source arm64 image:
`sha256:b5dd92045651b83a4e7aafcbd085bd7ced244d68af61e46493c77e1ce2ee0dbc`.
Test-signed artifact SHA256:
`7491046e1349648081555c4f6be0b72dfd3e6a5483e2e5ea381076b92effee2b`.
Unlike the preceding eight-stage run, this binary includes the product's internal
five-second observation deadline. Updates independently enforces receipt freshness.

Management receipts9/10 are1627decodedbytes each, below8192. Each reports exact
originaloperation/node/generation/input/registry/artifact/version, frozen raw config
hash and measured app+archiver identities. Both runtimes have managementready=true,
processing=false, quiescent=true, blocked=false, generation0 and Mirror-STBY.
All installation_complete/processing_allowed/activation_ready flags remain false.
Responses.json retains all decoded receipts/bytecounts. Product-native.txt records
all stages and final barrier-present/no-assignment assertions.

Updates independently authenticated the retained archive host binary, derived
expected environment digests from manifest-bound immutable image defaults plus
protected reviewed overrides (not running-container environment), checked all eight
credential materialization references for exact original bytes and nonpath binding
preservation, and rechecked host binary plus management/data config hashes before
and after bounded execution. No Docker socket was exposed to app containers.

The Updates orchestrator exited0 and was removed automatically. Exactly five stopped
Updates data/readiness command containers were removed after evidence collection.
Product owns test app/archiver/dependency cleanup; no further Updates process needs
the fixture. No cloud VM, production database, public release, live kit or activation
changed.

This proves actual management observation transport using real common-image
supervisors, in addition to the prior runtime/data checks. Test supervisors use
reviewed minimal fixture environments; this is not proof of the complete installer,
production DB/TLS/archive/LLM configuration, peer controller, cloud routing, active
permission, production signing trust or full customer readiness. Those acceptance
gates remain open. Historical observations expire after5seconds and cannot be used
as permanent activation evidence.
