#!/usr/bin/env bash
# siemcore-apply-update — root-side apply hook for the cascade updater.
#
# Invoked via sudo by the unprivileged siemcore-cascade-updater service after
# it has downloaded, signature-verified, and staged a siemcore bundle and
# flipped the "current" symlink.
#
# Phase argument (UPDATE-ENTRYPOINT-CONTRACT v1, updater >= 1.9.1): the
# updater appends the lifecycle phase as a trailing positional argument
# ("apply" or "rollback"). Pre-1.9.1 updaters pass no argument; we default to
# "apply" behavior. The phase is the ONLY caller input we honor — everything
# else (release dir, version) is resolved and validated locally: we never
# trust caller environment (sudo strips it anyway; no SETENV granted) and we
# derive the version from the staged bundle's own MANIFEST.json, which is
# correct on both apply and rollback by construction (the contract's
# "re-read the release dir, not the VERSION env var" rule).
#
# Sudoers grants the updater user exactly this command with no argument,
# "apply", or "rollback" — nothing else.
set -euo pipefail

CURRENT_LINK="/opt/siemcore-cascade/siemcore/current"
RELEASES_ROOT="/opt/siemcore-cascade/siemcore/releases"
INSTALL_DIR="/opt/siemcore-app-app-a"
UPDATER_USER="siemcore-cascade-updater"
LOG_TAG="siemcore-apply-update"

log() { logger -t "$LOG_TAG" -- "$*" || true; echo "[$LOG_TAG] $*"; }
die() { log "FATAL: $*"; exit 1; }

[ "$(id -u)" -eq 0 ] || die "must run as root"
[ $# -le 1 ] || die "at most one argument (the phase) is accepted"
PHASE="${1:-apply}"
case "$PHASE" in
    apply|rollback|health|first_install) ;;
    *) die "unsupported phase '$PHASE' (expected apply, rollback, health or first_install)" ;;
esac

# Servers installed by first_install: ordinary phases go to the root hook.
#
# first_install ends by installing the release's own update boundary
# (updater/host_files.py): the hook, the recovery runtime it runs, and the
# pin that names the signing key. From then on an update, a rollback and the
# health check all go through that hook, which takes a snapshot before the
# update, verifies the signed release from a private copy, and restores the
# previous version when the update fails (recovery protocol 1). updater/apply
# refuses an ordinary update that does not arrive that way.
#
# first_install itself never goes to the hook: it is the phase that puts the
# hook in place, and it must stay runnable again after an interruption.
# A host with no hook (installed before the hook existed, and without the
# recovery runtime in its own wrapper) keeps the direct path below.
HOOK="/usr/local/lib/siemcore-cascade/greenfield-hook.py"
HOOK_PIN="/etc/siemcore/greenfield-release.json"
if [ "$PHASE" != "first_install" ] && [ -f "$HOOK" ] && [ ! -L "$HOOK" ] && [ -f "$HOOK_PIN" ] && [ ! -L "$HOOK_PIN" ]; then
    [ "$(stat -c %u "$HOOK")" = "0" ] || die "update hook $HOOK is not root-owned"
    exec /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin /usr/bin/python3 "$HOOK" "$PHASE"
fi

# Missing symlink is phase-dependent:
#  - rollback: the executor removes the symlink on a FRESH-install rollback
#    (the first-ever cascade apply failed). The pre-existing app was installed
#    outside the cascade, so there is nothing to re-apply — but the failed
#    update.sh may have left the app down (its contract leaves rollback
#    material for the operator rather than auto-restoring). Per the
#    entrypoint contract, exit 0 only if the previous version is actually
#    live; otherwise exit non-zero so the failure report flags that the host
#    needs manual attention.
#  - apply: the symlink must exist (the executor flips it before invoking us);
#    its absence means something is broken — fail loudly.
if [ ! -L "$CURRENT_LINK" ]; then
    [ "$PHASE" != "health" ] || die "phase=health but no current symlink at $CURRENT_LINK"
    # apply and first_install both require the staged release the executor
    # flips into place before invoking us; its absence is a hard failure.
    [ "$PHASE" = "rollback" ] || die "phase=$PHASE but no current symlink at $CURRENT_LINK"
    for _ in 1 2 3 4 5 6; do
        if health="$(curl -fsS --max-time 10 http://127.0.0.1:8443/health/live 2>/dev/null)"; then
            log "fresh-install rollback: no staged release to restore; running app is alive ($health)"
            exit 0
        fi
        sleep 5
    done
    die "fresh-install rollback and the app is NOT responding on :8443 — manual attention required (update.sh rollback material is preserved in $INSTALL_DIR)"
fi

RELEASE_DIR="$(readlink -f "$CURRENT_LINK")"
[ -d "$RELEASE_DIR" ] || die "current symlink target $RELEASE_DIR is not a directory"

case "$RELEASE_DIR" in
    "$RELEASES_ROOT"/*) ;;
    *) die "release dir $RELEASE_DIR escapes $RELEASES_ROOT" ;;
esac

owner="$(stat -c %U "$RELEASE_DIR")"
[ "$owner" = "$UPDATER_USER" ] || die "release dir owned by '$owner', expected '$UPDATER_USER'"

META="$RELEASE_DIR/.updater-release.json"
[ -f "$META" ] || die "missing release metadata $META"
product="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["product"])' "$META")"
version="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$META")"
[ "$product" = "siemcore" ] || die "unexpected product '$product'"
[ -n "$version" ] || die "empty version in $META"

# The universal bundle extracts into a nested siemcore-universal-<version>/ dir.
BUNDLE_DIR="$RELEASE_DIR/siemcore-universal-$version"
[ -d "$BUNDLE_DIR" ] || die "bundle dir $BUNDLE_DIR not found in staged release"

manifest_version="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$BUNDLE_DIR/MANIFEST.json")"
[ "$manifest_version" = "$version" ] || die "MANIFEST.json version '$manifest_version' != staged metadata '$version'"

# Integrity: re-verify every bundle file against the bundle's own checksums.
# (The updater already verified the artifact's ed25519 signature + SHA-256
# before extraction; this catches post-extraction tampering in the staging
# tree, which is writable by the unprivileged updater user.)
( cd "$BUNDLE_DIR" && sha256sum --quiet -c checksums.txt ) || die "bundle checksum verification failed"

# --accept-update-sh-change: the ed25519 release signature already
# authenticates the bundle contents including update.sh itself; interactive
# consent is replaced by that signature in the cascade path.
log "phase=$PHASE: applying siemcore $version from $BUNDLE_DIR (install dir $INSTALL_DIR)"
cd "$BUNDLE_DIR"

# Delegation ordering (Update Entrypoint Contract v1.2): the shim enforces
# UNIVERSAL host policy first (tree containment, ownership, manifest
# cross-check, checksums — all done above), THEN delegates to the artifact's
# own entrypoint when present. The shim's fallback preconditions (expected
# layout, the unapplied-migration guard) apply ONLY to entrypoint-less
# releases — they must never gate an entrypoint-bearing release, otherwise a
# release that ships migrations together with the installer meant to run them
# would be refused before that installer executes. The entrypoint OWNS the
# whole apply (migrate → sync → restart → health) for both phases.
if [ -x "$BUNDLE_DIR/updater/apply" ]; then
    log "delegating to in-artifact entrypoint updater/apply (phase=$PHASE)"
    # Export the contract environment. sudo strips env by default; the
    # sudoers env_keep list preserves these for the shim, and we re-export
    # so the entrypoint (and the update.sh it calls) see them regardless.
    FROM_VERSION="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("installed_version",""))' "$INSTALL_DIR/.node.json" 2>/dev/null || echo "")"
    export UPDATER_PHASE="$PHASE" \
           PRODUCT="siemcore" \
           VERSION="$version" \
           FROM_VERSION="$FROM_VERSION" \
           CURRENT_DIR="$RELEASE_DIR" \
           INSTALL_ROOT="/opt/siemcore-cascade" \
           SIEMCORE_INSTALL_DIR="$INSTALL_DIR"
    ./updater/apply "$PHASE"
    log "siemcore $version phase=$PHASE via entrypoint completed"
else
    # Fallback: entrypoint-less release (pre-3.3.135). Preserve the historical
    # behavior — bring the staged release live via the app role's update.sh.
    log "no updater/apply entrypoint in bundle; using shim fallback (phase=$PHASE)"
    ./roles/app/update.sh --version "$version" --install-dir "$INSTALL_DIR" --yes --accept-update-sh-change
    log "siemcore $version applied successfully"
fi
