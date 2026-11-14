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
  --executor filesystem  wire real installs: enable the filesystem executor in
                         the config and install the bundled apply shim, the
                         NOPASSWD sudoers entry, the unit drop-in, and the
                         /opt/siemcore staging root. Required for a fresh host
                         that self-installs siemcore via the cascade.
  --license-key KEY      enrollment credential agreed with your operator
  --instance-id ID       stable unique id, e.g. siemcore-acme-01
  --parent-url URL       the mysoc relay's child-facing address,
                         e.g. https://relay.operator.example:18443
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
CUSTOMER_ID="" CUSTOMER_NAME="" SIGNING_KEY="" CURRENT_VERSION="" CA_FILE="" EXECUTOR=""
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
    prompt PARENT_URL "Mysoc relay address (e.g. https://relay.op.example:18443)"
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
    sed -e "s|url: https://mysoc.example.internal:18443|url: $PARENT_URL|" \
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

# enable_fs_executor_block uncomments the #BEGIN/#END-FS-EXECUTOR block in the
# installed config (the markers and leading '#' are stripped so the YAML nests
# correctly under simulation:). Idempotent: already-enabled configs are a no-op.
enable_fs_executor_block() {
    local target="$1" tmp
    tmp=$(mktemp)
    awk '
        /^#BEGIN-FS-EXECUTOR$/ { inblk=1; next }
        /^#END-FS-EXECUTOR$/   { inblk=0; next }
        inblk && /^#/ { sub(/^#/, ""); print; next }
        { print }
    ' "$target" > "$tmp"
    install -m 0640 -o root -g "$NAME" "$tmp" "$target"
    rm -f "$tmp"
}

# wire_filesystem_executor installs everything the filesystem executor needs that
# lives OUTSIDE the config: the siemcore apply shim, the single NOPASSWD sudoers
# entry, the unit drop-in (sudo needs NoNewPrivileges=no + ProtectSystem=no), and
# the staging root. All steps are idempotent. The shim is SiemCore-owned; the kit
# bundles it beside this script with a pinned SHA-256, and this function REFUSES
# to install a shim whose bytes do not match that pin (supply-chain gate).
wire_filesystem_executor() {
    local shim_src="$PWD/siemcore-apply-update.sh"
    local shim_sum="$PWD/siemcore-apply-update.sh.sha256"
    [[ -f "$shim_src" ]] || { echo "--executor filesystem: kit is missing siemcore-apply-update.sh" >&2; exit 1; }
    [[ -f "$shim_sum" ]] || { echo "--executor filesystem: kit is missing siemcore-apply-update.sh.sha256" >&2; exit 1; }

    echo "    verifying apply shim against pinned SHA-256"
    local want have
    want="$(awk '{print $1}' "$shim_sum" | head -1)"
    if command -v sha256sum >/dev/null 2>&1; then
        have="$(sha256sum "$shim_src" | awk '{print $1}')"
    else
        have="$(shasum -a 256 "$shim_src" | awk '{print $1}')"
    fi
    if [[ -z "$want" || "$want" != "$have" ]]; then
        echo "    apply shim checksum mismatch (pinned $want, got $have) — refusing to install" >&2
        exit 1
    fi

    echo "    enabling filesystem executor in /etc/$NAME/config.yaml"
    enable_fs_executor_block /etc/$NAME/config.yaml

    echo "    installing apply shim /usr/local/sbin/siemcore-apply-update"
    install -m 0755 -o root -g root "$shim_src" /usr/local/sbin/siemcore-apply-update

    echo "    installing sudoers drop-in /etc/sudoers.d/$NAME"
    # sudo strips the environment; env_keep passes through every variable the
    # updater sets on the apply call (see UPDATE-ENTRYPOINT-CONTRACT.md) so the
    # shim and any artifact entrypoint see the full phase context.
    cat > /etc/sudoers.d/$NAME <<EOF
Defaults!/usr/local/sbin/siemcore-apply-update env_keep += "UPDATER_PHASE CURRENT_DIR VERSION FROM_VERSION PRODUCT INSTALL_ROOT ARTIFACT_KIND ARTIFACT_SHA256 DEPENDENCY_VALIDATION"
$NAME ALL=(root) NOPASSWD: SETENV: /usr/local/sbin/siemcore-apply-update
EOF
    chmod 0440 /etc/sudoers.d/$NAME
    visudo -c -f /etc/sudoers.d/$NAME >/dev/null

    echo "    installing unit drop-in (NoNewPrivileges=no, ProtectSystem=no)"
    mkdir -p /etc/systemd/system/$NAME.service.d
    cat > /etc/systemd/system/$NAME.service.d/executor.conf <<'EOF'
[Service]
NoNewPrivileges=no
ProtectSystem=no
EOF

    echo "    creating staging root /opt/siemcore"
    # Matches simulation.filesystem.install_root in config.yaml. The executor
    # stages releases + the current symlink here as the updater service user.
    mkdir -p /opt/siemcore
    chown "$NAME:$NAME" /opt/siemcore
    chmod 0755 /opt/siemcore
}

# json_string VAR KEY FILE — read a top-level JSON string value without jq.
# The updater writes state.json with json.MarshalIndent, so each identity field
# is a simple "key": "value" pair; the values here (instance id, api key, relay
# token) never contain quotes, so this is safe.
json_string() {
    local __var="$1" __key="$2" __file="$3" __val
    __val="$(grep -oE "\"$__key\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" "$__file" | head -1 \
        | sed -E "s/.*:[[:space:]]*\"([^\"]*)\"\$/\1/")"
    printf -v "$__var" '%s' "$__val"
}

# reset_clean_state prepares a --clean (re-)enrolment WITHOUT losing the node's
# relay identity. The updater's persisted state records the last installed
# product version and WINS over the config's current_version, so a re-enrol on a
# host that still carries state.json would report "siemcore up to date" and
# silently skip first_install. But unlike a tier-1 node, a siemcore node enrols
# through a mysoc RELAY, which binds its instance_id to a relay_token at first
# contact; deleting state.json would drop that token and the relay would reject
# every later heartbeat (401 relay_token_absent) until the binding is cleared at
# the parent. So we PRESERVE instance_id, api_key and relay_token and drop only
# the version/attempt fields, leaving the node genuinely "fresh" at 0.0.0 while
# keeping its enrolment. A brand-new host has no state.json and needs nothing.
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
        printf '{\n'
        printf '  "instance_id": "%s"' "$iid"
        [[ -n "$akey" ]] && printf ',\n  "api_key": "%s"' "$akey"
        [[ -n "$rtok" ]] && printf ',\n  "relay_token": "%s"' "$rtok"
        printf '\n}\n'
    } > "$state"
    chmod 0600 "$state"
    chown "$NAME:$NAME" "$state" 2>/dev/null || true
    echo "    --clean: reset product version to 0.0.0, preserved relay identity" \
         "(instance_id${rtok:+, relay_token}); previous state saved to $(basename "$backup")"
}

echo "==> creating service user and directories"
id -u $NAME >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin $NAME
mkdir -p /var/lib/$NAME
# The caller's umask is intentionally untrusted. Greenfield/first-install
# commonly runs with umask 077, which would leave /etc/$NAME as 0700 root:root
# and the unprivileged service unable to TRAVERSE it to read its 0640
# config.yaml — the updater then cannot start, so it can never self-update out
# of the problem. Create the dir 0750 root:$NAME explicitly (survivability:
# matches kits/mysoc/install.sh; ported from version/1.16.1 f5dc7dd during the
# 1.18.0 stranded-commit audit).
install -d -m 0750 -o root -g "$NAME" /etc/$NAME

# A --clean (re-)enrolment must start the product at 0.0.0 so the first offer
# runs first_install, while keeping the relay identity (see reset_clean_state).
if [[ "$MODE" == clean ]]; then
    reset_clean_state
fi

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

if [[ "$EXECUTOR" == "filesystem" ]]; then
    echo "==> wiring filesystem executor (real installs)"
    wire_filesystem_executor
fi

echo "==> installing systemd unit"
install -m 0644 $NAME.service /etc/systemd/system/$NAME.service
# Survivability (1.18.0): the root-owned restore backstop (invariant 4). The
# main unit's OnFailure triggers it when the updater flaps past its start limit
# — the signature of a self-update to a binary that cannot stay up — and it rolls
# the self-update layout back to .previous and restarts the known-good binary.
if [[ -f "$PWD/$NAME-self-restore.sh" && -f "$PWD/$NAME-restore.service" ]]; then
    echo "    installing self-update restore backstop ($NAME-restore.service)"
    install -m 0755 -o root -g root "$PWD/$NAME-self-restore.sh" /usr/local/sbin/$NAME-self-restore
    install -m 0644 "$PWD/$NAME-restore.service" /etc/systemd/system/$NAME-restore.service
else
    echo "    WARNING: restore backstop files missing from kit; skipping (invariant 4 not armed)" >&2
fi
systemctl daemon-reload
systemctl enable $NAME

echo
echo "Done. Start with:  systemctl start $NAME"
echo "Logs:              journalctl -u $NAME -f"
if [[ "$MODE" == clean ]]; then
    if [[ "$EXECUTOR" == "filesystem" ]]; then
        cat <<'EOF'

FRESH-INSTALL (--clean --executor filesystem): on first start the updater pulls
the ring artifact and runs its updater/apply first_install phase (30m budget),
which self-provisions siemcore end to end. Nothing else to configure here. If
first_install fails, the updater reports the error and backs off (5m/15m/60m);
fix the host and it resumes automatically. Watch:
  journalctl -u siemcore-cascade-updater -f
EOF
    else
        cat <<'EOF'

FRESH-INSTALL CHECKLIST (--clean, no executor wired): for a cascade self-install
pass --executor filesystem (which enables the executor and installs the siemcore
apply shim + sudoers entry + unit drop-in). Otherwise the updater cannot apply a
real install and offered updates are reported as failures.
EOF
    fi
fi
