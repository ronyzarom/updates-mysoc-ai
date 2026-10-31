#!/bin/sh
# mysoc-updater-self-restore — root-owned self-update backstop (survivability
# invariant 4). Invoked by mysoc-updater-restore.service when the updater has
# failed its start limit, which is the signature of a self-update to a binary
# that cannot stay up. It points the self-update "current" symlink back at the
# recorded ".previous" target and restarts the main unit on the known-good
# binary. Idempotent and safe to run when there is nothing to restore.
set -eu

NAME=mysoc-updater
# SELF_RESTORE_LAYOUT overrides the layout root (tests only); production uses the
# fixed service path.
LAYOUT=${SELF_RESTORE_LAYOUT:-/var/lib/$NAME/self-update}
CURRENT_LINK=$LAYOUT/current
PREV_FILE=$LAYOUT/.previous

log() { logger -t "$NAME-self-restore" -- "$*" 2>/dev/null || echo "$NAME-self-restore: $*" >&2; }

if [ ! -f "$PREV_FILE" ]; then
    log "no .previous recorded; nothing to restore"
    exit 0
fi

PREV=$(cat "$PREV_FILE" 2>/dev/null || true)
if [ -z "$PREV" ] || [ ! -d "$PREV" ] || [ ! -e "$PREV/$NAME" ]; then
    log "recorded previous target is missing or invalid ($PREV); leaving current as-is"
    # Still try to clear the failed state so Restart=always can keep trying.
    systemctl reset-failed "$NAME.service" 2>/dev/null || true
    systemctl start "$NAME.service" 2>/dev/null || true
    exit 0
fi

CUR=$(readlink "$CURRENT_LINK" 2>/dev/null || true)
if [ "$CUR" = "$PREV" ]; then
    log "current already points at previous ($PREV); not a self-update failure"
else
    # Replace the symlink in place; ln -sfn is effectively atomic for our purpose.
    ln -sfn "$PREV" "$CURRENT_LINK"
    # Keep the service-user able to traverse/read the layout.
    chown -h "$NAME:$NAME" "$CURRENT_LINK" 2>/dev/null || true
    log "restored updater current -> $PREV after repeated failures"
fi

# Clear the failed state and bring the known-good binary back up.
systemctl reset-failed "$NAME.service" 2>/dev/null || true
systemctl start "$NAME.service" 2>/dev/null || true
