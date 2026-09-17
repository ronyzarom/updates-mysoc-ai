# Full application installer fixture — 2026-09-17

The twelve-stage source-only fixture completed successfully using the retained
signed `.99` common artifact. `responses.json` preserves every original response
and both retries. This is local fixture evidence, not deployment.

Artifact SHA256: `8b1daf97b825b0edc81091b8ad9683e5d89577646256ee28a92a48e9d055307c`.
Common image: `sha256:020cf32cb1d49eaca172d048b63d0e07f80dbd6ace1688d4d5ad5f973b844f4c`.

Stages1–8 (both data runtimes, schema, selective seed, runtime retry, and fresh
readiness observations) passed on the initial attempt. Application1 stage9 first
failed in the older Updates runner image before its application directory was
created. The same operation resumed in the prepared product fixture host; the
actual signed application installer passed. Management1 stage10 first failed
because jsonschema package preparation had not completed. After package import
and removal of temporary fixture preparation egress were verified, management1
passed without reinstalling application1. Application2 and management2 then
passed. Original failures were never overwritten.

Successful stages9/11 are only `management-installed-paused` partial receipts.
Stages10/12 independently verify the actual common-image app and archiver,
reviewed environment hashes, paused generation-zero state and original binding.
All installation/activation/processing completion flags remain false.

The product fixture uses real Compose, installer, data services, and management
observations. Kernel and Docker host-prerequisite responses are explicitly
controlled fixture responses; this does NOT establish real VM host acceptance.
This run also does not qualify interrupted application installation after a
partial mutation, final activation, or customer routing. No cloud rebuild,
production database operation, live-kit publication or rollout occurred.

## Same-operation retry and lost completion

Stages13/14 repeat the actual node1 installer and independently observe management;
both passed. Product reports exact journal/environment/TLS-key hashes, all four
data-container IDs, raw-log exclusion and disabled connectors preserved.

Stages15–17 explicitly inject SIGKILL in the qualification worker after the signed
installer returns and authorization/bundle checks pass, but before receipt
transmission. Stage15 fails as expected and retains `status=incomplete` with
`potential_partial_commit=true`. Stage16 repeats the unchanged operation and
installer successfully; stage17 independently observes paused management.
The fault hook exists only in the qualification runner, not in the signed product.
`lost-completion.json` records expected exit codes1/0/0. This qualifies recovery
from a committed installation with lost completion; it does not cover every
possible mid-installation or host-power-loss point.
