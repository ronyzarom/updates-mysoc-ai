#!/usr/bin/env bash
# mysoc-enroll.sh — ONE-command fresh install for a mysoc server (normal or DR).
#
# The admin runs a single command on a fresh VM:
#
#   curl -fsSL https://updates.mysoc.ai/install/mysoc-enroll.sh | sudo bash -s -- \
#     --license-key <KEY> --domain <FQDN> \
#     --tls wildcard --cert /path/fullchain.pem --key /path/privkey.pem
#
#   # or Let's Encrypt instead of a wildcard cert:
#   ... --tls letsencrypt --email ops@<FQDN>
#
# This THIN bootstrap does only what the updater/artifact cannot do for itself:
#   1. ensure rsync + curl exist (the only tools the apply shim needs up front),
#   2. fetch + checksum-verify the updater kit from updates.mysoc.ai,
#   3. write the install seed (/etc/mysoc/install-seed.env) the artifact reads,
#   4. install + enrol the updater (kit install.sh --clean --executor filesystem),
#   5. start the updater and wait for /health.
#
# Everything else — PostgreSQL, nginx, Node, the service user, the database and
# app role, the .env secrets, TLS, systemd units, the baseline schema and the
# code — is installed by the artifact's updater/apply "first_install" phase,
# through the exact same cascade path every later upgrade uses. A DR server is
# just a normal server; its DR role is chosen later in Settings -> Disaster
# Recovery (step 3). This script is steps 1-2 and is identical for both.
#
# Idempotent: re-running re-enrols without clobbering an existing seed or cert.
set -euo pipefail

log()  { echo "[mysoc-enroll] $*"; }
warn() { echo "[mysoc-enroll] WARNING: $*" >&2; }
die()  { echo "[mysoc-enroll] ERROR: $*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Defaults / args
# ---------------------------------------------------------------------------
LICENSE_KEY="" DOMAIN="" INSTANCE_ID="" SIGNING_KEY=""
TLS_MODE="" TLS_CERT="" TLS_KEY="" LE_EMAIL=""
SIEMCORE_URL=""
SMTP_HOST="" SMTP_PORT="" SMTP_USER="" SMTP_PASS="" SMTP_ENCRYPTION="" SMTP_FROM=""
ADMIN_EMAIL=""
SEED_FILE=""
UPDATES_URL="${MYSOC_UPDATES_URL:-https://updates.mysoc.ai}"
KIT_URL="${MYSOC_UPDATER_KIT_URL:-}"
WAIT_SECONDS="${MYSOC_ENROLL_WAIT:-2100}"   # 35 min; first_install budget is 30m
SEED_PATH="/etc/mysoc/install-seed.env"

usage() { sed -n '2,24p' "$0"; exit "${1:-0}"; }

# Pre-scan argv for --seed-file so it acts as DEFAULTS that explicit flags
# override. The seed uses the artifact's key names (TLS_CERT_PATH, SMTP_USERNAME,
# ...); map them onto this script's variables before the flag loop runs.
args=("$@")
for ((i=0; i<${#args[@]}; i++)); do
    [[ "${args[$i]}" == "--seed-file" ]] && SEED_FILE="${args[$((i+1))]:-}"
done
if [[ -n "$SEED_FILE" ]]; then
    [[ -f "$SEED_FILE" ]] || die "--seed-file not found: $SEED_FILE"
    # shellcheck disable=SC1090
    . "$SEED_FILE"
    TLS_CERT="${TLS_CERT:-${TLS_CERT_PATH:-}}"
    TLS_KEY="${TLS_KEY:-${TLS_KEY_PATH:-}}"
    SMTP_USER="${SMTP_USER:-${SMTP_USERNAME:-}}"
    SMTP_PASS="${SMTP_PASS:-${SMTP_PASSWORD:-}}"
fi

while [[ $# -gt 0 ]]; do
    case "$1" in
        --license-key)    LICENSE_KEY="${2:?}"; shift 2 ;;
        --domain)         DOMAIN="${2:?}"; shift 2 ;;
        --instance-id)    INSTANCE_ID="${2:?}"; shift 2 ;;
        --signing-key)    SIGNING_KEY="${2:?}"; shift 2 ;;
        --tls)            TLS_MODE="${2:?}"; shift 2 ;;
        --cert)           TLS_CERT="${2:?}"; shift 2 ;;
        --key)            TLS_KEY="${2:?}"; shift 2 ;;
        --email)          LE_EMAIL="${2:?}"; shift 2 ;;
        --siemcore-url)   SIEMCORE_URL="${2:?}"; shift 2 ;;
        --smtp-host)      SMTP_HOST="${2:?}"; shift 2 ;;
        --smtp-port)      SMTP_PORT="${2:?}"; shift 2 ;;
        --smtp-user)      SMTP_USER="${2:?}"; shift 2 ;;
        --smtp-pass)      SMTP_PASS="${2:?}"; shift 2 ;;
        --smtp-encryption) SMTP_ENCRYPTION="${2:?}"; shift 2 ;;
        --smtp-from)      SMTP_FROM="${2:?}"; shift 2 ;;
        --admin-email)    ADMIN_EMAIL="${2:?}"; shift 2 ;;
        --seed-file)      SEED_FILE="${2:?}"; shift 2 ;;
        --updates-url)    UPDATES_URL="${2:?}"; shift 2 ;;
        --kit-url)        KIT_URL="${2:?}"; shift 2 ;;
        -h|--help)        usage 0 ;;
        *) die "unknown flag: $1 (see --help)" ;;
    esac
done

[[ $EUID -eq 0 ]] || die "run as root (pipe to 'sudo bash')"

# Required inputs.
[[ -n "$LICENSE_KEY" ]] || die "--license-key is required"
[[ -n "$DOMAIN" ]]      || die "--domain is required"
INSTANCE_ID="${INSTANCE_ID:-$DOMAIN}"

# Infer TLS mode from the material provided when not stated explicitly.
if [[ -z "$TLS_MODE" ]]; then
    if [[ -n "$TLS_CERT" || -n "$TLS_KEY" ]]; then TLS_MODE="wildcard"
    elif [[ -n "$LE_EMAIL" ]];                 then TLS_MODE="letsencrypt"
    else die "specify --tls wildcard (with --cert/--key) or --tls letsencrypt (with --email)"
    fi
fi
case "$TLS_MODE" in
    wildcard)
        [[ -n "$TLS_CERT" && -n "$TLS_KEY" ]] || die "--tls wildcard needs --cert and --key"
        [[ -f "$TLS_CERT" ]] || die "cert not found: $TLS_CERT"
        [[ -f "$TLS_KEY"  ]] || die "key not found: $TLS_KEY" ;;
    letsencrypt)
        [[ -n "$LE_EMAIL" ]] || die "--tls letsencrypt needs --email" ;;
    *) die "--tls must be 'wildcard' or 'letsencrypt'" ;;
esac

# Sensible defaults for optional SMTP fields when a host is given.
if [[ -n "$SMTP_HOST" ]]; then
    SMTP_PORT="${SMTP_PORT:-587}"
    SMTP_ENCRYPTION="${SMTP_ENCRYPTION:-starttls}"
    SMTP_FROM="${SMTP_FROM:-noreply@$DOMAIN}"
fi

# ---------------------------------------------------------------------------
# 1. Pre-artifact tools: the apply shim needs rsync + curl before first_install.
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
KIT_URL="${KIT_URL:-$UPDATES_URL/install/mysoc-updater-kit-latest.tar.gz}"
log "step 2/5: fetching updater kit from $KIT_URL"
WORK="$(mktemp -d /tmp/mysoc-enroll.XXXXXX)"
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
# The tarball may contain a single top-level kit directory; descend into it.
KIT_DIR="$WORK/kit"
[[ -x "$KIT_DIR/install.sh" ]] || KIT_DIR="$(find "$WORK/kit" -maxdepth 2 -name install.sh -printf '%h\n' | head -1)"
[[ -n "$KIT_DIR" && -x "$KIT_DIR/install.sh" ]] || die "kit is missing install.sh"

# ---------------------------------------------------------------------------
# 3. Write the install seed the artifact's first_install reads.
# ---------------------------------------------------------------------------
log "step 3/5: writing install seed $SEED_PATH (0600)"
install -d -m 0755 "$(dirname "$SEED_PATH")"
umask 077
cat > "$SEED_PATH" <<EOF
# mysoc install seed — written by mysoc-enroll.sh for the artifact's
# first_install phase. 0600 root:root. The artifact GENERATES JWT_SECRET,
# ENCRYPTION_KEY and DB_PASSWORD into /opt/mysoc/backend/.env (never here) and
# scrubs SMTP_PASSWORD from this file once .env is written.
DOMAIN=$DOMAIN
TLS_MODE=$TLS_MODE
TLS_CERT_PATH=$TLS_CERT
TLS_KEY_PATH=$TLS_KEY
LE_EMAIL=$LE_EMAIL
SIEMCORE_URL=$SIEMCORE_URL
SMTP_HOST=$SMTP_HOST
SMTP_PORT=$SMTP_PORT
SMTP_USERNAME=$SMTP_USER
SMTP_PASSWORD=$SMTP_PASS
SMTP_ENCRYPTION=$SMTP_ENCRYPTION
SMTP_FROM=$SMTP_FROM
ADMIN_EMAIL=$ADMIN_EMAIL
EOF
chmod 0600 "$SEED_PATH"
chown root:root "$SEED_PATH"

# ---------------------------------------------------------------------------
# 4. Install + enrol the updater (fresh host, filesystem executor, real mode).
# ---------------------------------------------------------------------------
log "step 4/5: installing + enrolling updater (instance id: $INSTANCE_ID)"
install_args=(--clean --executor filesystem
    --license-key "$LICENSE_KEY" --instance-id "$INSTANCE_ID" --parent-url "$UPDATES_URL")
[[ -n "$SIGNING_KEY" ]] && install_args+=(--signing-key "$SIGNING_KEY")
( cd "$KIT_DIR" && bash ./install.sh "${install_args[@]}" )

# ---------------------------------------------------------------------------
# 5. Start the updater and wait for the app to come up.
# ---------------------------------------------------------------------------
log "step 5/5: starting updater; waiting up to ${WAIT_SECONDS}s for first_install"
systemctl restart mysoc-updater

url="https://$DOMAIN"
deadline=$((SECONDS + WAIT_SECONDS))
version=""
while (( SECONDS < deadline )); do
    out="$(curl -fsS --max-time 5 http://127.0.0.1:8080/health 2>/dev/null || true)"
    if grep -q '"status"[[:space:]]*:[[:space:]]*"healthy"' <<<"$out"; then
        version="$(grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' <<<"$out" | head -1 | sed -E 's/.*"([^"]*)"$/\1/')"
        echo
        log "DONE  $url  version=${version:-unknown}"
        log "The server is up. For a DR pair, set primary/standby in Settings -> Disaster Recovery."
        exit 0
    fi
    # Fail fast if the updater has given up on first_install for now.
    if journalctl -u mysoc-updater --since "-${WAIT_SECONDS}s" --no-pager 2>/dev/null \
        | grep -q "first_install failed; backing off"; then
        echo
        warn "first_install failed and the updater is backing off. Recent log:"
        journalctl -u mysoc-updater --no-pager -n 25 2>/dev/null | sed 's/^/    /' >&2
        die "fix the reported prerequisite and the updater will retry automatically (or 'systemctl restart mysoc-updater')"
    fi
    sleep 10
done

echo
warn "timed out after ${WAIT_SECONDS}s waiting for /health. Recent updater log:"
journalctl -u mysoc-updater --no-pager -n 25 2>/dev/null | sed 's/^/    /' >&2
die "install not confirmed healthy yet; check 'journalctl -u mysoc-updater -f'"
