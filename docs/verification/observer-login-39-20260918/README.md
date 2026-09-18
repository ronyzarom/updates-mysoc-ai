# Automatic Observer .38 -> .39 / branded login acceptance

SiemCore published release eef75af7-6b99-47c5-b978-108107b64fe9 from source b525e29,
channel obs-test-20260918, exactly alpha. Common archive SHA256
87880575107db69282ee727bd61ef04a33de63196fe407a8de3fb216094522af.
Updates changed no assignments, holds, configuration or application files.
Existing updater1.16.1.27 automatically applied .39 at08:20:22.553648Z.

Accepted operation ac7cda0f-f96c-4d57-b8f8-35835677ef63, binding hash
b438d6594d3cff1132ce2ae30a13e01358ba9ad2a4857940527fd1db583a4cf3,
chains to prior accepted .38 operation5b9fce01-a90d-4c25-977f-ea6bffd999ea.
Product binary SHA2567ac4774e5421c78021d1b09eced00ac18b3f99fd2eff54b56ddbbae7096bf1c3.

Live HTTPS acceptance (no certificate bypass):
- Anonymous root401 contains HTML login form and no WWW-Authenticate header.
- Login page and branded SVG return200; wrong password401.
- Valid form login303 creates Secure/HttpOnly/SameSite=Strict host-only cookie,
  Path=/, Max-Age28800; session-authenticated root200.
- Cross-origin logout403; legitimate logout303; old session rejected401.
- Exact .39 health, unchanged installation/updater identities, installed-unlinked,
  management ready, authority/processing/pod_ready false.

Only the test's own session was created/logged out; credentials and cookies were
neither printed nor written into evidence. Reusable check is
scripts/qualification/observer-unlinked-update/verify_live_login.py.

SiemCore separately verified the live root/login page visually in the Codex
internal browser: persistent branded form, violet standard mark and no Basic
popup/browser error. Updates' checks above verify the HTTP/session contract.

Final result: all seven stability samples at0/50/100/150/200/250/300 seconds passed.
Product PID11590 and updater PID10338 remained unchanged and active; original
bootstrap and accepted operation journals were unchanged. Machine health remained
exact .39 and root remained401 without a Basic challenge. Central heartbeat
08:22:24.203961Z reports updater1.16.1.27 and product3.3.152.39/running; successful
.38 -> .39 report at08:20:22.625581Z; alpha and automatic updates remain enabled.

Updates independently verified the persisted release identity, exact isolated
alpha target/channel and Ed25519 signature under the existing pinned fleet key.
See signed-release.json, alpha-rollup.json and stability.jsonl. No wider rollout.
