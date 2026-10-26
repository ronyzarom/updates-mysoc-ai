#!/usr/bin/env bash
# Enrol a fresh MySoc DR-lab host (testing-dr-a / testing-dr-b) into the
# cascade with the filesystem executor, STARTING IN OBSERVE MODE.
#
# This is the enroll-cloud.sh pattern generalised to take the tailored config
# as an argument. Run as root ON THE HOST with these files staged in /tmp:
#   /tmp/mysoc-updater-kit/      (built kit: install.sh, bin/, unit, config)
#   /tmp/testing-dr-config.yaml  (deploy/testing-dr/config-a.yaml or -b.yaml,
#                                 with server.license_key filled in)
#   /tmp/mysoc-apply-update.sh   (deploy/mysoc-apply-update.sh — root wrapper)
#
# Order of operations for the whole enrolment (host + server):
#   1. Run this script -> updater installed, running in OBSERVE mode.
#   2. First heartbeat creates the instance row (group=stable, auto-update on).
#   3. Server side (from the Mac, admin API key):
#        PUT /api/v1/instances/<id>/update-group   {"group":"alpha"}
#        PUT /api/v1/instances/<id>/auto-update     {"enabled":true}
#   4. On the host: set simulation.mode: real in /etc/mysoc-updater/config.yaml
#      and `systemctl restart mysoc-updater`. The next cycle takes the highest
#      alpha mysoc artifact through the root wrapper + the artifact entrypoint.
#
# OBSERVE until step 4 is deliberate: it stops the fresh 0.0.0 node from
# installing the highest *stable* release before it is moved to alpha.
set -euo pipefail

CONFIG_SRC="${1:-/tmp/testing-dr-config.yaml}"
KIT_DIR="${KIT_DIR:-/tmp/mysoc-updater-kit}"
WRAPPER_SRC="${WRAPPER_SRC:-/tmp/mysoc-apply-update.sh}"

[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }
[[ -f "$CONFIG_SRC" ]] || { echo "missing config: $CONFIG_SRC" >&2; exit 1; }
[[ -x "$KIT_DIR/install.sh" ]] || { echo "missing kit: $KIT_DIR/install.sh" >&2; exit 1; }
[[ -f "$WRAPPER_SRC" ]] || { echo "missing wrapper: $WRAPPER_SRC" >&2; exit 1; }

if grep -q 'PASTE-PLATFORM-KEY' "$CONFIG_SRC"; then
    echo "ERROR: $CONFIG_SRC still has the placeholder platform key; fill server.license_key first" >&2
    exit 1
fi

echo "== 1/7 host prerequisites"
command -v rsync >/dev/null || apt-get install -y rsync

echo "== 2/7 updater kit (user, binary, unit, dirs)"
bash "$KIT_DIR/install.sh"

echo "== 3/7 tailored configuration (OBSERVE mode)"
install -m 0640 -o root -g mysoc-updater "$CONFIG_SRC" /etc/mysoc-updater/config.yaml

echo "== 4/7 root apply wrapper"
install -m 0755 -o root -g root "$WRAPPER_SRC" /usr/local/sbin/mysoc-apply-update

echo "== 5/7 sudoers (single command, env passed through)"
cat > /etc/sudoers.d/mysoc-updater <<'EOF'
Defaults!/usr/local/sbin/mysoc-apply-update env_keep += "UPDATER_PHASE CURRENT_DIR VERSION FROM_VERSION PRODUCT INSTALL_ROOT"
mysoc-updater ALL=(root) NOPASSWD: SETENV: /usr/local/sbin/mysoc-apply-update
EOF
chmod 0440 /etc/sudoers.d/mysoc-updater
visudo -c -f /etc/sudoers.d/mysoc-updater

echo "== 6/7 unit override (sudo + /opt writes) and staging root"
mkdir -p /etc/systemd/system/mysoc-updater.service.d
cat > /etc/systemd/system/mysoc-updater.service.d/executor.conf <<'EOF'
[Service]
NoNewPrivileges=no
ProtectSystem=no
EOF
mkdir -p /opt/mysoc-cascade
chown mysoc-updater:mysoc-updater /opt/mysoc-cascade
chmod 0755 /opt/mysoc-cascade

echo "== 7/7 start (OBSERVE)"
systemctl daemon-reload
systemctl enable --now mysoc-updater
sleep 5
systemctl is-active mysoc-updater
echo "ENROLLMENT (OBSERVE) COMPLETE"
echo
echo "NEXT (deliberate manual steps):"
echo " - Confirm the instance appears on updates.mysoc.ai, then set its"
echo "   update_group=alpha and auto-update=true (server side, admin API key)."
echo " - mysoc-ai host prerequisites MUST be in place before switching to real:"
echo "     * mysoc-backend.service and mysoc-frontend.service installed+enabled"
echo "       (User=mysoc), backend boots from /opt/mysoc/backend/.env;"
echo "     * the alpha artifact ships updater/apply (recovery-aware) + migrate."
echo " - Then set simulation.mode: real in /etc/mysoc-updater/config.yaml and"
echo "   'systemctl restart mysoc-updater'. Watch: journalctl -u mysoc-updater -f"
echo " - /opt/mysoc/migrations starts empty: the artifact entrypoint applies"
echo "   migrations on the PRIMARY and records them; the STANDBY skips them"
echo "   (pg_is_in_recovery). The shim's fallback guard is never reached for an"
echo "   entrypoint-bearing release."
