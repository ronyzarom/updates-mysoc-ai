#!/usr/bin/env bash
# widecore-enroll.sh — ONE-command fresh install for a WideCore AI gateway.
#
# Run as root on a fresh Ubuntu 22.04 / 24.04 or Debian 12 VM:
#
#   curl -fsSL https://updates.mysoc.ai/install/widecore-enroll.sh | sudo bash -s -- \
#     --license-key <KEY> --config /root/server.conf
#
# This thin bootstrap does only what the updater/artifact cannot do for itself:
#   1. ensure curl exists,
#   2. fetch + checksum-verify the widecore updater kit from updates.mysoc.ai,
#   3. copy server.conf to /etc/widecore/install-seed.env (0600) and stage any
#      TLS_MODE=provided cert/key out of /root, /home, or /tmp into
#      /etc/widecore/seed-tls/ (widecore-updater runs with ProtectHome=true
#      and PrivateTmp=true),
#   4. install + enroll widecore-updater (kit install.sh --clean --executor
#      filesystem) against the parent MySoc relay, at version 0.0.0,
#   5. start widecore-updater and wait for "update applied and verified",
#      then confirm http://127.0.0.1:9180/livez and print the address + version.
#
# Everything else — PostgreSQL (when local), widecore.env, TLS placement,
# migrations, initial admin, systemd unit — is provisioned by the release's own
# updater/apply "first_install" phase through the normal cascade path.
set -euo pipefail

log()  { echo "[widecore-enroll] $*"; }
warn() { echo "[widecore-enroll] WARNING: $*" >&2; }
die()  { echo "[widecore-enroll] ERROR: $*" >&2; exit 1; }

LICENSE_KEY="" CONFIG_FILE="" INSTANCE_ID="" SIGNING_KEY="" UPDATE_GROUP=""
UPDATES_URL="${MYSOC_UPDATES_URL:-https://updates.mysoc.ai}"
KIT_URL="${WIDECORE_UPDATER_KIT_URL:-}"
WAIT_SECONDS="${WIDECORE_ENROLL_WAIT:-2100}"   # 35 min; first_install budget is 30m
SEED_PATH="/etc/widecore/install-seed.env"
INSTANCE_ID_FILE="/etc/widecore/instance-id"
HEALTH_URL="${WIDECORE_HEALTH_URL:-http://127.0.0.1:9180/livez}"

usage() { sed -n '2,23p' "$0"; exit "${1:-0}"; }

while [[ $# -gt 0 ]]; do
    case "$1" in
        --license-key)  LICENSE_KEY="${2:?}"; shift 2 ;;
        --config)       CONFIG_FILE="${2:?}"; shift 2 ;;
        --instance-id)  INSTANCE_ID="${2:?}"; shift 2 ;;
        --signing-key)  SIGNING_KEY="${2:?}"; shift 2 ;;
        --update-group) UPDATE_GROUP="${2:?}"; shift 2 ;;
        --updates-url)  UPDATES_URL="${2:?}"; shift 2 ;;
        --kit-url)      KIT_URL="${2:?}"; shift 2 ;;
        -h|--help)      usage 0 ;;
        *) die "unknown flag: $1 (see --help)" ;;
    esac
done

[[ $EUID -eq 0 ]] || die "run as root (pipe to 'sudo bash')"
[[ -n "$LICENSE_KEY" ]] || die "--license-key is required"
[[ -n "$CONFIG_FILE" ]] || die "--config <server.conf> is required"
[[ -f "$CONFIG_FILE" ]] || die "--config not found: $CONFIG_FILE"

conf_get() {
    local key="$1" line
    line="$(grep -E "^[[:space:]]*${key}[[:space:]]*=" "$CONFIG_FILE" | tail -1 || true)"
    [[ -z "$line" ]] && return 0
    line="${line#*=}"
    line="${line%\"}"; line="${line#\"}"
    line="${line%\'}"; line="${line#\'}"
    printf '%s' "$line"
}

PARENT_URL="$(conf_get UPDATES_PARENT_URL)"
PARENT_ID="$(conf_get UPDATES_PARENT_ID)"
CUSTOMER_ID="$(conf_get CUSTOMER_ID)"
CUSTOMER_NAME="$(conf_get CUSTOMER_NAME)"
RELAY_CA_FILE="$(conf_get UPDATES_RELAY_CA_FILE)"
TLS_MODE="$(conf_get TLS_MODE)"
TLS_CERT_FILE="$(conf_get TLS_CERT_FILE)"
[[ -z "$TLS_CERT_FILE" ]] && TLS_CERT_FILE="$(conf_get TLS_CERT_PATH)"
TLS_KEY_FILE="$(conf_get TLS_KEY_FILE)"
[[ -z "$TLS_KEY_FILE" ]] && TLS_KEY_FILE="$(conf_get TLS_KEY_PATH)"
DOMAIN="$(conf_get DOMAIN)"
[[ -z "$UPDATE_GROUP" ]] && UPDATE_GROUP="$(conf_get UPDATE_GROUP)"
[[ -z "$INSTANCE_ID" ]] && INSTANCE_ID="$(conf_get UPDATER_INSTANCE_ID)"
[[ -z "$SIGNING_KEY" ]] && SIGNING_KEY="$(conf_get UPDATES_SIGNING_KEY)"

[[ -n "$PARENT_URL" ]]    || die "server.conf is missing UPDATES_PARENT_URL (the parent MySoc relay address)"
[[ -n "$PARENT_ID" ]]     || die "server.conf is missing UPDATES_PARENT_ID (the parent MySoc instance id)"
[[ -n "$CUSTOMER_ID" ]]   || die "server.conf is missing CUSTOMER_ID"
[[ -n "$CUSTOMER_NAME" ]] || die "server.conf is missing CUSTOMER_NAME"

# ---------------------------------------------------------------------------
# 1. Pre-kit tools.
# ---------------------------------------------------------------------------
log "step 1/5: ensuring curl"
if ! command -v curl >/dev/null 2>&1; then
    command -v apt-get >/dev/null 2>&1 || die "missing curl and no apt-get to install it"
    apt-get update -qq
    apt-get install -y curl
fi

# ---------------------------------------------------------------------------
# 2. Fetch + verify the updater kit.
# ---------------------------------------------------------------------------
KIT_URL="${KIT_URL:-$UPDATES_URL/install/widecore-updater-kit-latest.tar.gz}"
log "step 2/5: fetching updater kit from $KIT_URL"
WORK="$(mktemp -d /tmp/widecore-enroll.XXXXXX)"
trap 'rm -rf "$WORK"' EXIT
curl -fsSL "$KIT_URL" -o "$WORK/kit.tar.gz" || die "failed to download kit from $KIT_URL"
if curl -fsSL "$KIT_URL.sha256" -o "$WORK/kit.tar.gz.sha256" 2>/dev/null; then
    expected="$(awk '{print $1}' "$WORK/kit.tar.gz.sha256" | head -1)"
    actual="$(sha256sum "$WORK/kit.tar.gz" | awk '{print $1}')"
    [[ -n "$expected" && "$expected" == "$actual" ]] \
        || die "kit checksum mismatch (expected $expected, got $actual) — refusing to install"
    log "    kit checksum verified: $actual"
else
    warn "no published .sha256 for the kit; proceeding on TLS trust only"
fi
mkdir -p "$WORK/kit"
tar -xzf "$WORK/kit.tar.gz" -C "$WORK/kit"
KIT_DIR="$WORK/kit"
[[ -x "$KIT_DIR/install.sh" ]] || KIT_DIR="$(find "$WORK/kit" -maxdepth 2 -name install.sh -printf '%h\n' | head -1)"
[[ -n "$KIT_DIR" && -x "$KIT_DIR/install.sh" ]] || die "kit is missing install.sh"

# ---------------------------------------------------------------------------
# 3. Place server.conf where first_install reads it, and stage provided TLS
#    material outside /root, /home, and /tmp (ProtectHome=true / PrivateTmp=true).
# ---------------------------------------------------------------------------
log "step 3/5: writing seed $SEED_PATH (0600)"
install -d -m 0750 /etc/widecore
( umask 077; cp "$CONFIG_FILE" "$SEED_PATH" )
chmod 0600 "$SEED_PATH"
chown root:root "$SEED_PATH"

if [[ "$TLS_MODE" == "provided" && ( -n "$TLS_CERT_FILE" || -n "$TLS_KEY_FILE" ) ]]; then
    needs_stage=0
    case "$TLS_CERT_FILE" in /root/*|/home/*|/tmp/*) needs_stage=1 ;; esac
    case "$TLS_KEY_FILE"  in /root/*|/home/*|/tmp/*) needs_stage=1 ;; esac
    if (( needs_stage )); then
        [[ -f "$TLS_CERT_FILE" ]] || die "TLS_CERT_FILE not found: $TLS_CERT_FILE"
        [[ -f "$TLS_KEY_FILE"  ]] || die "TLS_KEY_FILE not found: $TLS_KEY_FILE"
        install -d -m 0700 -o root -g root /etc/widecore/seed-tls
        install -m 0600 -o root -g root "$TLS_CERT_FILE" /etc/widecore/seed-tls/fullchain.pem
        install -m 0600 -o root -g root "$TLS_KEY_FILE"  /etc/widecore/seed-tls/privkey.pem
        tmp_seed="$(umask 077 && mktemp /etc/widecore/.install-seed.env.XXXXXX)"
        sed -e 's|^[[:space:]]*TLS_CERT_FILE=.*|TLS_CERT_FILE="/etc/widecore/seed-tls/fullchain.pem"|' \
            -e 's|^[[:space:]]*TLS_CERT_PATH=.*|TLS_CERT_PATH="/etc/widecore/seed-tls/fullchain.pem"|' \
            -e 's|^[[:space:]]*TLS_KEY_FILE=.*|TLS_KEY_FILE="/etc/widecore/seed-tls/privkey.pem"|' \
            -e 's|^[[:space:]]*TLS_KEY_PATH=.*|TLS_KEY_PATH="/etc/widecore/seed-tls/privkey.pem"|' \
            "$SEED_PATH" > "$tmp_seed"
        chmod 0600 "$tmp_seed"
        chown root:root "$tmp_seed"
        mv -f "$tmp_seed" "$SEED_PATH"
        log "    staged TLS material to /etc/widecore/seed-tls/ (0600) for ProtectHome/PrivateTmp compatibility"
    fi
fi

CA_ARGS=()
if [[ -n "$RELAY_CA_FILE" ]]; then
    [[ -f "$RELAY_CA_FILE" ]] || die "UPDATES_RELAY_CA_FILE not found: $RELAY_CA_FILE"
    CA_ARGS=(--ca-file "$RELAY_CA_FILE")
fi

# ---------------------------------------------------------------------------
# 3b. Instance id: reuse a persisted id so a re-run never creates a second
#     identity.
# ---------------------------------------------------------------------------
if [[ -z "$INSTANCE_ID" && -f "$INSTANCE_ID_FILE" ]]; then
    INSTANCE_ID="$(tr -d '[:space:]' < "$INSTANCE_ID_FILE")"
    [[ -n "$INSTANCE_ID" ]] && log "    reusing persisted instance id: $INSTANCE_ID"
fi
if [[ -z "$INSTANCE_ID" ]]; then
    if command -v uuidgen >/dev/null 2>&1; then
        INSTANCE_ID="widecore-$(uuidgen | tr '[:upper:]' '[:lower:]')"
    elif [[ -r /proc/sys/kernel/random/uuid ]]; then
        INSTANCE_ID="widecore-$(cat /proc/sys/kernel/random/uuid)"
    else
        die "cannot generate an instance id (no uuidgen or /proc uuid); pass --instance-id"
    fi
    log "    generated instance id: $INSTANCE_ID"
fi
( umask 077; printf '%s\n' "$INSTANCE_ID" > "$INSTANCE_ID_FILE" )
chmod 0600 "$INSTANCE_ID_FILE"

# Signing key: prefer explicit value; else trust-on-first-use from the parent
# relay (when reachable unauthenticated) or the updates server that served the
# kit.
if [[ -z "$SIGNING_KEY" ]]; then
    SIGNING_CA_ARGS=()
    [[ -n "$RELAY_CA_FILE" ]] && SIGNING_CA_ARGS=(--cacert "$RELAY_CA_FILE")
    SIGNING_SOURCE=""
    SIGNING_KEY="$(curl -fsS --max-time 10 "${SIGNING_CA_ARGS[@]}" \
        "$PARENT_URL/api/v1/signing-key" 2>/dev/null | grep -o '[0-9a-f]\{64\}' | head -1 || true)"
    if [[ -n "$SIGNING_KEY" ]]; then
        SIGNING_SOURCE="$PARENT_URL"
    else
        SIGNING_KEY="$(curl -fsS --max-time 10 \
            "$UPDATES_URL/api/v1/signing-key" 2>/dev/null | grep -o '[0-9a-f]\{64\}' | head -1 || true)"
        [[ -n "$SIGNING_KEY" ]] && SIGNING_SOURCE="$UPDATES_URL"
    fi
    if [[ -n "$SIGNING_KEY" ]]; then
        warn "signing key fetched from $SIGNING_SOURCE (trust-on-first-use); verify out of band: $SIGNING_KEY"
    else
        die "no signing key: set UPDATES_SIGNING_KEY in server.conf or pass --signing-key (64 hex chars)"
    fi
fi

# ---------------------------------------------------------------------------
# 4. Install + enroll widecore-updater (fresh host, filesystem executor).
# ---------------------------------------------------------------------------
log "step 4/5: installing + enrolling widecore-updater (instance id: $INSTANCE_ID)"
GROUP_ARGS=()
[[ -n "$UPDATE_GROUP" ]] && GROUP_ARGS=(--update-group "$UPDATE_GROUP")
install_args=(--clean --executor filesystem
    --license-key "$LICENSE_KEY" --instance-id "$INSTANCE_ID"
    --parent-url "$PARENT_URL" --parent-id "$PARENT_ID"
    --customer-id "$CUSTOMER_ID" --customer-name "$CUSTOMER_NAME"
    --signing-key "$SIGNING_KEY" "${GROUP_ARGS[@]}" "${CA_ARGS[@]}")
( cd "$KIT_DIR" && bash ./install.sh "${install_args[@]}" )

# ---------------------------------------------------------------------------
# 5. Start widecore-updater and wait for first_install to complete.
# ---------------------------------------------------------------------------
RUN_SINCE="$(date '+%F %T')"
log "step 5/5: starting widecore-updater; waiting up to ${WAIT_SECONDS}s for first_install"
systemctl restart widecore-updater

applied_line() {
    { journalctl -u widecore-updater --since "$RUN_SINCE" --no-pager 2>/dev/null \
        | grep "update applied and verified" | grep -i widecore | tail -1; } || true
}
applied_version() {
    { printf '%s' "$1" | grep -oE 'to_version["=:[:space:]]+"?[0-9][0-9.]+' \
        | grep -oE '[0-9][0-9.]+' | tail -1; } || true
}

ADDRESS="${DOMAIN:+https://$DOMAIN}"
deadline=$((SECONDS + WAIT_SECONDS))
while (( SECONDS < deadline )); do
    line="$(applied_line)"
    if [[ -n "$line" ]]; then
        version="$(applied_version "$line")"
        if curl -fsS --max-time 5 "$HEALTH_URL" >/dev/null 2>&1; then
            health="healthy at $HEALTH_URL"
        else
            health="applied (liveness probe $HEALTH_URL not yet answering)"
        fi
        echo
        log "DONE  widecore ${version:+version $version }$health"
        log "      address:  ${ADDRESS:-<set DOMAIN in server.conf>}"
        log "      instance: $INSTANCE_ID"
        if [[ -f /etc/widecore/first-sign-in.txt ]]; then
            log "      initial admin password: /etc/widecore/first-sign-in.txt (delete after first sign-in)"
        fi
        exit 0
    fi
    if journalctl -u widecore-updater --since "$RUN_SINCE" --no-pager 2>/dev/null \
        | grep -q "first_install failed; backing off"; then
        echo
        warn "first_install failed and widecore-updater is backing off. Recent log:"
        journalctl -u widecore-updater --no-pager -n 25 2>/dev/null | sed 's/^/    /' >&2
        die "fix the reported prerequisite and widecore-updater will retry automatically (or 'systemctl restart widecore-updater')"
    fi
    sleep 10
done

echo
warn "timed out after ${WAIT_SECONDS}s waiting for 'update applied and verified'. Recent updater log:"
journalctl -u widecore-updater --no-pager -n 25 2>/dev/null | sed 's/^/    /' >&2
die "install not confirmed applied yet; check 'journalctl -u widecore-updater -f'"
