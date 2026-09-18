#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/../../.." && pwd)
PRODUCT=${1:?exact SiemCore source path required}
IMAGE=siemcore-observer-fixture:local
NAME="updates-observer-integration-$$"
docker image inspect "$IMAGE" >/dev/null
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true' EXIT
for mode in ${OBSERVER_FIXTURE_CASES:-success health-failure interrupted installed-cli}; do
 docker run -d --pull never --name "$NAME" --network none --privileged --cgroupns=private --tmpfs /run --tmpfs /run/lock "$IMAGE" >/dev/null
 docker exec "$NAME" mkdir -p /fixture/adapter /fixture/product
 docker cp "$ROOT/deploy/observer-update-v1/." "$NAME:/fixture/adapter/"
 docker cp "$PRODUCT/deploy/cluster/updater/." "$NAME:/fixture/product/"
 docker cp "$ROOT/scripts/qualification/observer-unlinked-update/integration_fixture.py" "$NAME:/fixture/run.py"
 docker exec "$NAME" sh -c 'touch /fixture/ISOLATED_CONTAINER; chown -R root:root /fixture; chmod -R go-w /fixture'
 docker exec "$NAME" python3 /fixture/run.py "$mode"
 docker rm -f "$NAME" >/dev/null
 done
