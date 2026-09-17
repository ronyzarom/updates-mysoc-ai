# Admin installation package repository

Live entry point: https://updates.mysoc.ai/downloads/

Initial packages: MySoc and SiemCore Linux amd64, updater 1.16.1.24,
package revision r1. SiemCore supports standalone and pod A/B/witness through
product-provided input JSON. No credentials are included. MySoc requires its
product provisioning/apply hook before service start. ARM64 and Windows/SWF
are not included in this publication.

The packages reuse the exact signed alpha updater binary (SHA256
65f5ef057759582544174ebe438f838c96a61c0aa21e3014481a0fdc29c27dfc).
They are installation artifacts, separate from automatic release selection;
publishing them does not change fleet assignments, holds or release targets.

## Packaging and maintenance

`scripts/packaging/admin_repository.py` takes `--binary`, `--receipt`,
`--provisioning-source`, `--package-revision` (for example `r2`) and `--output`.
It reuses an existing published updater binary and copies the matching hook from
committed, clean SiemCore source; `PROVISIONING_COMMIT` records that source.
It adds role and clean-install parameter documentation, checksums and a manifest.
The original r1 packages used the 1.16.1.23 provisioning kit. New schema 3 packages
must not reuse that historical hook; the script no longer accepts
`--provisioning-kit`. Both the chosen updater binary and product hook must be
qualified for the advertised installation types before publication.

Do not overwrite published versioned archives. Use a new package revision for
packaging changes, even when the updater binary remains unchanged. Extend the
catalog while retaining earlier package URLs. Do not rebuild an already-published
updater version. Future architectures require their own qualified signed binary.

`manifest.sig` is base64 Ed25519 over the bytes
`mysoc-installation-repository-v1\n` followed by exact manifest.json bytes.
Signing uses the existing server RELEASE_SIGNING_SEED without exporting it.
The public key must be independently pinned by the admin. See the downloadable
VERIFY.txt and verify-download.py. Manifest signing is separate from release
publication and uses a distinct domain; it cannot be used as a release signature.

Nginx location configuration: deploy/downloads/nginx-location.conf.
Hosted directory: /home/bitnami/updates-mysoc-ai/admin-downloads/.
Only GET/HEAD are allowed, directory listing is disabled, and HTTPS is used.
The repository contains distributable software and public documentation only.
Validate nginx configuration before reloading. Activation backup of the original
vhost is updates-mysoc-ai.conf.before-admin-downloads. No app/API restart needed.

Validation: archive/member checksums, installer syntax/help, signed manifest,
full HTTPS downloads of both archives, page and documentation responses, and
unchanged origin /health. This verifies package delivery, not fresh-host product
qualification; product prerequisite and full-install acceptance gates still apply.
