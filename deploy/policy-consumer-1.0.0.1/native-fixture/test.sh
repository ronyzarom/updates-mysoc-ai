set -eu
mkdir -p /root/package /etc/siemcore-cascade-updater /usr/local/lib/siemcore-recovery/1.0.0.4 /var/lib/siemcore-recovery /etc/sudoers.d
cp /package/consumer.py /package/wrapper /package/install.py /package/README.md /package/test_consumer.py /package/files.json /root/package/
cp /runtime/*.py /usr/local/lib/siemcore-recovery/1.0.0.4/
cp /baseline/policy.before.json /etc/siemcore-cascade-updater/recovery-policy.json
cp /fixture/original-wrapper /usr/local/sbin/siemcore-apply-update
chmod 600 /etc/siemcore-cascade-updater/recovery-policy.json
chmod 755 /usr/local/sbin/siemcore-apply-update
printf '# unchanged existing sudo boundary\n' > /etc/sudoers.d/fixture
printf '#!/bin/sh\necho inactive\n' > /usr/local/bin/systemctl
chmod 755 /usr/local/bin/systemctl
sha256sum /etc/sudoers.d/fixture /etc/siemcore-cascade-updater/recovery-policy.json /usr/local/lib/siemcore-recovery/1.0.0.4/*.py > /tmp/before.sha
python3 /root/package/install.py
python3 /root/package/install.py
sha256sum -c /tmp/before.sha
cmp /root/package/wrapper /usr/local/sbin/siemcore-apply-update
cmp /root/package/consumer.py /usr/local/lib/siemcore-policy-consumer/1.0.0.1/consumer.py
printf '#!/bin/sh\necho active\n' > /usr/local/bin/systemctl
if python3 /root/package/install.py >/tmp/refusal 2>&1; then exit 1; fi
grep 'updater must already be stopped' /tmp/refusal
printf '#!/bin/sh\necho inactive\n' > /usr/local/bin/systemctl
printf '{"stage":"applying"}\n' > /var/lib/siemcore-recovery/transaction.json
if python3 /root/package/install.py >/tmp/refusal 2>&1; then exit 1; fi
grep 'unfinished recovery' /tmp/refusal
printf '{"stage":"applied"}\n' > /var/lib/siemcore-recovery/transaction.json
printf '\n# drift\n' >> /usr/local/lib/siemcore-recovery/1.0.0.4/recovery.py
if python3 /root/package/install.py >/tmp/refusal 2>&1; then exit 1; fi
grep 'installed runtime drift' /tmp/refusal
printf 'PASS native isolated installer: idempotent, policy/runtime/sudo preservation, active/pending/drift refusal\n'
