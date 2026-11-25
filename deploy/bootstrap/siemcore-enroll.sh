#!/usr/bin/env bash
# siemcore-enroll.sh — ONE-command fresh install for a SiemCore server.
#
# The operator runs a single command on a fresh Ubuntu 24.04 VM (Docker and the
# base packages already present, per SiemCore's prerequisites):
#
#   curl -fsSL https://updates.mysoc.ai/install/siemcore-enroll.sh | sudo bash -s -- \
#     --license-key <KEY> --config /root/server.conf
#
# This THIN bootstrap does only what the updater/artifact cannot do for itself:
#   1. ensure rsync + curl exist,
#   2. fetch + checksum-verify the siemcore updater kit from updates.mysoc.ai,
#   3. place SiemCore's server.conf where its first_install reads it, the TLS
#      material under /etc/ssl/siemcore, and any archive credential out of /root
#      into /etc/siemcore (the updater unit runs with ProtectHome=yes),
#   4. install + enrol the cascade updater (kit install.sh --clean --executor
#      filesystem) against the parent MySoc relay, at version 0.0.0,
#   5. start the updater and wait for the updater's own "update applied and
#      verified" line, then print the address + version.
#
# Everything else — Docker compose, the database, secrets, TLS wiring, the app
# itself — is installed by the release's updater/apply "first_install" phase,
# through the exact same cascade path every later upgrade uses (see the bundled
# UPDATE-ENTRYPOINT-CONTRACT.md).
#
# This script NEVER edits SiemCore's keys: server.conf is copied verbatim to the
# seed path; only the enrolment keys below are read to drive the updater.
#
# Idempotent: re-running reuses the persisted instance id (so the node never
# gets a second identity) and re-enrols without clobbering the seed or cert.
set -euo pipefail

log()  { echo "[siemcore-enroll] $*"; }
warn() { echo "[siemcore-enroll] WARNING: $*" >&2; }
die()  { echo "[siemcore-enroll] ERROR: $*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Defaults / args
# ---------------------------------------------------------------------------
LICENSE_KEY="" CONFIG_FILE="" INSTANCE_ID="" SIGNING_KEY=""
UPDATES_URL="${MYSOC_UPDATES_URL:-https://updates.mysoc.ai}"
KIT_URL="${SIEMCORE_UPDATER_KIT_URL:-}"
WAIT_SECONDS="${SIEMCORE_ENROLL_WAIT:-2100}"   # 35 min; first_install budget is 30m
SEED_PATH="/etc/siemcore/install-seed.env"
INSTANCE_ID_FILE="/etc/siemcore/instance-id"
HEALTH_URL="${SIEMCORE_HEALTH_URL:-http://127.0.0.1:8443/health/live}"

usage() { sed -n '2,22p' "$0"; exit "${1:-0}"; }

while [[ $# -gt 0 ]]; do
    case "$1" in
        --license-key)  LICENSE_KEY="${2:?}"; shift 2 ;;
        --config)       CONFIG_FILE="${2:?}"; shift 2 ;;
        --instance-id)  INSTANCE_ID="${2:?}"; shift 2 ;;
        --signing-key)  SIGNING_KEY="${2:?}"; shift 2 ;;
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

# ---------------------------------------------------------------------------
# Read ONLY the enrolment keys from server.conf. Everything else is SiemCore's
# and is copied through untouched. The file is KEY=value; source it in a
# subshell so stray keys cannot clobber this script's variables.
# ---------------------------------------------------------------------------
conf_get() {
    # conf_get KEY — print the value of KEY from CONFIG_FILE (last wins), with
    # surrounding quotes stripped. No code execution (grep/sed, not source).
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
TLS_CERT_FILE="$(conf_get TLS_CERT_FILE)"
TLS_KEY_FILE="$(conf_get TLS_KEY_FILE)"
UPDATE_GROUP="$(conf_get UPDATE_GROUP)"
# SiemCore server.conf / server_config.py name this key in the SINGULAR.
ARCHIVE_CREDENTIAL_FILE="$(conf_get ARCHIVE_CREDENTIAL_FILE)"
DOMAIN="$(conf_get DOMAIN)"
# SiemCore server.conf names the server SERVER_NAME; use it for the DONE address
# when DOMAIN is not set.
SERVER_NAME="$(conf_get SERVER_NAME)"
# SiemCore's key for the updater identity is UPDATER_INSTANCE_ID (not INSTANCE_ID,
# which is a different SiemCore concept). Precedence: --instance-id flag > conf
# UPDATER_INSTANCE_ID > persisted file > freshly generated id.
[[ -z "$INSTANCE_ID" ]] && INSTANCE_ID="$(conf_get UPDATER_INSTANCE_ID)"
[[ -z "$SIGNING_KEY" ]] && SIGNING_KEY="$(conf_get UPDATES_SIGNING_KEY)"

[[ -n "$PARENT_URL" ]]    || die "server.conf is missing UPDATES_PARENT_URL (the parent MySoc relay address)"
[[ -n "$PARENT_ID" ]]     || die "server.conf is missing UPDATES_PARENT_ID (the parent MySoc instance id)"
[[ -n "$CUSTOMER_ID" ]]   || die "server.conf is missing CUSTOMER_ID"
[[ -n "$CUSTOMER_NAME" ]] || die "server.conf is missing CUSTOMER_NAME"

# ---------------------------------------------------------------------------
# 1. Pre-kit tools.
# ---------------------------------------------------------------------------
log "step 1/5: ensuring rsync + curl"
missing=()
command -v rsync >/dev/null 2>&1 || missing+=(rsync)
command -v curl  >/dev/null 2>&1 || missing+=(curl)
if ((${#missing[@]})); then
    command -v apt-get >/dev/null 2>&1 || die "missing ${missing[*]} and no apt-get to install them"
    apt-get update -qq
    apt-get install -y "${missing[@]}"
fi

# ---------------------------------------------------------------------------
# 2. Fetch + verify the updater kit.
# ---------------------------------------------------------------------------
KIT_URL="${KIT_URL:-$UPDATES_URL/install/siemcore-updater-kit-latest.tar.gz}"
log "step 2/5: fetching updater kit from $KIT_URL"
WORK="$(mktemp -d /tmp/siemcore-enroll.XXXXXX)"
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
# 3. Place server.conf where first_install reads it, and the TLS material.
# ---------------------------------------------------------------------------
log "step 3/5: writing seed $SEED_PATH (0600) and TLS material"
install -d -m 0755 /etc/siemcore
( umask 077; cp "$CONFIG_FILE" "$SEED_PATH" )
chmod 0600 "$SEED_PATH"
chown root:root "$SEED_PATH"

if [[ -n "$TLS_CERT_FILE" || -n "$TLS_KEY_FILE" ]]; then
    install -d -m 0750 /etc/ssl/siemcore
    [[ -n "$TLS_CERT_FILE" ]] && { [[ -f "$TLS_CERT_FILE" ]] || die "TLS_CERT_FILE not found: $TLS_CERT_FILE"; install -m 0644 "$TLS_CERT_FILE" /etc/ssl/siemcore/fullchain.pem; }
    [[ -n "$TLS_KEY_FILE" ]]  && { [[ -f "$TLS_KEY_FILE"  ]] || die "TLS_KEY_FILE not found: $TLS_KEY_FILE";  install -m 0600 "$TLS_KEY_FILE"  /etc/ssl/siemcore/privkey.pem; }
fi

# Archive credential: first_install runs with ProtectHome=yes and so cannot read
# /root or /home. Copy the GCS archiver credential named in server.conf out to
# /etc/siemcore/provisioning/ (0600), the same pattern as the TLS material, so
# the app can read it during first_install regardless of where it was staged.
if [[ -n "$ARCHIVE_CREDENTIAL_FILE" ]]; then
    [[ -f "$ARCHIVE_CREDENTIAL_FILE" ]] || die "ARCHIVE_CREDENTIAL_FILE not found: $ARCHIVE_CREDENTIAL_FILE"
    install -d -m 0700 /etc/siemcore/provisioning
    install -m 0600 -o root -g root "$ARCHIVE_CREDENTIAL_FILE" \
        "/etc/siemcore/provisioning/$(basename "$ARCHIVE_CREDENTIAL_FILE")"
    log "    copied archive credential to /etc/siemcore/provisioning/ (0600)"
fi

# Resolve the relay CA path for the kit (pin only when the relay serves a
# private certificate). If the conf gave one, make sure it exists.
CA_ARGS=()
if [[ -n "$RELAY_CA_FILE" ]]; then
    [[ -f "$RELAY_CA_FILE" ]] || die "UPDATES_RELAY_CA_FILE not found: $RELAY_CA_FILE"
    CA_ARGS=(--ca-file "$RELAY_CA_FILE")
fi

# ---------------------------------------------------------------------------
# 3b. Instance id: reuse a persisted id so a re-run never creates a second
# identity (the DR-lab lesson). Precedence: --instance-id / conf INSTANCE_ID >
# persisted file > a freshly generated, persisted id. The id is NEVER derived
# from the domain (a changing domain must not change identity).
# ---------------------------------------------------------------------------
if [[ -z "$INSTANCE_ID" && -f "$INSTANCE_ID_FILE" ]]; then
    INSTANCE_ID="$(tr -d '[:space:]' < "$INSTANCE_ID_FILE")"
    [[ -n "$INSTANCE_ID" ]] && log "    reusing persisted instance id: $INSTANCE_ID"
fi
if [[ -z "$INSTANCE_ID" ]]; then
    if command -v uuidgen >/dev/null 2>&1; then
        INSTANCE_ID="siemcore-$(uuidgen | tr '[:upper:]' '[:lower:]')"
    elif [[ -r /proc/sys/kernel/random/uuid ]]; then
        INSTANCE_ID="siemcore-$(cat /proc/sys/kernel/random/uuid)"
    else
        die "cannot generate an instance id (no uuidgen or /proc uuid); pass --instance-id"
    fi
    log "    generated instance id: $INSTANCE_ID"
fi
( umask 077; printf '%s\n' "$INSTANCE_ID" > "$INSTANCE_ID_FILE" )
chmod 0600 "$INSTANCE_ID_FILE"

# Signing key: the whole fleet pins one key. Prefer an explicit value; else try
# the parent relay (when reachable unauthenticated) or the updates server that
# served the kit (trust-on-first-use) so the one command still works.
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
# 4. Install + enrol the updater (fresh host, filesystem executor, real mode).
# ---------------------------------------------------------------------------
log "step 4/5: installing + enrolling updater (instance id: $INSTANCE_ID)"
[[ -n "$UPDATE_GROUP" && "$UPDATE_GROUP" != "stable" ]] && \
    warn "UPDATE_GROUP=$UPDATE_GROUP in server.conf cannot be set by the installer; a new instance enrols in 'stable'. Ask the operator to move the ring on the updates server after first heartbeat."
install_args=(--clean --executor filesystem
    --license-key "$LICENSE_KEY" --instance-id "$INSTANCE_ID"
    --parent-url "$PARENT_URL" --parent-id "$PARENT_ID"
    --customer-id "$CUSTOMER_ID" --customer-name "$CUSTOMER_NAME"
    --signing-key "$SIGNING_KEY" "${CA_ARGS[@]}")
( cd "$KIT_DIR" && bash ./install.sh "${install_args[@]}" )

# ---------------------------------------------------------------------------
# 5. Start the updater and wait for the install to be APPLIED AND VERIFIED.
#
# The authoritative signal is the updater's own success line for this product,
# "update applied and verified", NOT the first time :8443 answers a health
# probe — a health endpoint can answer before the first_install completes (old
# container, partial bring-up). We wait on the log line, read the version it
# reports, then confirm health and print the address + version.
# ---------------------------------------------------------------------------
# Record the wall-clock instant just before we (re)start the updater, so the
# journal reads below see ONLY this run. Reading a fixed -2100s window would pick
# up lines from an earlier run — an old "backing off" would make us die at once,
# and an old success would print DONE at once (findings 3). %F %T is the format
# journalctl --since accepts.
RUN_SINCE="$(date '+%F %T')"
log "step 5/5: starting updater; waiting up to ${WAIT_SECONDS}s for first_install"
systemctl restart siemcore-cascade-updater

# The applied-and-verified line for the siemcore product (slog; tolerate both the
# text handler "to_version=X" and a JSON handler "to_version":"X"). The trailing
# `|| true` is load-bearing: under `set -euo pipefail` a grep with no match
# returns 1 and pipefail would propagate it, ending the whole script at the
# `line="$(applied_line)"` assignment before DONE is ever printed (finding 4).
applied_line() {
    { journalctl -u siemcore-cascade-updater --since "$RUN_SINCE" --no-pager 2>/dev/null \
        | grep "update applied and verified" | grep -i siemcore | tail -1; } || true
}
applied_version() {
    { printf '%s' "$1" | grep -oE 'to_version["=:[:space:]]+"?[0-9][0-9.]+' \
        | grep -oE '[0-9][0-9.]+' | tail -1; } || true
}
# DONE address: prefer DOMAIN, fall back to SERVER_NAME (finding 2).
ADDR_HOST="${DOMAIN:-$SERVER_NAME}"
ADDRESS="${ADDR_HOST:+https://$ADDR_HOST}"

deadline=$((SECONDS + WAIT_SECONDS))
while (( SECONDS < deadline )); do
    line="$(applied_line)"
    if [[ -n "$line" ]]; then
        version="$(applied_version "$line")"
        # Best-effort health confirmation (do not fail the install on a flaky probe).
        if curl -fsS --max-time 5 "$HEALTH_URL" >/dev/null 2>&1; then
            health="healthy at $HEALTH_URL"
        else
            health="applied (health probe $HEALTH_URL not yet answering)"
        fi
        echo
        log "DONE  siemcore ${version:+version $version }$health"
        log "      address:  ${ADDRESS:-<set DOMAIN or SERVER_NAME in server.conf>}"
        log "      instance: $INSTANCE_ID"
        log "If this is a new server, set its update ring on the updates server (defaults to 'stable')."
        exit 0
    fi
    if journalctl -u siemcore-cascade-updater --since "$RUN_SINCE" --no-pager 2>/dev/null \
        | grep -q "first_install failed; backing off"; then
        echo
        warn "first_install failed and the updater is backing off. Recent log:"
        journalctl -u siemcore-cascade-updater --no-pager -n 25 2>/dev/null | sed 's/^/    /' >&2
        die "fix the reported prerequisite and the updater will retry automatically (or 'systemctl restart siemcore-cascade-updater')"
    fi
    sleep 10
done

echo
warn "timed out after ${WAIT_SECONDS}s waiting for 'update applied and verified'. Recent updater log:"
journalctl -u siemcore-cascade-updater --no-pager -n 25 2>/dev/null | sed 's/^/    /' >&2
die "install not confirmed applied yet; check 'journalctl -u siemcore-cascade-updater -f'"
