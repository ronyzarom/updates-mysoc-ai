# nginx: public `/install/` path on updates.mysoc.ai

The one-command fresh install (`deploy/bootstrap/mysoc-enroll.sh`, published by
`scripts/publish-install.sh`) needs an unauthenticated static path so a bare VM
can run:

```bash
curl -fsSL https://updates.mysoc.ai/install/mysoc-enroll.sh | sudo bash -s -- \
  --license-key <KEY> --domain <FQDN> --tls wildcard --cert ... --key ...
```

Release artifact downloads are license-gated (`X-License-Key`), so the bootstrap
hop must be served by nginx as plain static content instead.

## Live layout (confirmed on the box)

- Fronting nginx: `nginx/1.28.0`, config under the Bitnami tree.
- The `updates.mysoc.ai` 443 server block lives in:
  `/opt/bitnami/nginx/conf/server_blocks/updates-mysoc-ai.conf`
- It already serves a static alias with exactly the pattern we need:
  `location ^~ /downloads/ { alias /home/bitnami/updates-mysoc-ai/admin-downloads/; }`

## Apply (DEPLOY-GATED — only on Rony's explicit instruction)

1. On the box, create the public directory:
   ```bash
   mkdir -p /home/bitnami/updates-mysoc-ai/public/install
   ```
2. Add the contents of [`install-location.conf`](install-location.conf) into the
   443 server block in
   `/opt/bitnami/nginx/conf/server_blocks/updates-mysoc-ai.conf`
   (next to the existing `location ^~ /downloads/`, before `location /`).
3. Validate and reload:
   ```bash
   sudo nginx -t && sudo nginx -s reload
   ```
4. Publish the files:
   ```bash
   scripts/publish-install.sh --yes
   ```
5. Verify:
   ```bash
   curl -fsSL https://updates.mysoc.ai/install/mysoc-enroll.sh | head -1
   curl -fsSL https://updates.mysoc.ai/install/mysoc-updater-kit-latest.tar.gz.sha256
   ```

Notes:
- `^~ /install/` takes precedence over regex locations, so it will not be
  shadowed by the `~ ^/(siemcore|mysoc|mysoc-updater)/v…` download regex or the
  Go catch-all. The existing exact-match `location = /install.sh` is unrelated
  (different URI) and can stay.
- Updating the kit later is just `scripts/publish-install.sh --yes` (a file
  drop); no nginx change and no update-server release are needed again.
