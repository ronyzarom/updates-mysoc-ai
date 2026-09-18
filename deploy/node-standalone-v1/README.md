# Independent node standalone transition

This opt-in root component enables one signed transition from independent management to standalone customer processing. Normal and Observer installations keep their existing paths. The immutable POD node identity and original bootstrap receipts remain unchanged.

The coordinator validates protected source history, signed artifacts, exact configuration inventory and durable operation identity before invoking the verified product worker. Acceptance requires measured application health, image/binary identity, retained database identities, archive readiness, disabled paid AI and exact MySoc registration. Interrupted operations reconcile or recover using the same operation; a restored operation is terminal. Routine subsequent standalone upgrades remain disabled pending their separate qualification.

`QUALIFICATION.json` and `docs/verification/node-a-standalone-20260918/native-product.json` record disposable native product qualification, including synthetic ingestion/archive retrieval, checksums and interrupted recovery. They do not assert live A deployment.

`prerequisite_install.py` installs only the fixed root component and updater configuration through a signed host-bound kit. Enrollment generates local encryption material and exports public hashes. Installation accepts an expiring signed encrypted capsule containing the two approved customer inputs. Secrets do not appear in GCP metadata. Product application remains exclusively through cascade.

Run the standalone configuration, transaction, root, adapter, capsule and prerequisite unittest modules under `scripts.tests`; run `go test ./pkg/updatersim` for dispatch and compatibility coverage. The prerequisite tests mock service management; live GCP startup and cascade acceptance must be verified separately.
