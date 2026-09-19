# Exact .47 → .50 Normal ingress qualification

These scripts are for disposable native AMD64 VMs with synthetic identities only.
They are not fleet installers. Never run them on a customer or production host.

The original `.49` product rejected valid address-qualified port settings. Product
helper fix `51f1ebe` is included in immutable `.50`, source `7832d56`.
`native-drill50.py` is product drill `d900059` plus restricted root-only capture
of failed apply subprocess output; no signed runtime file is modified for logging.

Run clean `.47` through signed kit `1.16.1.33-r3` first using the product native
fixture runner. `prepare-fixture.py` retains its synthetic Ed25519 private key
under root0600 solely so the same fixture can test the next signed delivery.
Never commit that key or substitute it into a real registration.

`run-native50.py` binds exact disposable VM IDs and runtime hashes, then runs the
direct or interrupted-pin case. These passed on separate fresh VMs. Receipts are
in `docs/verification/setup49-eligibility-20260919/ingress-*-result.json`.
Each tests signed preflight, apply/retry, rollback twice, a real port collision
and automatic recovery, retained endpoints, and final target health.

`cascade50-fixture.py` tests the real systemd updater using a loopback TLS parent,
the original fixture bootstrap trust, and exact `.50` bytes. The prerequisite
runtime and hook must be installed first. It signs a measured fixture-only port
retention authorization and requires successful report, Normal identity, exact
endpoints, applied recovery journal, and healthy target. The updater is stopped
when the loopback parent closes. This integration case is pending execution.

No test clones initialized disks, copies databases, changes real fleet assignments,
or grants POD authority. Product publication and live rollout require separate
review of these results and effective channel/hold eligibility.
