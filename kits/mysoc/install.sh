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
  --executor filesystem  wire real installs: enable the filesystem executor in
                         the config and install the root apply wrapper, the
                         NOPASSWD sudoers entry, the unit drop-in, and the
                         /opt/mysoc-cascade staging root. Required for a fresh
                         host that self-installs via the cascade.
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
CURRENT_VERSION="" CA_FILE="" EXECUTOR=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --clean)  MODE=clean ;;
        --update) MODE=update ;;
        --executor)        EXECUTOR="${2:?--executor needs a value}"; shift ;;
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
if [[ -n "$EXECUTOR" && "$EXECUTOR" != "filesystem" ]]; then
    echo "unsupported --executor '$EXECUTOR' (only 'filesystem')" >&2
    exit 1
fi
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
# lives OUTSIDE the config: the root apply wrapper, the single NOPASSWD sudoers
# entry, the unit drop-in (sudo needs NoNewPrivileges=no + ProtectSystem=no), and
# the staging root. All steps are idempotent. The wrapper ships in the kit beside
# this script.
wire_filesystem_executor() {
    local shim_src="$PWD/mysoc-apply-update.sh"
    [[ -f "$shim_src" ]] || { echo "--executor filesystem: kit is missing mysoc-apply-update.sh" >&2; exit 1; }

    echo "    enabling filesystem executor in /etc/$NAME/config.yaml"
    enable_fs_executor_block /etc/$NAME/config.yaml

    echo "    installing root apply wrapper /usr/local/sbin/mysoc-apply-update"
    install -m 0755 -o root -g root "$shim_src" /usr/local/sbin/mysoc-apply-update

    echo "    installing sudoers drop-in /etc/sudoers.d/$NAME"
    cat > /etc/sudoers.d/$NAME <<EOF
Defaults!/usr/local/sbin/mysoc-apply-update env_keep += "UPDATER_PHASE CURRENT_DIR VERSION FROM_VERSION PRODUCT INSTALL_ROOT"
$NAME ALL=(root) NOPASSWD: SETENV: /usr/local/sbin/mysoc-apply-update
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

    echo "    creating staging root /opt/mysoc-cascade"
    mkdir -p /opt/mysoc-cascade
    chown "$NAME:$NAME" /opt/mysoc-cascade
    chmod 0755 /opt/mysoc-cascade
}

echo "==> creating service user and directories"
id -u $NAME >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin $NAME
# Explicit modes (never inherit the caller's umask): the service user must be
# able to TRAVERSE /etc/$NAME to read its 0640 config, so the dir is 0750
# root:$NAME. install -d applies the mode even if the directory already exists.
install -d -m 0750 -o root   -g "$NAME" /etc/$NAME
install -d -m 0750 -o "$NAME" -g "$NAME" /var/lib/$NAME

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
systemctl daemon-reload
systemctl enable $NAME

echo
echo "Done. Start with:  systemctl start $NAME"
echo "Logs:              journalctl -u $NAME -f"
if [[ "$MODE" == clean ]]; then
    if [[ "$EXECUTOR" == "filesystem" ]]; then
        cat <<'EOF'

FRESH-INSTALL (--clean --executor filesystem): on first start the updater pulls
the ring artifact and runs its updater/apply first_install phase, which
self-provisions the host end to end (deps, DB baseline, .env, TLS, units, nginx,
code, health gate). Nothing else to configure here. If first_install fails, the
updater reports the error and backs off (5m/15m/60m); fix the host and it
resumes automatically. Watch:  journalctl -u mysoc-updater -f
EOF
    else
        cat <<'EOF'

FRESH-INSTALL CHECKLIST (--clean, no executor wired): the first offer installs
the app. For a cascade self-install pass --executor filesystem (which wires the
executor + root apply shim). Otherwise the updater cannot apply a real install.
Releases shipping DB migrations need an artifact that ships updater/apply (it
lays down and records the baseline).
EOF
    fi
fi
