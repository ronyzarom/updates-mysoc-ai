#!/bin/bash
# Install the MySoc platform updater (cascade tier 1) as a systemd service.
# Run as root on the mysoc platform host.
set -euo pipefail

NAME=mysoc-updater
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  BIN="bin/${NAME}-linux-amd64" ;;
    aarch64) BIN="bin/${NAME}-linux-arm64" ;;
    *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

cd "$(dirname "$0")"

if [[ $EUID -ne 0 ]]; then
    echo "run as root (sudo ./install.sh)" >&2
    exit 1
fi

echo "==> creating service user and directories"
id -u $NAME >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin $NAME
mkdir -p /etc/$NAME /var/lib/$NAME

echo "==> installing binary (self-updatable layout)"
# The binary lives in a versioned directory owned by the service user, and
# runs through symlinks: this is what lets the unprivileged updater update
# itself later with an atomic symlink swap + service restart. /usr/local/bin
# keeps a stable entry point for the unit and for operators.
VER=$("./$BIN" version 2>/dev/null | awk 'NR==1{print $2}')
if [[ -z "$VER" ]]; then
    echo "could not determine binary version from $BIN" >&2
    exit 1
fi
LAYOUT=/var/lib/$NAME/self-update
mkdir -p "$LAYOUT/releases/$VER"
install -m 0755 "$BIN" "$LAYOUT/releases/$VER/$NAME"
ln -sfn "$LAYOUT/releases/$VER" "$LAYOUT/current"
ln -sfn "$LAYOUT/current/$NAME" /usr/local/bin/$NAME
chown -R $NAME:$NAME /var/lib/$NAME

echo "==> installing configuration"
if [[ -f /etc/$NAME/config.yaml ]]; then
    echo "    /etc/$NAME/config.yaml exists — leaving it untouched"
else
    install -m 0640 -o root -g $NAME config.yaml /etc/$NAME/config.yaml
    echo "    EDIT /etc/$NAME/config.yaml before starting."
    # Tailored kits pre-fill most values, so point at what is actually
    # unresolved instead of echoing a fixed checklist that may be stale:
    if grep -nE "CHANGE-ME|PASTE-" /etc/$NAME/config.yaml; then
        echo "    ^ these values still need to be filled in"
    else
        echo "    all placeholders are pre-filled; verify current_version and start"
    fi
fi

echo "==> installing systemd unit"
install -m 0644 $NAME.service /etc/systemd/system/$NAME.service
systemctl daemon-reload
systemctl enable $NAME

echo
echo "Done. After editing the config:  systemctl start $NAME"
echo "Logs:                            journalctl -u $NAME -f"
