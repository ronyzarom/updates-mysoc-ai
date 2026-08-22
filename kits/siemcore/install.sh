#!/bin/bash
# Install the SiemCore cascade updater (tier 2) as a systemd service.
# Run as root on the siemcore server.
#
# The unit is named siemcore-cascade-updater (NOT siemcore-updater) so it
# cannot collide with the retired v2 daemon of that name in posture audits.
#
# Two ways to use it:
#   sudo ./install.sh
#       installs binary/unit and prints which config values to edit (legacy).
#   sudo ./install.sh --update --license-key ... --parent-url ... [options]
#       additionally renders the config — no editing needed.
set -euo pipefail

NAME=siemcore-cascade-updater

usage() {
    cat <<'EOF'
Usage: sudo ./install.sh [--clean|--update] [options]

Modes (enable flag-driven config rendering):
  --clean                fresh host (no siemcore installed yet): enrolls at
                         version 0.0.0 so the first offer performs the install
  --update               siemcore already installed: enrolls at its version

Options:
  --license-key KEY      enrollment credential agreed with your operator
  --instance-id ID       stable unique id, e.g. siemcore-acme-01
  --parent-url URL       the mysoc relay's child-facing address,
                         e.g. https://relay.operator.example:8443
  --parent-id ID         instance id of the mysoc node this siemcore belongs to
  --customer-id ID       end customer identifier (groups the fleet view)
  --customer-name NAME   end customer display name
  --signing-key HEX      pinned release-signing public key (ask your operator)
  --current-version V    required with --update
  --ca-file PATH         the relay's cert.pem to pin; omit when the relay
                         serves a publicly trusted certificate
  -h, --help             this text

Missing required values are prompted for interactively on a terminal.
EOF
}

MODE="" LICENSE_KEY="" PARENT_URL="" INSTANCE_ID="" PARENT_ID=""
CUSTOMER_ID="" CUSTOMER_NAME="" SIGNING_KEY="" CURRENT_VERSION="" CA_FILE=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --clean)  MODE=clean ;;
        --update) MODE=update ;;
        --license-key)     LICENSE_KEY="${2:?--license-key needs a value}"; shift ;;
        --instance-id)     INSTANCE_ID="${2:?--instance-id needs a value}"; shift ;;
        --parent-url)      PARENT_URL="${2:?--parent-url needs a value}"; shift ;;
        --parent-id)       PARENT_ID="${2:?--parent-id needs a value}"; shift ;;
        --customer-id)     CUSTOMER_ID="${2:?--customer-id needs a value}"; shift ;;
        --customer-name)   CUSTOMER_NAME="${2:?--customer-name needs a value}"; shift ;;
        --signing-key)     SIGNING_KEY="${2:?--signing-key needs a value}"; shift ;;
        --current-version) CURRENT_VERSION="${2:?--current-version needs a value}"; shift ;;
        --ca-file)         CA_FILE="${2:?--ca-file needs a value}"; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown flag: $1" >&2; usage; exit 1 ;;
    esac
    shift
done
if [[ -z "$MODE" && ( -n "$LICENSE_KEY$PARENT_URL$INSTANCE_ID$PARENT_ID$CUSTOMER_ID$CUSTOMER_NAME$SIGNING_KEY$CURRENT_VERSION$CA_FILE" ) ]]; then
    echo "config flags require a mode: --clean or --update" >&2
    exit 1
fi

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

# prompt VAR "question" — interactive fallback for a missing required value.
prompt() {
    local var="$1" question="$2" value
    [[ -n "${!var}" ]] && return 0
    if [[ ! -t 0 ]]; then
        echo "missing required value: $question (use the corresponding flag)" >&2
        exit 1
    fi
    read -r -p "$question: " value
    [[ -n "$value" ]] || { echo "value must not be empty" >&2; exit 1; }
    printf -v "$var" '%s' "$value"
}

# render_config — fill the kit template from flags and install it.
render_config() {
    prompt LICENSE_KEY "Enrollment credential (agreed with your operator)"
    prompt PARENT_URL "Mysoc relay address (e.g. https://relay.op.example:8443)"
    prompt INSTANCE_ID "Instance id (e.g. siemcore-acme-01)"
    prompt PARENT_ID "Parent mysoc instance id"
    prompt CUSTOMER_ID "Customer id"
    prompt CUSTOMER_NAME "Customer display name"
    prompt SIGNING_KEY "Release-signing public key (64 hex chars, from your operator)"

    if [[ "$MODE" == clean ]]; then
        CURRENT_VERSION="0.0.0"
    else
        prompt CURRENT_VERSION "Currently installed siemcore version"
    fi

    if [[ -n "$CA_FILE" ]]; then
        [[ -f "$CA_FILE" ]] || { echo "--ca-file not found: $CA_FILE" >&2; exit 1; }
        install -m 0644 -o root -g $NAME "$CA_FILE" /etc/$NAME/mysoc-relay-ca.pem
    fi

    local target=/etc/$NAME/config.yaml tmp
    if [[ -f "$target" ]]; then
        cp -p "$target" "$target.bak-$(date -u +%Y%m%d-%H%M%S)"
        echo "    existing config backed up beside it"
    fi
    tmp=$(mktemp)
    sed -e "s|url: https://mysoc.example.internal:8443|url: $PARENT_URL|" \
        -e "s|license_key: \"CHANGE-ME-SIEMCORE-CREDENTIAL\"|license_key: \"$LICENSE_KEY\"|" \
        -e "s|id: siemcore-CHANGE-ME|id: $INSTANCE_ID|" \
        -e "s|parent_id: mysoc-CHANGE-ME|parent_id: $PARENT_ID|" \
        -e "s|customer_id: customer-CHANGE-ME|customer_id: $CUSTOMER_ID|" \
        -e "s|customer_name: \"Customer Name\"|customer_name: \"$CUSTOMER_NAME\"|" \
        -e "s|public_key: \"PASTE-HEX-PUBLIC-KEY\"|public_key: \"$SIGNING_KEY\"|" \
        -e "s|current_version: \"0.0.0\"|current_version: \"$CURRENT_VERSION\"|" \
        config.yaml > "$tmp"
    if [[ -n "$CA_FILE" ]]; then
        sed -i.sedbak "s|ca_file: mysoc-relay-ca.pem|ca_file: /etc/$NAME/mysoc-relay-ca.pem|" "$tmp"
    else
        # Relay serves a publicly trusted certificate — no pin needed.
        sed -i.sedbak "s|^  ca_file: mysoc-relay-ca.pem|  # ca_file: (not set — relay serves a publicly trusted certificate)|" "$tmp"
    fi
    rm -f "$tmp.sedbak"
    install -m 0640 -o root -g $NAME "$tmp" "$target"
    rm -f "$tmp"

    if grep -nE "CHANGE-ME|PASTE-" "$target"; then
        echo "    ^ unresolved placeholders remain — fix before starting" >&2
        exit 1
    fi
    echo "    config rendered: instance=$INSTANCE_ID parent=$PARENT_URL version=$CURRENT_VERSION mode=$MODE"
}

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
if [[ -n "$MODE" ]]; then
    render_config
elif [[ -f /etc/$NAME/config.yaml ]]; then
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
echo "Done. Start with:  systemctl start $NAME"
echo "Logs:              journalctl -u $NAME -f"
if [[ "$MODE" == clean ]]; then
    cat <<'EOF'

FRESH-INSTALL CHECKLIST (--clean): the first offer will install siemcore via
the filesystem executor once it is enabled. Before enabling auto-update:
  1. Host provisioning is NOT the updater's job: compose prerequisites,
     database, and secrets must exist (siemcore team's provisioning path).
  2. The executor block stays commented until the v3 compose contract is
     confirmed (see docs/UPDATE-ENTRYPOINT-CONTRACT.md).
EOF
fi
