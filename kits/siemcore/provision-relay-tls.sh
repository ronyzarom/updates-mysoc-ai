#!/bin/bash
# ============================================================
# SiemCore cascade updater  -  PROVISION / ROTATE RELAY TLS
#
# Point the child-facing relay listener (:18443) at a PUBLICLY
# TRUSTED certificate (the fleet SSL.com *.siemcore.ai wildcard)
# so SWF agents validate against the system trust store with no
# per-agent pinning. Run as root on the siemcore host, after
# install.sh, and again on every certificate rotation.
#
# This is a HOST-MAINTENANCE operation invoked directly by an
# operator under explicit approval. It is NOT called from the
# updater's own cascade apply loop (no SSH in the apply path).
#
# What it does (idempotent, atomic, self-rolling-back):
#   1. VALIDATE the source: key matches cert; leaf is not
#      self-signed and carries its chain; leaf covers the expected
#      relay hostname (SAN/CN, wildcard-aware); >= --min-days left.
#   2. NO-CHANGE DETECTION: if the installed cert already has the
#      same SHA-256 fingerprint AND config already points at it,
#      exit 0 without restarting (unless --force-restart).
#   3. BACK UP the current tls/ material and config.yaml.
#   4. ATOMICALLY install fullchain.pem (0644) + privkey.pem (0640,
#      root:siemcore-cascade-updater) into /etc/NAME/tls.
#   5. Set relay.tls.cert_file/key_file in config.yaml (uncommenting
#      the template lines if needed).
#   6. RESTART the service (the relay loads certs at process start).
#   7. VERIFY the served :18443 leaf fingerprint equals the file we
#      installed (parity) and /health responds. On ANY failure,
#      restore the backups and restart back to the known-good state.
#
# Exits: 0 ok/no-op; 2 usage; 3 invalid source; 4 restart failed
# (rolled back); 5 verification failed (rolled back).
# ============================================================
set -euo pipefail

NAME=siemcore-cascade-updater
CONFIG="/etc/$NAME/config.yaml"
TLS_DIR="/etc/$NAME/tls"
LISTEN_PORT=18443

log()  { echo "[provision-relay-tls] $*"; }
warn() { echo "[provision-relay-tls] WARN: $*" >&2; }
err()  { echo "[provision-relay-tls] ERROR: $*" >&2; }

usage() {
    cat <<USAGE
Usage:
  sudo $0 --crt <fullchain.pem> --key <privkey.pem> [options]

Options:
  --expect-host <fqdn>   hostname the served cert MUST cover (SAN/CN,
                         wildcard-aware). Default: host FQDN.
  --min-days <n>         minimum remaining validity in days (default 21).
  --config <path>        updater config (default: $CONFIG)
  --dry-run              validate + report what WOULD change; mutate nothing.
  --force-restart        restart the service even when nothing changed.
  --no-restart           install files + edit config but do not restart.
  --no-verify            skip the post-restart parity / health check.
  -h, --help             this text

Example (fleet SSL.com wildcard already staged on the host):
  sudo $0 --crt /etc/ssl/siemcore/fullchain.pem \\
          --key /etc/ssl/siemcore/privkey.pem \\
          --expect-host cyfox-il.siemcore.ai
USAGE
}

SRC_CRT="" SRC_KEY="" EXPECT_HOST="" MIN_DAYS=21
DRY_RUN=0 FORCE_RESTART=0 DO_RESTART=1 DO_VERIFY=1
while [[ $# -gt 0 ]]; do
    case "$1" in
        --crt)           SRC_CRT="$2"; shift 2 ;;
        --key)           SRC_KEY="$2"; shift 2 ;;
        --expect-host)   EXPECT_HOST="$2"; shift 2 ;;
        --min-days)      MIN_DAYS="$2"; shift 2 ;;
        --config)        CONFIG="$2"; shift 2 ;;
        --dry-run)       DRY_RUN=1; shift ;;
        --force-restart) FORCE_RESTART=1; shift ;;
        --no-restart)    DO_RESTART=0; shift ;;
        --no-verify)     DO_VERIFY=0; shift ;;
        -h|--help)       usage; exit 0 ;;
        *) err "unknown arg: $1"; usage; exit 2 ;;
    esac
done

[[ -n "$SRC_CRT" && -n "$SRC_KEY" ]] || { err "--crt and --key are required"; usage; exit 2; }
[[ "$MIN_DAYS" =~ ^[0-9]+$ ]] || { err "--min-days must be an integer"; exit 2; }
command -v openssl >/dev/null 2>&1 || { err "openssl not on PATH"; exit 2; }
[[ -f "$CONFIG" ]] || { err "config not found: $CONFIG (run install.sh first)"; exit 2; }
[[ -z "$EXPECT_HOST" ]] && EXPECT_HOST="$(hostname -f 2>/dev/null || hostname)"

SUDO=""
[[ $EUID -ne 0 ]] && SUDO="sudo"
can_read() {
    test -r "$1" && return 0
    if test -d "$(dirname "$1")"; then
        test -e "$1" && $SUDO test -r "$1"
        return
    fi
    $SUDO test -r "$1"
}
read_file() { if test -r "$1"; then cat "$1"; else $SUDO cat "$1"; fi; }
# grep -qE that escalates to sudo only when the file isn't directly readable,
# so config.yaml being root/group-only doesn't turn a real match into a
# spurious "not found" (which would insert a duplicate cert_file/key_file).
grep_q() { local re="$1" f="$2"; if test -r "$f"; then grep -qE "$re" "$f"; else $SUDO grep -qE "$re" "$f"; fi; }

can_read "$SRC_CRT" || { err "source cert not readable: $SRC_CRT"; exit 3; }
can_read "$SRC_KEY" || { err "source key not readable: $SRC_KEY"; exit 3; }
SRC_CRT_PEM="$(read_file "$SRC_CRT")"
SRC_KEY_PEM="$(read_file "$SRC_KEY")"

host_matches() {
    local name host suffix hostrest
    name="$(echo "$1" | tr '[:upper:]' '[:lower:]')"
    host="$(echo "$2" | tr '[:upper:]' '[:lower:]')"
    [[ "$name" == "$host" ]] && return 0
    if [[ "$name" == '*.'* ]]; then
        suffix="${name#\*.}"; hostrest="${host#*.}"
        [[ "$host" == *.* && "$hostrest" == "$suffix" ]] && return 0
    fi
    return 1
}

# 1. validate ---------------------------------------------------------------
cpub="$(echo "$SRC_CRT_PEM" | openssl x509 -noout -pubkey 2>/dev/null | openssl md5 2>/dev/null || true)"
kpub="$(echo "$SRC_KEY_PEM" | openssl pkey -pubout 2>/dev/null       | openssl md5 2>/dev/null || true)"
[[ -n "$cpub" && "$cpub" == "$kpub" ]] || { err "private key does NOT match certificate"; exit 3; }

subj="$(echo "$SRC_CRT_PEM" | openssl x509 -noout -subject 2>/dev/null | sed 's/^subject=//')"
issr="$(echo "$SRC_CRT_PEM" | openssl x509 -noout -issuer  2>/dev/null | sed 's/^issuer=//')"
if [[ "$subj" == "$issr" ]]; then
    err "leaf certificate is self-signed (subject == issuer): $subj"
    err "SWF agents validating against public roots will reject it — use the SSL.com wildcard"
    exit 3
fi
chain_count="$(echo "$SRC_CRT_PEM" | grep -c 'BEGIN CERTIFICATE' || true)"
if [[ "${chain_count:-0}" -lt 2 ]]; then
    warn "cert file has only the leaf (no intermediates); clients that don't fetch"
    warn "missing intermediates will FAIL trust. Use the CA fullchain/CA-bundle."
fi
if ! echo "$SRC_CRT_PEM" | openssl x509 -noout -checkend $(( MIN_DAYS * 86400 )) >/dev/null 2>&1; then
    enddate="$(echo "$SRC_CRT_PEM" | openssl x509 -noout -enddate 2>/dev/null | sed 's/^notAfter=//')"
    err "certificate has < ${MIN_DAYS} days validity left (notAfter=$enddate)"; exit 3
fi
san_names=()
while IFS= read -r _san; do [[ -n "$_san" ]] && san_names+=("$_san"); done < <(
    echo "$SRC_CRT_PEM" | openssl x509 -noout -ext subjectAltName 2>/dev/null \
        | grep -oE 'DNS:[^,]+' | sed 's/DNS://' | tr -d ' ')
cn_name="$(echo "$subj" | grep -oE 'CN *= *[^,/]+' | sed 's/CN *= *//' | tr -d ' ' | head -1)"
covered=0
for n in ${san_names[@]+"${san_names[@]}"} "$cn_name"; do
    [[ -z "$n" ]] && continue
    host_matches "$n" "$EXPECT_HOST" && { covered=1; break; }
done
[[ "$covered" -eq 1 ]] || { err "certificate does not cover relay host '$EXPECT_HOST' (SAN: ${san_names[*]:-none})"; exit 3; }

SRC_FP="$(echo "$SRC_CRT_PEM" | openssl x509 -noout -fingerprint -sha256 2>/dev/null | sed 's/.*=//')"
enddate="$(echo "$SRC_CRT_PEM" | openssl x509 -noout -enddate 2>/dev/null | sed 's/^notAfter=//')"
log "validation OK  : key matches cert; covers '$EXPECT_HOST'; >= ${MIN_DAYS}d validity"
log "cert issuer    : ${issr:-<unknown>}"
log "cert not-after : ${enddate:-<unknown>}"
log "cert sha256 fp : ${SRC_FP}"

DST_CRT="$TLS_DIR/fullchain.pem"
DST_KEY="$TLS_DIR/privkey.pem"

# 2. change detection -------------------------------------------------------
cert_changed=1
if can_read "$DST_CRT"; then
    dst_fp="$(read_file "$DST_CRT" | openssl x509 -noout -fingerprint -sha256 2>/dev/null | sed 's/.*=//' || true)"
    [[ -n "$dst_fp" && "$dst_fp" == "$SRC_FP" ]] && cert_changed=0
fi
config_changed=1
if grep_q "^[[:space:]]*cert_file:[[:space:]]*$DST_CRT[[:space:]]*$" "$CONFIG" \
   && grep_q "^[[:space:]]*key_file:[[:space:]]*$DST_KEY[[:space:]]*$" "$CONFIG"; then
    config_changed=0
fi
log "change detect  : cert_changed=$cert_changed config_changed=$config_changed"

if [[ "$DRY_RUN" -eq 1 ]]; then
    if [[ "$cert_changed" -eq 0 && "$config_changed" -eq 0 ]]; then
        log "DRY-RUN        : nothing would change (relay already serves this cert)."
    else
        [[ "$cert_changed"   -eq 1 ]] && log "DRY-RUN        : WOULD install $DST_CRT + $DST_KEY (fp -> $SRC_FP)"
        [[ "$config_changed" -eq 1 ]] && log "DRY-RUN        : WOULD set relay.tls.cert_file/key_file in $CONFIG"
        log "DRY-RUN        : WOULD restart $NAME"
    fi
    exit 0
fi
if [[ "$cert_changed" -eq 0 && "$config_changed" -eq 0 && "$FORCE_RESTART" -eq 0 ]]; then
    log "no-op          : relay already serves this cert and config points at it."
    exit 0
fi

# 3. backups ----------------------------------------------------------------
TS="$(date -u +%Y%m%d-%H%M%S)"
BACKUPS=()
backup_file() { local f="$1"; if $SUDO test -f "$f"; then $SUDO cp -p "$f" "$f.bak-$TS"; BACKUPS+=("$f"); log "backup         : $f -> $f.bak-$TS"; fi; }
restore_backups() { warn "rolling back ..."; for f in ${BACKUPS[@]+"${BACKUPS[@]}"}; do $SUDO test -f "$f.bak-$TS" && { $SUDO cp -p "$f.bak-$TS" "$f"; warn "restored $f"; }; done; }

$SUDO install -d -m 0755 -o root -g root "$TLS_DIR"
backup_file "$DST_CRT"; backup_file "$DST_KEY"; backup_file "$CONFIG"

# 4. atomic install ---------------------------------------------------------
TMP_CRT="$TLS_DIR/.fullchain.pem.tmp-$TS"
TMP_KEY="$TLS_DIR/.privkey.pem.tmp-$TS"
echo "$SRC_CRT_PEM" | $SUDO tee "$TMP_CRT" >/dev/null
echo "$SRC_KEY_PEM" | $SUDO tee "$TMP_KEY" >/dev/null
# The service runs as user NAME; group-read the key, world-read the cert.
$SUDO chown root:"$NAME" "$TMP_CRT" "$TMP_KEY"
$SUDO chmod 0644 "$TMP_CRT"
$SUDO chmod 0640 "$TMP_KEY"
$SUDO mv -f "$TMP_CRT" "$DST_CRT"
$SUDO mv -f "$TMP_KEY" "$DST_KEY"
log "installed      : $DST_CRT (0644) + $DST_KEY (0640 root:$NAME)"

# 5. point config at it -----------------------------------------------------
set_kv() {  # key value : ensure "    key: value" under relay.tls
    local key="$1" val="$2"
    if grep_q "^[[:space:]]*#?[[:space:]]*${key}:[[:space:]]*/" "$CONFIG"; then
        $SUDO sed -i.sedbak -E "s|^[[:space:]]*#?[[:space:]]*${key}:[[:space:]]*/.*|    ${key}: ${val}|" "$CONFIG"
        $SUDO rm -f "$CONFIG.sedbak"
    else
        # No cert_file/key_file line present at all: insert under the relay
        # tls `dir:` line (always present in the kit template).
        $SUDO sed -i.sedbak -E "s|^([[:space:]]*dir:[[:space:]].*/relay-tls)[[:space:]]*\$|\1\n    ${key}: ${val}|" "$CONFIG"
        $SUDO rm -f "$CONFIG.sedbak"
    fi
}
set_kv cert_file "$DST_CRT"
set_kv key_file  "$DST_KEY"
log "config         : relay.tls.cert_file/key_file -> $TLS_DIR"

# 6. restart ----------------------------------------------------------------
if [[ "$DO_RESTART" -eq 0 ]]; then
    log "restart        : skipped (--no-restart). Restart $NAME to load the cert."
    exit 0
fi
log "restarting $NAME ..."
if ! $SUDO systemctl restart "$NAME"; then
    err "systemctl restart failed"; restore_backups; $SUDO systemctl restart "$NAME" || true; exit 4
fi

# 7. verify -----------------------------------------------------------------
if [[ "$DO_VERIFY" -eq 0 ]]; then
    log "verify         : skipped (--no-verify)"; exit 0
fi
verify_failed() { err "$1"; restore_backups; warn "restarting $NAME with restored cert ..."; $SUDO systemctl restart "$NAME" || true; exit 5; }

served_fp=""
for _ in $(seq 1 20); do
    served_fp="$(echo | openssl s_client -connect "127.0.0.1:${LISTEN_PORT}" -servername "$EXPECT_HOST" 2>/dev/null \
        | openssl x509 -noout -fingerprint -sha256 2>/dev/null | sed 's/.*=//' || true)"
    [[ -n "$served_fp" ]] && break
    sleep 1
done
[[ -n "$served_fp" ]] || verify_failed "no certificate served on :${LISTEN_PORT} after restart"
[[ "$served_fp" == "$SRC_FP" ]] || verify_failed "served :${LISTEN_PORT} fp ($served_fp) != installed ($SRC_FP)"
log "parity OK      : :${LISTEN_PORT} serves the canonical cert ($served_fp)"

if ! curl -sS -k --max-time 8 "https://127.0.0.1:${LISTEN_PORT}/health" >/dev/null 2>&1; then
    verify_failed "/health on :${LISTEN_PORT} did not respond after restart"
fi
log "health OK      : :${LISTEN_PORT}/health responding"
log "OK - relay serves the publicly trusted cert (fp $SRC_FP). Children can drop server.ca_file."
exit 0
