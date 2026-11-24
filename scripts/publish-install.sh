#!/usr/bin/env bash
# publish-install.sh — publish the one-command fresh-install bootstrap + updater
# kit to the PUBLIC /install/ path on updates.mysoc.ai.
#
# Usage: scripts/publish-install.sh [mysoc|siemcore|widecore] [--yes]   (default: mysoc)
#
# This serves (for the mysoc tier; siemcore is the twin, siemcore-*):
#   https://updates.mysoc.ai/install/mysoc-enroll.sh
#   https://updates.mysoc.ai/install/mysoc-updater-kit-<VERSION>.tar.gz(.sha256)
#   https://updates.mysoc.ai/install/mysoc-updater-kit-latest.tar.gz(.sha256)
# so the admin one-liner works:
#   curl -fsSL https://updates.mysoc.ai/install/mysoc-enroll.sh | sudo bash -s -- ...
#
# DEPLOY-GATED (see .cursor/rules/development-cycle.mdc): this pushes to
# production. It refuses a dirty tree and does nothing but PRINT a plan unless
# run with --yes. Only run it after Rony's explicit deploy instruction.
set -euo pipefail

cd "$(dirname "$0")/.."

SERVER_USER="${PUBLISH_USER:-bitnami}"
SERVER_IP="${PUBLISH_HOST:-18.201.144.150}"
SSH_KEY="${PUBLISH_SSH_KEY:-$HOME/Downloads/LightsailDefaultKey-eu-west-updates-mysoc.ai.pem}"
REMOTE_PUBLIC_DIR="${PUBLISH_REMOTE_DIR:-/home/bitnami/updates-mysoc-ai/public/install}"

CONFIRM=0
TIER=mysoc
for arg in "$@"; do
    case "$arg" in
        --yes)                     CONFIRM=1 ;;
        mysoc|siemcore|widecore)   TIER="$arg" ;;
        *) echo "unknown argument: $arg (expected mysoc|siemcore|widecore and/or --yes)" >&2; exit 1 ;;
    esac
done

KIT_NAME="${TIER}-updater-kit"
BOOTSTRAP="deploy/bootstrap/${TIER}-enroll.sh"
ENROLL_BASENAME="${TIER}-enroll.sh"

VERSION="$(tr -d '[:space:]' < VERSION)"
if [[ -n "$(git status --porcelain)" ]]; then
    echo "ERROR: working tree is dirty; publish only a clean, committed build (dev-cycle rule)." >&2
    exit 1
fi

KIT_DIR="dist/updater-kits"
TARBALL="$KIT_DIR/${KIT_NAME}-${VERSION}.tar.gz"
if [[ ! -f "$TARBALL" ]]; then
    echo "==> building kit ${VERSION} (not found at $TARBALL)"
    scripts/build-kits.sh "$TIER"
fi
[[ -f "$TARBALL" ]] || { echo "ERROR: $TARBALL still missing after build" >&2; exit 1; }
[[ -f "$TARBALL.sha256" ]] || { echo "ERROR: $TARBALL.sha256 missing" >&2; exit 1; }
[[ -f "$BOOTSTRAP" ]] || { echo "ERROR: $BOOTSTRAP missing" >&2; exit 1; }

base="${KIT_NAME}-${VERSION}.tar.gz"
latest="${KIT_NAME}-latest.tar.gz"
cat <<PLAN

Publish plan (tier ${TIER}, version ${VERSION}):
  from:
    $BOOTSTRAP
    $TARBALL (+ .sha256)
  to:  ${SERVER_USER}@${SERVER_IP}:${REMOTE_PUBLIC_DIR}/
    ${ENROLL_BASENAME}
    ${base} (+ .sha256)
    ${latest} (+ .sha256)  -> copy of ${base}
PLAN

if [[ "$CONFIRM" -ne 1 ]]; then
    echo "DRY RUN — re-run with --yes to publish. (Requires Rony's deploy approval.)"
    exit 0
fi

[[ -f "$SSH_KEY" ]] || { echo "ERROR: SSH key not found: $SSH_KEY (set PUBLISH_SSH_KEY)" >&2; exit 1; }
SSH=(ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "${SERVER_USER}@${SERVER_IP}")
SCP=(scp -i "$SSH_KEY" -o StrictHostKeyChecking=no)

echo "==> ensuring remote public dir ${REMOTE_PUBLIC_DIR}"
"${SSH[@]}" "mkdir -p '${REMOTE_PUBLIC_DIR}'"

echo "==> uploading bootstrap + kit"
"${SCP[@]}" "$BOOTSTRAP"       "${SERVER_USER}@${SERVER_IP}:${REMOTE_PUBLIC_DIR}/${ENROLL_BASENAME}"
"${SCP[@]}" "$TARBALL"         "${SERVER_USER}@${SERVER_IP}:${REMOTE_PUBLIC_DIR}/${base}"
"${SCP[@]}" "$TARBALL.sha256"  "${SERVER_USER}@${SERVER_IP}:${REMOTE_PUBLIC_DIR}/${base}.sha256"

echo "==> pointing -latest at ${base} and fixing perms"
"${SSH[@]}" "cd '${REMOTE_PUBLIC_DIR}' \
    && cp -f '${base}' '${latest}' \
    && sed 's#${base}#${latest}#' '${base}.sha256' > '${latest}.sha256' \
    && chmod 0644 '${ENROLL_BASENAME}' '${base}' '${base}.sha256' '${latest}' '${latest}.sha256'"

echo "==> verifying public URLs"
for f in "${ENROLL_BASENAME}" "${latest}.sha256"; do
    if curl -fsSL "https://updates.mysoc.ai/install/$f" >/dev/null 2>&1; then
        echo "    OK  https://updates.mysoc.ai/install/$f"
    else
        echo "    WARN unreachable: https://updates.mysoc.ai/install/$f (is the nginx /install/ location live? see deploy/nginx/)" >&2
    fi
done

echo "DONE — published ${VERSION} to ${REMOTE_PUBLIC_DIR}"
