#!/usr/bin/env bash
# One-shot host-side flip: give the testing.mysoc.ai updater real hands.
# Run as root on testing.mysoc.ai with mysoc-apply-update.sh in /tmp.
set -euo pipefail

echo "== 0/6 host prerequisites"
# Bitnami AMIs ship without rsync; the wrapper preflights it.
command -v rsync >/dev/null || apt-get install -y rsync

echo "== 1/6 install root wrapper"
install -m 0755 -o root -g root /tmp/mysoc-apply-update.sh /usr/local/sbin/mysoc-apply-update

echo "== 2/6 sudoers entry (single command, no password, executor env passed)"
# env_keep is required: the executor hands context (CURRENT_DIR etc.) to the
# wrapper via environment, and sudo's env_reset strips it otherwise.
cat > /etc/sudoers.d/mysoc-updater <<'EOF'
Defaults!/usr/local/sbin/mysoc-apply-update env_keep += "UPDATER_PHASE CURRENT_DIR VERSION FROM_VERSION PRODUCT INSTALL_ROOT"
mysoc-updater ALL=(root) NOPASSWD: SETENV: /usr/local/sbin/mysoc-apply-update
EOF
chmod 0440 /etc/sudoers.d/mysoc-updater
visudo -c -f /etc/sudoers.d/mysoc-updater

echo "== 3/6 systemd override: allow sudo + /opt writes (siemcore-proven)"
mkdir -p /etc/systemd/system/mysoc-updater.service.d
cat > /etc/systemd/system/mysoc-updater.service.d/executor.conf <<'EOF'
# Real installs need sudo (NoNewPrivileges must be off) and the sudo child
# inherits the unit's mount namespace, so ProtectSystem must be off too.
# The privilege boundary is the single-command sudoers entry instead.
[Service]
NoNewPrivileges=no
ProtectSystem=no
EOF

echo "== 4/6 cascade staging root"
mkdir -p /opt/mysoc-cascade
chown mysoc-updater:mysoc-updater /opt/mysoc-cascade
chmod 0755 /opt/mysoc-cascade

echo "== 5/6 patch updater config"
python3 - <<'PYEOF'
cfg_path = "/etc/mysoc-updater/config.yaml"
text = open(cfg_path).read()

old_draft = """  # PRODUCTION INSTALLS: uncomment to perform real versioned installs with
  # atomic activation and automatic rollback. Commands below match the
  # testing.mysoc.ai host (units: mysoc-backend, mysoc-frontend).
  # executor: filesystem
  # filesystem:
  #   install_root: /opt/mysoc
  #   restart_command: ["systemctl", "restart", "mysoc-backend", "mysoc-frontend"]
  #   health_command: ["bash", "-c", "curl -fsS http://127.0.0.1:8080/health"]
  #   keep_releases: 3
"""
new_block = """  # REAL INSTALLS (enabled 2026-08-22): the executor stages verified releases
  # under /opt/mysoc-cascade and activates via the root wrapper, which syncs
  # code into /opt/mysoc, restarts services, and health-gates at :8080.
  # Releases shipping unapplied DB migrations are refused (clean rollback).
  executor: filesystem
  filesystem:
    install_root: /opt/mysoc-cascade
    restart_command: ["sudo", "/usr/local/sbin/mysoc-apply-update"]
    health_command: ["bash", "-c", "curl -fsS http://127.0.0.1:8080/health"]
    keep_releases: 3
    command_timeout: 300s
"""
if old_draft not in text:
    raise SystemExit("draft executor block not found; config drifted, aborting")
text = text.replace(old_draft, new_block)

old_ver = 'current_version: "1.3.12.6"'
if old_ver not in text:
    raise SystemExit("current_version 1.3.12.6 not found; config drifted, aborting")
text = text.replace(old_ver, 'current_version: "1.3.12.9"')

open(cfg_path, "w").write(text)
print("config patched")
PYEOF

echo "== 6/6 reload + restart updater"
systemctl daemon-reload
systemctl restart mysoc-updater
sleep 3
systemctl is-active mysoc-updater
echo "FLIP COMPLETE"
echo
echo "MANUAL CHECKLIST (hosts previously deployed via scp/deploy-update.sh):"
echo " 1. Baseline /opt/mysoc/migrations to the full applied set (the wrapper's"
echo "    migration guard compares release .sql filenames against it; a sparse"
echo "    dir causes false refusals). Copy the repo migration set — files only."
echo " 2. Adjust products[].current_version in this script's config patch to"
echo "    the version actually installed on THIS host before running."
echo " 3. Server side: put the instance in the correct update_group."
