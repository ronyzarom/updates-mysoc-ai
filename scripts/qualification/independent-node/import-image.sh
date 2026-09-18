#!/usr/bin/env bash
# Import existing cached bytes, then establish a fixture-only immutable digest.
set -euo pipefail
host=${1:?root fixture name required}
source=${2:?locally cached image reference required}
alias=${3:?fixture image alias required}
[[ "$host" =~ ^updates-node-root-[a-z0-9-]+$ && "$alias" =~ ^[a-z0-9][a-z0-9-]*$ ]] || exit 1
[[ $(docker inspect "$host" --format '{{index .Config.Labels "updates.independent-node-fixture"}}') == true ]] || exit 1
source_context=${SOURCE_DOCKER_CONTEXT:-$(docker context show)}
docker --context "$source_context" image inspect "$source" >/dev/null
archive=$(mktemp)
trap 'rm -f "$archive"' EXIT
docker --context "$source_context" image save "$source" -o "$archive"
# Engine image IDs differ for containerd/legacy stores. Verify the saved config
# digest, rather than confusing a source manifest/index digest with a config ID.
source_id=$(python3 - "$archive" <<'PYCODE'
import hashlib, json, sys, tarfile
with tarfile.open(sys.argv[1]) as archive:
    manifest=json.load(archive.extractfile('manifest.json'))
    if len(manifest) != 1:
        raise SystemExit('expected exactly one saved image')
    config=archive.extractfile(manifest[0]['Config']).read()
    print('sha256:'+hashlib.sha256(config).hexdigest())
PYCODE
)
docker exec -i "$host" docker load < "$archive" >/dev/null
target="localhost:5000/fixture/$alias:imported"
docker exec "$host" docker tag "$source_id" "$target"
docker exec "$host" docker push "$target" >/dev/null
actual_id=$(docker exec "$host" docker image inspect "$target" --format '{{.Id}}')
[[ "$actual_id" == "$source_id" ]] || { echo 'imported image identity changed' >&2; exit 1; }
reference=$(docker exec "$host" docker image inspect "$target" --format '{{index .RepoDigests 0}}')
[[ "$reference" == "localhost:5000/fixture/$alias@sha256:"* ]] || exit 1
printf '%s\n' "$reference"
