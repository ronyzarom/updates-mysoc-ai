# Independent node root-host fixture

This is disposable local test infrastructure, not a product installer or release.
It runs real systemd and a nested Docker daemon, so protected installer paths
such as `/opt/siemcore-node-unlinked-1` are identical in the root process and
Docker mount operations. No monkeypatched runtime path mapping is required.

The outer container is privileged for nested Docker/systemd, with no network,
no host Docker socket, no host mounts, and no published ports. Copy reviewed
source and disposable fixture inputs with `docker cp`. Never copy fleet keys,
customer credentials, or customer databases. Install a disposable test CA inside
the fixture when testing HTTPS.

## Prepare

The base images must already exist locally. Building infrastructure installs
Debian Docker/registry/test utilities. Product execution itself has no external
network and must use cached signed/pinned dependencies with pulls disabled.

```sh
docker build --pull=false -t updates-node-root-fixture:local scripts/qualification/independent-node
scripts/qualification/independent-node/start-root-host.sh updates-node-root-example
scripts/qualification/independent-node/import-image.sh updates-node-root-example LOCAL_IMAGE_REFERENCE fixture-alias
```

The import helper exports a previously cached image, checks its configuration
SHA256 after import, and establishes an immutable reference in the root host's
loopback-only registry. Use the printed `localhost:5000/fixture/...@sha256:...`
reference in disposable product settings. These are **fixture digests**, not
claims that upstream manifest digests survive export/import. Containerd-backed
outer image IDs and classic nested Docker image IDs refer to different objects;
the saved image configuration digest is the comparison boundary.

For capacity isolation, set up a dedicated local Colima profile without changing
the user's active context:

```sh
colima start updates-node-root-20260918 --activate=false --cpu 2 --memory 4 --disk 30 --runtime docker
# Transfer only the root fixture infrastructure image from the existing cache.
docker --context colima save updates-node-root-fixture:local | docker --context colima-updates-node-root-20260918 load
DOCKER_CONTEXT=colima-updates-node-root-20260918 scripts/qualification/independent-node/start-root-host.sh updates-node-root-e2e
DOCKER_CONTEXT=colima-updates-node-root-20260918 SOURCE_DOCKER_CONTEXT=colima scripts/qualification/independent-node/import-image.sh updates-node-root-e2e LOCAL_IMAGE_REFERENCE fixture-alias
```

Always specify the destination context for follow-up exec/cp/remove commands.
Use distinct outer roots for node1 and node2; do not delete one node's data to
simulate another node. Each root can have its own nested port443.

## Qualification boundary

Fixture infrastructure was verified on 2026-09-18: systemd/nested Docker active,
image configuration preserved, loopback digest lookup successful, and a real
nested container read a marker from the root host's exact `/opt` bind path.
Outer inspection reported network `none`, zero mounts and privileged mode.
The shared default Docker VM was not restarted or pruned; a separate profile
was created because its disk had insufficient space.

This does not establish product installation success. Run SiemCore's reviewed
signed fixture through the original root hook with a complete schema5 envelope,
disposable signing trust and TLS/login checks. Require exact artifact/version/
identity health, inactive processing and preserved retry data for both nodes.

After both teams collect evidence, remove only owned fixture containers with
`docker --context CONTEXT rm -fv NAME`. Stop the dedicated Colima profile when
no fixture work remains. Do not prune the shared Docker environment.
