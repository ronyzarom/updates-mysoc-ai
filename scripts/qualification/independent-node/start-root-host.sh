#!/usr/bin/env bash
# Isolated test host: real systemd, nested Docker and loopback-only fixture registry.
set -euo pipefail
name=${1:?supply a unique updates-node-root-* container name}
[[ "$name" =~ ^updates-node-root-[a-z0-9-]+$ ]] || { echo 'invalid fixture name' >&2; exit 1; }
image=updates-node-root-fixture:local
docker image inspect "$image" >/dev/null
if docker inspect "$name" >/dev/null 2>&1; then echo 'fixture already exists; refusing reuse' >&2; exit 1; fi
created=false
cleanup_failed() { if [[ "$created" == true ]]; then docker rm -fv "$name" >/dev/null; fi; }
trap cleanup_failed EXIT
docker run -d --pull never --name "$name" --label updates.independent-node-fixture=true \
  --network none --privileged --cgroupns private --tmpfs /run --tmpfs /run/lock \
  --entrypoint /sbin/init "$image" >/dev/null
created=true
ready=false
for attempt in {1..120}; do
  if docker exec "$name" sh -c 'docker info >/dev/null 2>&1 && curl --max-time 2 --fail --silent http://127.0.0.1:5000/v2/ >/dev/null' 2>/dev/null; then ready=true; break; fi
  sleep 0.25
done
if [[ "$ready" != true ]]; then
  docker exec "$name" journalctl -u docker -u docker-registry --no-pager -n 40 >&2
  exit 1
fi
trap - EXIT
printf 'Ready: %s (no outer network, no host mounts/socket)\n' "$name"
printf 'Cleanup after testing: docker --context %s rm -fv %s\n' "$(docker context show)" "$name"
