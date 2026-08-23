#!/bin/bash
# Install the MySoc platform updater (cascade tier 1) as a systemd service.
# Run as root on the mysoc platform host.
#
# Two ways to use it:
#   sudo ./install.sh
#       installs binary/unit and prints which config values to edit (legacy).
#   sudo ./install.sh --clean|--update --license-key ... --instance-id ...
#       additionally renders /etc/mysoc-updater/config.yaml — no editing needed.
set -euo pipefail

NAME=mysoc-updater

usage() {
    cat <<'EOF'
Usage: sudo ./install.sh [--clean|--update] [options]

Modes (enable flag-driven config rendering):
  --clean                fresh host (no app installed yet): enrolls at version
                         0.0.0 so the first offer performs the initial install
  --update               app already installed: enrolls at its current version

Options:
  --license-key KEY      operator platform key from the updates dashboard
  --instance-id ID       stable unique id, e.g. mysoc-cloud-mysoc-ai
  --parent-url URL       update server (default: https://updates.mysoc.ai)
  --signing-key HEX      pinned release-signing public key; fetched from the
                         parent with a TOFU warning when omitted
  --current-version V    used with --update (auto-detected from
                         /opt/mysoc/VERSION or :8080/health when possible)
  --ca-file PATH         CA certificate to pin for the parent (relay setups)
  -h, --help             this text

Missing required values are prompted for interactively on a terminal.
EOF
}

MODE="" LICENSE_KEY="" PARENT_URL="" INSTANCE_ID="" SIGNING_KEY=""
CURRENT_VERSION="" CA_FILE=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --clean)  MODE=clean ;;
        --update) MODE=update ;;
        --license-key)     LICENSE_KEY="${2:?--license-key needs a value}"; shift ;;
        --instance-id)     INSTANCE_ID="${2:?--instance-id needs a value}"; shift ;;
        --parent-url)      PARENT_URL="${2:?--parent-url needs a value}"; shift ;;
        --signing-key)     SIGNING_KEY="${2:?--signing-key needs a value}"; shift ;;
        --current-version) CURRENT_VERSION="${2:?--current-version needs a value}"; shift ;;
        --ca-file)         CA_FILE="${2:?--ca-file needs a value}"; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown flag: $1" >&2; usage; exit 1 ;;
    esac
    shift
done
if [[ -z "$MODE" && ( -n "$LICENSE_KEY$INSTANCE_ID$PARENT_URL$SIGNING_KEY$CURRENT_VERSION$CA_FILE" ) ]]; then
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
    prompt LICENSE_KEY "Operator platform key (from the updates dashboard)"
    prompt INSTANCE_ID "Instance id (e.g. mysoc-cloud-mysoc-ai)"
    PARENT_URL="${PARENT_URL:-https://updates.mysoc.ai}"

    if [[ "$MODE" == clean ]]; then
        CURRENT_VERSION="0.0.0"
    elif [[ -z "$CURRENT_VERSION" ]]; then
        # Best-effort detection on the host, then prompt.
        if [[ -f /opt/mysoc/VERSION ]]; then
            CURRENT_VERSION="$(tr -d '[:space:]' < /opt/mysoc/VERSION)"
            echo "    detected current app version $CURRENT_VERSION (/opt/mysoc/VERSION)"
        else
            CURRENT_VERSION="$(curl -fsS --max-time 3 http://127.0.0.1:8080/health 2>/dev/null \
                | grep -o '"version":"[^"]*"' | cut -d'"' -f4 || true)"
            [[ -n "$CURRENT_VERSION" ]] && echo "    detected current app version $CURRENT_VERSION (:8080/health)"
        fi
        prompt CURRENT_VERSION "Currently installed app version"
    fi

    local curl_ca=()
    if [[ -n "$CA_FILE" ]]; then
        [[ -f "$CA_FILE" ]] || { echo "--ca-file not found: $CA_FILE" >&2; exit 1; }
        install -m 0644 -o root -g $NAME "$CA_FILE" /etc/$NAME/parent-ca.pem
        curl_ca=(--cacert /etc/$NAME/parent-ca.pem)
    fi

    if [[ -z "$SIGNING_KEY" ]]; then
        SIGNING_KEY="$(curl -fsS --max-time 10 "${curl_ca[@]}" "$PARENT_URL/api/v1/signing-key" 2>/dev/null \
            | grep -o '[0-9a-f]\{64\}' | head -1 || true)"
        if [[ -n "$SIGNING_KEY" ]]; then
            echo "    WARNING: signing key fetched from $PARENT_URL (trust-on-first-use)."
            echo "    Verify out-of-band that it is: $SIGNING_KEY"
        fi
        prompt SIGNING_KEY "Release-signing public key (64 hex chars)"
    fi

    local target=/etc/$NAME/config.yaml tmp
    if [[ -f "$target" ]]; then
        cp -p "$target" "$target.bak-$(date -u +%Y%m%d-%H%M%S)"
        echo "    existing config backed up beside it"
    fi
    tmp=$(mktemp)
    sed -e "s|license_key: \"PASTE-PLATFORM-KEY\"|license_key: \"$LICENSE_KEY\"|" \
        -e "s|id: mysoc-CHANGE-ME|id: $INSTANCE_ID|" \
        -e "s|public_key: \"PASTE-HEX-PUBLIC-KEY\"|public_key: \"$SIGNING_KEY\"|" \
        -e "s|current_version: \"0.0.0\"|current_version: \"$CURRENT_VERSION\"|" \
        -e "s|url: https://updates.mysoc.ai|url: $PARENT_URL|" \
        config.yaml > "$tmp"
    if [[ -n "$CA_FILE" ]]; then
        sed -i.sedbak "/^  url: /a\\
  ca_file: /etc/$NAME/parent-ca.pem" "$tmp" && rm -f "$tmp.sedbak"
    fi
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

FRESH-INSTALL CHECKLIST (--clean): the first offer will install the app via
the filesystem executor. Before enabling auto-update for this node:
  1. Host provisioning is NOT the updater's job: .env secrets, database,
     and service units must exist (app team's provisioning path).
  2. The executor + root apply shim must be configured
     (see docs/UPDATE-ENTRYPOINT-CONTRACT.md).
  3. Releases shipping DB migrations are refused until the migration
     baseline exists on this host.
EOF
fi
