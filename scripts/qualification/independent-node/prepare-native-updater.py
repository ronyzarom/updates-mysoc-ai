#!/usr/bin/env python3
"""Prepare compiled updater component test in an existing disposable signed root."""
import json
import os
from pathlib import Path
import pwd
import shutil
import subprocess

if os.geteuid() != 0 or os.environ.get('INDEPENDENT_NODE_FIXTURE') != '1':
    raise SystemExit('explicit disposable root fixture required')
root=Path('/root/node-qualification')
if os.environ.get('INDEPENDENT_NODE_FRESH') == '1':
    if Path('/var/lib/siemcore-greenfield/journal.json').exists() or any(Path('/opt').glob('siemcore-node-unlinked-*')):
        raise SystemExit('fresh qualification refuses existing product state')
    if not (root/'fixture.json').is_file():
        raise SystemExit('prepared signed fixture required')
else:
    if not (root/'result.json').is_file() or json.loads((root/'result.json').read_text()).get('status') != 'passed':
        raise SystemExit('successful signed root fixture required')
data=json.loads((root/'envelope.json').read_text())
name='node-updater-fixture'
try:
    user=pwd.getpwnam(name)
except KeyError:
    subprocess.run(['useradd','--system','--no-create-home','--shell','/usr/sbin/nologin',name],check=True)
    user=pwd.getpwnam(name)
fixture=Path('/fixture');fixture.mkdir(mode=0o755,exist_ok=True);fixture.chmod(0o755)
metadata={key:data['release'][key] for key in ('version','sha256','signature','public_key')}
metadata.update(node_id=data['application']['node_id'],updater_id=data['application']['updater_instance_id'])
(fixture/'native-node.json').write_text(json.dumps(metadata));(fixture/'native-node.json').chmod(0o644)
shutil.copyfile(root/'release.tar.gz',fixture/'node-release.tar.gz');(fixture/'node-release.tar.gz').chmod(0o644)
wrapper=Path('/usr/local/sbin/siemcore-apply-update')
content='#!/bin/sh\nexec /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin /usr/bin/python3 /root/node-source/greenfield-hook.py "$@"\n'
if wrapper.exists() and wrapper.read_text()!=content:
    raise SystemExit('refusing to replace another root executor')
wrapper.write_text(content);wrapper.chmod(0o755)
sudo=Path('/etc/sudoers.d/node-updater-fixture')
sudo.write_text(name+' ALL=(root) NOPASSWD: /usr/local/sbin/siemcore-apply-update apply, /usr/local/sbin/siemcore-apply-update health\n');sudo.chmod(0o440)
subprocess.run(['visudo','-cf',str(sudo)],check=True)
# Only updater-owned staging/cache change ownership. Product data/journals and
# private signing/application policy remain protected and unchanged.
for directory in (Path('/opt/siemcore-cascade'),Path('/var/lib/siemcore-cascade-updater/artifacts')):
    for current,dirs,files in os.walk(directory):
        os.chown(current,user.pw_uid,user.pw_gid)
        for entry in dirs+files:
            os.chown(Path(current)/entry,user.pw_uid,user.pw_gid,follow_symlinks=False)
print('Prepared unprivileged updater component test; product policy/data unchanged')
