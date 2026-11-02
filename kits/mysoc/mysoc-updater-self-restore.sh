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
# SELF_RESTORE_STATE overrides the state file (tests only).
STATE_FILE=${SELF_RESTORE_STATE:-/var/lib/$NAME/state.json}

log() { logger -t "$NAME-self-restore" -- "$*" 2>/dev/null || echo "$NAME-self-restore: $*" >&2; }

# Only roll the self-update layout back when this failure is actually a
# self-update failure. Evidence: state.json carries a pending_self_update marker
# (written just before the handoff), OR .previous was touched very recently
# (within 1 h) — i.e. an activation just happened. Otherwise an unrelated
# start-limit failure (a bad config edit, a missing dependency) would wrongly
# downgrade to a possibly weeks-old .previous. In that case we still clear the
# failed state and restart so Restart=always keeps trying the intended binary.
is_selfupdate_failure() {
    if [ -f "$STATE_FILE" ] && grep -q '"pending_self_update"' "$STATE_FILE" 2>/dev/null; then
        return 0
    fi
    if [ -f "$PREV_FILE" ]; then
        # .previous modified within the last hour => an activation just happened.
        if [ -n "$(find "$PREV_FILE" -mmin -60 2>/dev/null)" ]; then
            return 0
        fi
    fi
    return 1
}

if ! is_selfupdate_failure; then
    log "failure is not a recent self-update (no pending marker, .previous not recent); not rolling back, just restarting"
    systemctl reset-failed "$NAME.service" 2>/dev/null || true
    systemctl start "$NAME.service" 2>/dev/null || true
    exit 0
fi

if [ ! -f "$PREV_FILE" ]; then
    log "no .previous recorded; nothing to restore"
    systemctl reset-failed "$NAME.service" 2>/dev/null || true
    systemctl start "$NAME.service" 2>/dev/null || true
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
