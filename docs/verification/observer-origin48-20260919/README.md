# Observer browser Origin correction — 2026-09-19

Signed SiemCore 3.3.152.48 was published at 13:49:19 UTC from source
5297542933d6eb8682d0fd2fc7fe3c5bd2be33b9, release ID
132f18dd-3ae2-4938-85fe-e0d8bc3e8b70. SHA256:
473a6010ec73ddc33698e8f554696c120996ec5c30aa8699109f2e3c7ad9791d;
size 65,419,652 bytes. Updates independently verified all 185 inner checksums,
manifest version/commit and both observer-ui-auth-v1 and
observer-unlinked-update-v1 capabilities. Signature verification passed under
the existing fleet key, including origin catalog readback.

Publication is restricted to obs-test-20260918 and explicit alpha. The guarded
publication asserted that its sole subscriber is
bezeq-pod-test-observer-8079452056575878166, and that fleet controls, other release
metadata and benchmark hold remained unchanged. See publication.json.

SiemCore supplied native Chromium form regression evidence: no-referrer emitted
Origin:null, whereas same-origin emitted the expected origin. The product's
strict origin guard remains enabled. Product Go tests and 14 Python Observer
update tests passed. Product evidence:
siemcore-bezeq-pod/docs/operations/evidence/observer-origin-2026-09-19/regression.json.

Pre-update origin heartbeat confirmed .39 running with updater 1.16.1.33,
alpha/automatic updates. SiemCore independently read the effective local
self-update stable channel, observer-unlinked installation type, correct product
channel and no pending operation via Tailscale. Publication alone is not
installation acceptance; live automatic apply and browser verification follow.

## Live result

SiemCore reports automatic cascade apply/verification at 13:50:38 UTC. Native
browser submission now reaches the normal incorrect-password response; valid
credentials create a secure session with authenticated root 200. Cross-origin
requests remain rejected, logout succeeds and the expired session is rejected.
Health is .48/healthy with unchanged Observer identity, installed-unlinked,
management ready, authority/processing disabled. Product-reported evidence is
in product-live-acceptance.json. No SSH application apply was used.

The origin independently recorded target .48 success and empty error at
13:52:03 UTC. Final installed-version heartbeat is recorded in final-rollup.json.
