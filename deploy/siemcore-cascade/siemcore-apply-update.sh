#!/usr/bin/env bash
# siemcore-apply-update — root-side apply step for the siemcore cascade updater.
#
# PLACEHOLDER (updates-mysoc version/1.18.1). The AUTHORITATIVE shim is
# SiemCore-owned and lives in the siemcore repo under deploy/cascade/. SiemCore
# hands over the exact bytes plus their SHA-256; updates-mysoc then replaces
# this file and its pinned checksum (siemcore-apply-update.sh.sha256) and only
# publishes the kit once the two match. Until then the kit builds and installs,
# but with THIS faithful stand-in, which implements the contract's envelope and
# delegates all real work to the release's own updater/apply entrypoint.
#
# Contract (docs/UPDATE-ENTRYPOINT-CONTRACT.md): the updater's filesystem
# executor runs this command with the lifecycle phase both as $1 and as
# UPDATER_PHASE ("apply", "rollback", or "first_install"); context arrives in
# CURRENT_DIR (the activated release symlink), VERSION, FROM_VERSION, PRODUCT,
# INSTALL_ROOT (and ARTIFACT_KIND / ARTIFACT_SHA256 / DEPENDENCY_VALIDATION).
# The health gate is a SEPARATE health_command the executor runs after this
# returns, so this script never sees a "health" phase.
#
# A siemcore release provisions and runs itself through a compose-based
# entrypoint it ships at updater/apply inside the artifact; this stand-in
# delegates to it and never tries to invent compose/database logic of its own.
set -euo pipefail

log() { echo "[siemcore-apply-update] $*"; }

CUR="${CURRENT_DIR:?CURRENT_DIR environment variable is required}"
PHASE="${1:-${UPDATER_PHASE:-apply}}"

[[ -e "$CUR" ]] || { log "release dir missing: $CUR"; exit 1; }
RELDIR="$(readlink -f "$CUR")"
[[ -f "$RELDIR/VERSION" ]] || { log "release has no VERSION file"; exit 1; }
RELVER="$(tr -d '[:space:]' < "$RELDIR/VERSION")"

# Delegation: a siemcore release owns its whole apply (provision/migrate/
# restart/health) through its bundled entrypoint. Exit status flows through; on
# rollback the previous release's own entrypoint runs instead.
if [[ -x "$RELDIR/updater/apply" ]]; then
    log "delegating $PHASE of $RELVER to artifact entrypoint"
    exec "$RELDIR/updater/apply" "$PHASE"
fi

# No entrypoint in the artifact: this stand-in cannot provision a siemcore host
# (compose, database, secrets, TLS). Refuse rather than report a success that
# did not happen; the updater clears the staging and backs off.
log "release $RELVER ships no updater/apply entrypoint; the SiemCore apply shim"
log "must be provided by SiemCore (deploy/cascade/) — refusing $PHASE"
exit 1
