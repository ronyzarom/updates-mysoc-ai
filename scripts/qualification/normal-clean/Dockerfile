FROM --platform=linux/amd64 docker:28-cli@sha256:625d9431a9f54c5a2bc90f24f0e1c3d55b1349fd857dd85035f98c2c9acbdd4d AS client
FROM --platform=linux/amd64 debian:bookworm-slim@sha256:88200866dfff7ea7f5cbcb6ec7c8a701889efe6fe859fe64d6990e4b07ea4171
ENV container=docker
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends systemd systemd-sysv dbus docker.io docker-registry python3 python3-yaml python3-cryptography sudo curl openssl ca-certificates jq rsync iproute2 uuid-runtime && rm -rf /var/lib/apt/lists/*
COPY --from=client /usr/local/bin/docker /usr/local/bin/docker
COPY --from=client /usr/local/libexec/docker/cli-plugins/docker-compose /usr/local/libexec/docker/cli-plugins/docker-compose
RUN systemctl enable docker && mkdir -p /etc/docker && printf '{"storage-driver":"vfs"}\n' > /etc/docker/daemon.json
CMD ["/sbin/init"]
