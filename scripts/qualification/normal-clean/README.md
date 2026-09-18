# Normal clean kit qualification harness

Work in progress; no passing Normal clean product claim yet.

This fixture invokes the unmodified signed kit installer, systemd updater and
product root apply hook. It uses synthetic updater/MySoc/customer identities and
loopback TLS/API delivery, with an exact signed product artifact. There are no
host mounts/socket or published ports. Require a native Linux amd64 fixture host.
The Dockerfile installs **test infrastructure only**, never application prerequisites.

The local arm64/emulated attempt on 2026-09-19 failed before product execution:
Docker CLI Go runtime `taggedPointerPack` crash and nested Docker service failure.
Do not treat that as product failure or rewrite the signed binary for the fixture.

`prepare-fixture.py` currently creates a schema1 Normal baseline. Schema3/GCS
configuration and end-to-end archive integration require a separate fixture case;
a baseline success cannot qualify GCS. `run-kit-fixture.py` uses paths
`/etc/normal-qualification`, `/opt/fixture-kit`, and a signed `release.tar.gz`.
Do not copy real customer inputs into this fixture.

Normal .44 universal legacy bootstrap would build Patroni on an empty host.
SiemCore is preparing a pinned bootstrap dependency variant from the same product
tree. Do not bypass this by allowing arbitrary build/downloads during execution.

Native rerun on fresh A's isolated container passed real installer/type dispatch,
signed delivery/root-hook execution, exact retry and changed-input refusal.
Product .44 fails first on missing shasum; a second fresh fixture with that test
utility reaches an attempted public Patroni base-image pull, blocked by no network.
The real host stays empty. See docs/NORMAL-CLEAN-A-HANDOFF.md.
