#!/bin/bash
# Install the WideCore cascade updater (leaf child of mysoc) as a systemd service.
# Run as root on the widecore gateway host.
#
# Two ways to use it:
#   sudo ./install.sh
#       installs binary/unit and prints which config values to edit (legacy).
#   sudo ./install.sh --clean --executor filesystem --license-key ... --parent-url ... [options]
#       additionally renders the config and wires the WideCore host files.
set -euo pipefail

NAME=widecore-updater

usage() {
    cat <<'EOF'
Usage: sudo ./install.sh [--clean|--update] [options]

Modes (enable flag-driven config rendering):
  --clean                fresh host (no widecore installed yet): enrolls at
                         version 0.0.0 so the first offer performs first_install
  --update               widecore already installed: enrolls at its version

Options:
  --executor filesystem  wire real installs: install the four bundled,
                         SHA-256-pinned WideCore host files (apply shim, sudoers
                         entry, updater-override.conf unit drop-in, and
                         updater-executor.yaml spliced into the config) plus the
                         /opt/widecore-cascade staging root.
  --license-key KEY      enrollment credential agreed with your operator
  --instance-id ID       stable unique id, e.g. widecore-acme-01
  --parent-url URL       the mysoc relay's child-facing address,
                         e.g. https://relay.operator.example:18443
  --parent-id ID         instance id of the mysoc node this widecore belongs to
  --customer-id ID       end customer identifier (groups the fleet view)
  --customer-name NAME   end customer display name
  --signing-key HEX      pinned release-signing public key (ask your operator)
  --update-group RING    optional initial ring (alpha|beta|stable|production);
                         honored only when creating a brand-new instance_id
  --current-version V    required with --update
  --ca-file PATH         the relay's cert.pem to pin; omit when the relay
                         serves a publicly trusted certificate
  -h, --help             this text

Missing required values are prompted for interactively on a terminal.
EOF
}

MODE="" LICENSE_KEY="" PARENT_URL="" INSTANCE_ID="" PARENT_ID=""
CUSTOMER_ID="" CUSTOMER_NAME="" SIGNING_KEY="" CURRENT_VERSION="" CA_FILE=""
EXECUTOR="" UPDATE_GROUP=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --clean)  MODE=clean ;;
        --update) MODE=update ;;
        --executor)        EXECUTOR="${2:?--executor needs a value}"; shift ;;
        --license-key)     LICENSE_KEY="${2:?--license-key needs a value}"; shift ;;
        --instance-id)     INSTANCE_ID="${2:?--instance-id needs a value}"; shift ;;
        --parent-url)      PARENT_URL="${2:?--parent-url needs a value}"; shift ;;
        --parent-id)       PARENT_ID="${2:?--parent-id needs a value}"; shift ;;
        --customer-id)     CUSTOMER_ID="${2:?--customer-id needs a value}"; shift ;;
        --customer-name)   CUSTOMER_NAME="${2:?--customer-name needs a value}"; shift ;;
        --signing-key)     SIGNING_KEY="${2:?--signing-key needs a value}"; shift ;;
        --update-group)    UPDATE_GROUP="${2:?--update-group needs a value}"; shift ;;
        --current-version) CURRENT_VERSION="${2:?--current-version needs a value}"; shift ;;
        --ca-file)         CA_FILE="${2:?--ca-file needs a value}"; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown flag: $1" >&2; usage; exit 1 ;;
    esac
    shift
done
if [[ -n "$EXECUTOR" && "$EXECUTOR" != "filesystem" ]]; then
    echo "unsupported --executor '$EXECUTOR' (only 'filesystem')" >&2
    exit 1
fi
if [[ -n "$UPDATE_GROUP" ]]; then
    case "$UPDATE_GROUP" in
        alpha|beta|stable|production) ;;
        *) echo "invalid --update-group '$UPDATE_GROUP' (expected alpha, beta, stable, or production)" >&2; exit 1 ;;
    esac
fi
if [[ -z "$MODE" && ( -n "$LICENSE_KEY$PARENT_URL$INSTANCE_ID$PARENT_ID$CUSTOMER_ID$CUSTOMER_NAME$SIGNING_KEY$CURRENT_VERSION$CA_FILE$UPDATE_GROUP" ) ]]; then
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

render_config() {
    prompt LICENSE_KEY "Enrollment credential (agreed with your operator)"
    prompt PARENT_URL "Mysoc relay address (e.g. https://relay.op.example:18443)"
    prompt INSTANCE_ID "Instance id (e.g. widecore-acme-01)"
    prompt PARENT_ID "Parent mysoc instance id"
    prompt CUSTOMER_ID "Customer id"
    prompt CUSTOMER_NAME "Customer display name"
    prompt SIGNING_KEY "Release-signing public key (64 hex chars, from your operator)"

    if [[ "$MODE" == clean ]]; then
        CURRENT_VERSION="0.0.0"
    else
        prompt CURRENT_VERSION "Currently installed widecore version"
    fi

    if [[ -n "$CA_FILE" ]]; then
        [[ -f "$CA_FILE" ]] || { echo "--ca-file not found: $CA_FILE" >&2; exit 1; }
        install -m 0644 -o root -g "$NAME" "$CA_FILE" "/etc/$NAME/mysoc-relay-ca.pem"
    fi

    local target="/etc/$NAME/config.yaml" tmp
    if [[ -f "$target" ]]; then
        cp -p "$target" "$target.bak-$(date -u +%Y%m%d-%H%M%S)"
        echo "    existing config backed up beside it"
    fi
    tmp=$(mktemp)
    sed -e "s|url: https://mysoc.example.internal:18443|url: $PARENT_URL|" \
        -e "s|license_key: \"CHANGE-ME-WIDECORE-CREDENTIAL\"|license_key: \"$LICENSE_KEY\"|" \
        -e "s|id: widecore-CHANGE-ME|id: $INSTANCE_ID|" \
        -e "s|parent_id: mysoc-CHANGE-ME|parent_id: $PARENT_ID|" \
        -e "s|customer_id: customer-CHANGE-ME|customer_id: $CUSTOMER_ID|" \
        -e "s|customer_name: \"Customer Name\"|customer_name: \"$CUSTOMER_NAME\"|" \
        -e "s|public_key: \"PASTE-HEX-PUBLIC-KEY\"|public_key: \"$SIGNING_KEY\"|" \
        -e "s|current_version: \"0.0.0\"|current_version: \"$CURRENT_VERSION\"|" \
        config.yaml > "$tmp"
    if [[ -n "$UPDATE_GROUP" ]]; then
        sed -i.sedbak "s|^  # update_group: stable|  update_group: $UPDATE_GROUP|" "$tmp"
    fi
    if [[ -n "$CA_FILE" ]]; then
        sed -i.sedbak "s|ca_file: mysoc-relay-ca.pem|ca_file: /etc/$NAME/mysoc-relay-ca.pem|" "$tmp"
    else
        sed -i.sedbak "s|^  ca_file: mysoc-relay-ca.pem|  # ca_file: (not set — relay serves a publicly trusted certificate)|" "$tmp"
    fi
    rm -f "$tmp.sedbak"
    install -m 0640 -o root -g "$NAME" "$tmp" "$target"
    rm -f "$tmp"

    if grep -nE "CHANGE-ME|PASTE-" "$target"; then
        echo "    ^ unresolved placeholders remain — fix before starting" >&2
        exit 1
    fi
    echo "    config rendered: instance=$INSTANCE_ID parent=$PARENT_URL version=$CURRENT_VERSION mode=$MODE${UPDATE_GROUP:+ group=$UPDATE_GROUP}"
}

# splice_executor_block replaces whatever sits between the #BEGIN/#END-FS-EXECUTOR
# markers in the installed config with WideCore's updater-executor.yaml verbatim.
splice_executor_block() {
    local target="$1" block="$2" tmp
    tmp=$(mktemp)
    awk -v blockfile="$block" '
        /^#BEGIN-FS-EXECUTOR$/ { print; while ((getline line < blockfile) > 0) print line; close(blockfile); skip=1; next }
        /^#END-FS-EXECUTOR$/   { skip=0; print; next }
        skip { next }
        { print }
    ' "$target" > "$tmp"
    install -m 0640 -o root -g "$NAME" "$tmp" "$target"
    rm -f "$tmp"
}

verify_pinned() {
    local file="$1" sums="$2" base want have
    base="$(basename "$file")"
    [[ -f "$file" ]] || { echo "    missing host file: $base — refusing" >&2; exit 1; }
    want="$(awk -v b="$base" '$2==b || $2=="*"b {print $1; exit}' "$sums")"
    if command -v sha256sum >/dev/null 2>&1; then
        have="$(sha256sum "$file" | awk '{print $1}')"
    else
        have="$(shasum -a 256 "$file" | awk '{print $1}')"
    fi
    [[ -n "$want" && "$want" == "$have" ]] || {
        echo "    host file $base failed its pinned checksum (pin ${want:-none}, got $have) — refusing" >&2
        exit 1
    }
}

# wire_filesystem_executor installs the four WideCore-owned host files
# byte-for-byte after verifying their SHA-256 pins, plus the staging root.
wire_filesystem_executor() {
    local shim_src="$PWD/widecore-apply-update"
    local sudoers_src="$PWD/sudoers-widecore-updater"
    local dropin_src="$PWD/updater-override.conf"
    local block_src="$PWD/updater-executor.yaml"
    local sums="$PWD/widecore-host-files.sha256"
    [[ -f "$sums" ]] || { echo "--executor filesystem: kit is missing widecore-host-files.sha256" >&2; exit 1; }

    echo "    verifying WideCore host files against pinned SHA-256"
    verify_pinned "$shim_src"    "$sums"
    verify_pinned "$sudoers_src" "$sums"
    verify_pinned "$dropin_src"  "$sums"
    verify_pinned "$block_src"   "$sums"

    echo "    splicing updater-executor.yaml into /etc/$NAME/config.yaml"
    splice_executor_block "/etc/$NAME/config.yaml" "$block_src"

    echo "    installing apply shim /usr/local/sbin/widecore-apply-update"
    install -m 0755 -o root -g root "$shim_src" /usr/local/sbin/widecore-apply-update

    echo "    installing sudoers drop-in /etc/sudoers.d/$NAME"
    install -m 0440 -o root -g root "$sudoers_src" "/etc/sudoers.d/$NAME"
    visudo -c -f "/etc/sudoers.d/$NAME" >/dev/null

    echo "    installing unit drop-in /etc/systemd/system/$NAME.service.d/override.conf"
    mkdir -p "/etc/systemd/system/$NAME.service.d"
    install -m 0644 -o root -g root "$dropin_src" "/etc/systemd/system/$NAME.service.d/override.conf"

    echo "    creating staging root /opt/widecore-cascade"
    mkdir -p /opt/widecore-cascade
    chown "$NAME:$NAME" /opt/widecore-cascade
    chmod 0755 /opt/widecore-cascade
}

json_string() {
    local __var="$1" __key="$2" __file="$3" __val
    __val="$(grep -oE "\"$__key\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" "$__file" | head -1 \
        | sed -E "s/.*:[[:space:]]*\"([^\"]*)\"\$/\1/" || true)"
    printf -v "$__var" '%s' "$__val"
}

# reset_clean_state prepares a --clean (re-)enrollment WITHOUT losing the
# node's relay identity (instance_id, api_key, relay_token).
reset_clean_state() {
    local state="/var/lib/$NAME/state.json"
    [[ -f "$state" ]] || return 0
    local iid akey rtok backup
    json_string iid  instance_id "$state"
    json_string akey api_key     "$state"
    json_string rtok relay_token "$state"
    backup="$state.bak-$(date -u +%Y%m%d-%H%M%S)"
    cp -p "$state" "$backup"
    {
        local sep=$'\n'
        printf '{'
        if [[ -n "$iid"  ]]; then printf '%s  "instance_id": "%s"' "$sep" "$iid";  sep=$',\n'; fi
        if [[ -n "$akey" ]]; then printf '%s  "api_key": "%s"'     "$sep" "$akey"; sep=$',\n'; fi
        if [[ -n "$rtok" ]]; then printf '%s  "relay_token": "%s"' "$sep" "$rtok"; sep=$',\n'; fi
        printf '\n}\n'
    } > "$state"
    chmod 0600 "$state"
    chown "$NAME:$NAME" "$state" 2>/dev/null || true
    local preserved=""
    [[ -n "$iid"  ]] && preserved="${preserved:+$preserved, }instance_id"
    [[ -n "$akey" ]] && preserved="${preserved:+$preserved, }api_key"
    [[ -n "$rtok" ]] && preserved="${preserved:+$preserved, }relay_token"
    echo "    --clean: reset product version to 0.0.0, preserved relay identity" \
         "(${preserved:-none present}); previous state saved to $(basename "$backup")"
}

echo "==> creating service user and directories"
id -u "$NAME" >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin "$NAME"
mkdir -p "/var/lib/$NAME"
install -d -m 0750 -o root -g "$NAME" "/etc/$NAME"

if [[ "$MODE" == clean ]]; then
    reset_clean_state
fi

echo "==> installing binary (self-updatable layout)"
VER=$("./$BIN" version 2>/dev/null | awk 'NR==1{print $2}')
if [[ -z "$VER" ]]; then
    echo "could not determine binary version from $BIN" >&2
    exit 1
fi
LAYOUT="/var/lib/$NAME/self-update"
mkdir -p "$LAYOUT/releases/$VER"
install -m 0755 "$BIN" "$LAYOUT/releases/$VER/$NAME"
ln -sfn "$LAYOUT/releases/$VER" "$LAYOUT/current"
ln -sfn "$LAYOUT/current/$NAME" "/usr/local/bin/$NAME"
chown -R "$NAME:$NAME" "/var/lib/$NAME"

echo "==> installing configuration"
if [[ -n "$MODE" ]]; then
    render_config
elif [[ -f "/etc/$NAME/config.yaml" ]]; then
    echo "    /etc/$NAME/config.yaml exists — leaving it untouched"
else
    install -m 0640 -o root -g "$NAME" config.yaml "/etc/$NAME/config.yaml"
    echo "    EDIT /etc/$NAME/config.yaml before starting."
    if grep -nE "CHANGE-ME|PASTE-" "/etc/$NAME/config.yaml"; then
        echo "    ^ these values still need to be filled in"
    else
        echo "    all placeholders are pre-filled; verify current_version and start"
    fi
fi

if [[ "$EXECUTOR" == "filesystem" ]]; then
    echo "==> wiring filesystem executor (real installs)"
    wire_filesystem_executor
fi

echo "==> installing systemd unit"
install -m 0644 "$NAME.service" "/etc/systemd/system/$NAME.service"
if [[ -f "$PWD/$NAME-self-restore.sh" && -f "$PWD/$NAME-restore.service" ]]; then
    echo "    installing self-update restore backstop ($NAME-restore.service)"
    install -m 0755 -o root -g root "$PWD/$NAME-self-restore.sh" "/usr/local/sbin/$NAME-self-restore"
    install -m 0644 "$PWD/$NAME-restore.service" "/etc/systemd/system/$NAME-restore.service"
else
    echo "    WARNING: restore backstop files missing from kit; skipping (invariant 4 not armed)" >&2
fi
systemctl daemon-reload
systemctl enable "$NAME"

echo
echo "Done. Start with:  systemctl start $NAME"
echo "Logs:              journalctl -u $NAME -f"
if [[ "$MODE" == clean ]]; then
    if [[ "$EXECUTOR" == "filesystem" ]]; then
        cat <<'EOF'

FRESH-INSTALL (--clean --executor filesystem): on first start the updater pulls
the ring artifact and runs its updater/apply first_install phase (30m budget),
which self-provisions widecore end to end. If first_install fails, the updater
reports the error and backs off (5m/15m/60m); fix the host and it resumes
automatically. Watch:
  journalctl -u widecore-updater -f
EOF
    else
        cat <<'EOF'

FRESH-INSTALL CHECKLIST (--clean, no executor wired): for a cascade self-install
pass --executor filesystem (which enables the executor and installs the widecore
apply shim + sudoers entry + unit drop-in). Otherwise the updater cannot apply a
real install and offered updates are reported as failures.
EOF
    fi
fi
