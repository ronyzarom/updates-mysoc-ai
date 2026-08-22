#!/usr/bin/env bash
# One-shot enrollment of cloud.mysoc.ai into the cascade with real installs.
# Run as root on cloud.mysoc.ai with these files staged in /tmp:
#   /tmp/mysoc-updater-kit/   (kit 1.9.0.1: install.sh, bin/, unit, config)
#   /tmp/cloud-mysoc-config.yaml
#   /tmp/mysoc-apply-update.sh
set -euo pipefail

echo "== 1/7 host prerequisites"
command -v rsync >/dev/null || apt-get install -y rsync

echo "== 2/7 updater kit (user, binary, unit, dirs)"
bash /tmp/mysoc-updater-kit/install.sh

echo "== 3/7 tailored configuration"
install -m 0640 -o root -g mysoc-updater /tmp/cloud-mysoc-config.yaml /etc/mysoc-updater/config.yaml

echo "== 4/7 root apply wrapper"
install -m 0755 -o root -g root /tmp/mysoc-apply-update.sh /usr/local/sbin/mysoc-apply-update

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

echo "== 7/7 start"
systemctl daemon-reload
systemctl restart mysoc-updater
sleep 5
systemctl is-active mysoc-updater
echo "ENROLLMENT COMPLETE"
echo
echo "REMAINING (deliberate manual steps):"
echo " - /opt/mysoc/migrations is EMPTY: the migration guard will refuse any"
echo "   release until the mysoc team baselines it to the set actually applied"
echo "   in cloud's database. This is a safety feature, not a defect."
echo " - Server side: set the instance's update_group (beta) after first heartbeat."
