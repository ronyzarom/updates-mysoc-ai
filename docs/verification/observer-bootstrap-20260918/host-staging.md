# Authorized Observer staging

Fresh target verified via GCP metadata: VM8079452056575878166, machine ID
b40908cd5d7a4cf1bf44f45bf98205e9. Access used the product-approved podadmin/IAP
host-preparation route. No direct product installer/hook was executed.

Staged under root-private `/root/observer-provisioning`:

- Fixed kit 1.16.1.26-r1 and signed manifest; manifest signature, archive SHA256,
  binary signature and all package checksums verified on target.
- Pinned parent relay CA (public certificate).
- Exact agreed application identity/management input, mode0600.
- Fresh target-generated enrollment credential, mode0600, never logged/exported.

Preflight found TLS files owned UID501/GID50, inherited from preparation. Changed
only owner/group to root:root; preserved file bytes and modes (certificate0644,
key0600). Kit exact machine/TLS protection validation then passed.

Updater `/etc` config and product greenfield policy remain absent. No service
installation, enrollment or application execution has occurred. Signed release
receipt and product readiness signal are required before installation.
