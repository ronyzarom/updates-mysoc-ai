#!/usr/bin/env bash
# mysoc-apply-update — root-side apply step for the mysoc cascade updater.
#
# Invoked (via a single NOPASSWD sudoers entry) by the updater's filesystem
# executor after it stages a verified release and flips the cascade "current"
# symlink. This script bridges the cascade layout to the live app layout:
# it syncs code from the activated release into /opt/mysoc, restarts the
# services, and health-gates the result. A non-zero exit makes the executor
# roll back (flip the symlink to the previous release and re-run this script,
# which then re-syncs the previous code).
#
# Contract (docs/UPDATE-ENTRYPOINT-CONTRACT.md): the phase arrives as $1 and
# as UPDATER_PHASE ("apply" or "rollback"); context in CURRENT_DIR, VERSION,
# FROM_VERSION, PRODUCT, INSTALL_ROOT. The authoritative version is read from
# the release directory itself so the same code path is correct on apply and
# on rollback. After the host-policy checks below, control is delegated to
# the artifact's own updater/apply entrypoint when the release ships one.
set -euo pipefail

log() { echo "[mysoc-apply-update] $*"; }

APP=/opt/mysoc
CUR="${CURRENT_DIR:?CURRENT_DIR environment variable is required}"
PHASE="${1:-${UPDATER_PHASE:-apply}}"

# Preflight everything before mutating anything: a failure past this point
# must never leave disk and running processes out of sync.
for tool in rsync curl install systemctl; do
    command -v "$tool" >/dev/null || { log "required tool missing: $tool"; exit 1; }
done

[[ -e "$CUR" ]] || { log "release dir missing: $CUR"; exit 1; }
RELDIR="$(readlink -f "$CUR")"
[[ -f "$RELDIR/VERSION" ]] || { log "release has no VERSION file"; exit 1; }
RELVER="$(tr -d '[:space:]' < "$RELDIR/VERSION")"
[[ -x "$RELDIR/backend/mysoc-backend" ]] || { log "backend binary missing or not executable"; exit 1; }
[[ -f "$RELDIR/frontend/server.js" ]] || { log "frontend server.js missing"; exit 1; }

# Migration guard: this script never mutates the database. A release that
# ships migrations this host has not seen must be applied through the
# documented DB migration path first; refusing here triggers a clean rollback.
if [[ -d "$RELDIR/migrations" ]]; then
    missing=0
    while IFS= read -r f; do
        name="$(basename "$f")"
        if [[ ! -f "$APP/migrations/$name" ]]; then
            missing=$((missing + 1))
            log "unapplied migration shipped in release: $name"
        fi
    done < <(find "$RELDIR/migrations" -maxdepth 1 -name '*.sql' | sort)
    if (( missing > 0 )); then
        log "REFUSING install: $missing new migration(s) present; run DB migrations first, then retry"
        exit 1
    fi
fi

# Delegation: a release that ships its own entrypoint owns the apply logic
# from here (host policy above has already passed). Exit status flows through.
if [[ -x "$RELDIR/updater/apply" ]]; then
    log "delegating $PHASE of $RELVER to artifact entrypoint"
    exec "$RELDIR/updater/apply" "$PHASE"
fi

log "$PHASE: bringing $RELVER live from $RELDIR"

# Backend: single binary, atomic replace, previous kept beside it.
cp -p "$APP/backend/mysoc-backend" "$APP/backend/mysoc-backend.pre-cascade" 2>/dev/null || true
install -m 0755 -o bitnami -g bitnami "$RELDIR/backend/mysoc-backend" "$APP/backend/.mysoc-backend.new"
mv -f "$APP/backend/.mysoc-backend.new" "$APP/backend/mysoc-backend"

# Frontend: full tree sync; host-side env files are preserved (never shipped).
rsync -a --delete --exclude='.env*' "$RELDIR/frontend/" "$APP/frontend/"
chown -R bitnami:bitnami "$APP/frontend"

# Migrations dir: add-only sync (guard above ensures nothing is pending).
if [[ -d "$RELDIR/migrations" ]]; then
    rsync -a "$RELDIR/migrations/" "$APP/migrations/"
fi

install -m 0644 -o bitnami -g bitnami "$RELDIR/VERSION" "$APP/VERSION"

# Deploy provenance for /version and audits. This file is the wrapper's to
# own; host-side config (.env) is never touched.
cat > "$APP/DEPLOY_INFO.json" <<EOF
{"version": "$RELVER", "applied_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)", "source": "cascade-updater", "from_version": "${FROM_VERSION:-unknown}"}
EOF
chown bitnami:bitnami "$APP/DEPLOY_INFO.json"

log "restarting services"
systemctl restart mysoc-backend mysoc-frontend

log "health gate: backend must report healthy at $RELVER (up to 240s)"
deadline=$((SECONDS + 240))
while true; do
    out="$(curl -fsS --max-time 5 http://127.0.0.1:8080/health 2>/dev/null || true)"
    if grep -q '"status":"healthy"' <<<"$out" && grep -q "\"version\":\"$RELVER\"" <<<"$out"; then
        break
    fi
    if (( SECONDS >= deadline )); then
        log "health gate FAILED; last response: ${out:-<none>}"
        exit 1
    fi
    sleep 5
done
curl -fs --max-time 5 -o /dev/null http://127.0.0.1:3000/ || { log "frontend not responding on :3000"; exit 1; }

log "healthy at $RELVER"
