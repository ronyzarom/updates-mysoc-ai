# Test infrastructure only. Product/runtime images are imported from the local cache.
FROM siemcore-data-runtime-fixture:local AS client
FROM updates-observer-fixture:local
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    docker.io docker-registry curl apache2-utils python3-bcrypt \
    && rm -rf /var/lib/apt/lists/*
COPY --from=client /usr/local/bin/docker /usr/local/bin/docker
COPY --from=client /usr/local/libexec/docker/cli-plugins/docker-compose /usr/local/libexec/docker/cli-plugins/docker-compose

# Loopback-only fixture registry restores immutable references after docker save/load.
COPY registry.fixture.yml /etc/docker/registry/config.yml
