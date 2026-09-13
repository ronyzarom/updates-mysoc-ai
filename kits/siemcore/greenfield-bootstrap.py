#!/usr/bin/env python3
"""Configure the existing updater for a pinned, application-owned first install."""
import json
import hashlib
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys

NAME = 'siemcore-cascade-updater'


def validate(data):
    if set(data) != {'application', 'release'}:
        raise ValueError('application and signed release inputs required')
    release = data['release']
    if not re.fullmatch(r'\d+\.\d+\.\d+\.\d+', release.get('version', '')):
        raise ValueError('release version required')
    for name in ('sha256', 'public_key'):
        if not re.fullmatch(r'[0-9a-f]{64}', release.get(name, '')):
            raise ValueError('release checksum/signing key required')
    import base64
    if len(base64.b64decode(release.get('signature', ''), validate=True)) != 64:
        raise ValueError('signed release required')
    if not re.fullmatch(r'[a-z][a-z0-9-]{0,40}', release.get('channel', '')):
        raise ValueError('explicit release channel required')
    app = data['application']
    if app.get('schema') != 1 or app.get('topology') != 'single':
        raise ValueError('standalone bootstrap required')
    for name in ('cluster_id', 'instance_id', 'database_name'):
        if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]{0,100}', app.get(name, '')):
            raise ValueError('invalid application identity')


def write_private(path, data):
    encoded = json.dumps(data, sort_keys=True).encode()
    if path.exists():
        if path.is_symlink() or path.read_bytes() != encoded:
            raise ValueError('existing bootstrap configuration differs')
        return
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'wb') as stream:
        stream.write(encoded)
        stream.flush()
        os.fsync(stream.fileno())


def main():
    if os.geteuid() != 0 or len(sys.argv) != 3:
        raise ValueError('root invocation and input/kit paths required')
    source, kit = Path(sys.argv[1]), Path(sys.argv[2])
    info = source.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o077:
        raise ValueError('input must be root-owned and private')
    data = json.loads(source.read_text())
    validate(data)
    receipt = Path('/etc/siemcore/updater-bootstrap.json')
    fingerprint = hashlib.sha256(source.read_bytes()).hexdigest()
    if receipt.exists():
        if receipt.is_symlink() or json.loads(receipt.read_text()).get('input_sha256') != fingerprint:
            raise ValueError('bootstrap retry input differs')
        subprocess.run(['systemctl', 'start', NAME], check=True)
        return
    for path in Path('/opt').glob('siemcore-*'):
        if path.name != 'siemcore-cascade':
            raise ValueError('clean bootstrap refuses existing SiemCore resources')
    subprocess.run(['sha256sum', '--quiet', '-c', 'SHA256SUMS'], cwd=kit, check=True)
    config = Path('/etc/' + NAME + '/config.yaml')
    text = config.read_text()
    # Fail before enabling if enrollment identity/signing pin do not match.
    if 'id: ' + data['application']['instance_id'] + '\n' not in text:
        raise ValueError('updater and application instance IDs differ')
    if 'public_key: "' + data['release']['public_key'] + '"' not in text:
        raise ValueError('updater and bootstrap signing pins differ')
    block = '''  executor: filesystem
  filesystem:
    install_root: /opt/siemcore-cascade
    restart_command: ["sudo", "-n", "/usr/local/sbin/siemcore-apply-update"]
    health_command: ["python3", "-c", "import json,os,urllib.request; d=json.load(urllib.request.urlopen('http://127.0.0.1:8443/health/live',timeout=10)); assert d.get('version')==os.environ['VERSION']"]
    command_timeout: 15m
    keep_releases: 3
'''
    if re.search(r'^  executor:', text, re.M):
        if block not in text:
            raise ValueError('existing executor differs')
    else:
        text, count = re.subn(r'(^simulation:\n)', lambda m: m[1] + block, text, flags=re.M)
        if count != 1:
            raise ValueError('expected one simulation configuration')
    text, count = re.subn(r'(^    channel: )[a-z0-9-]+$', lambda m: m[1] + data['release']['channel'], text, flags=re.M)
    if count != 1:
        raise ValueError('expected one product channel')
    root = Path('/etc/siemcore')
    root.mkdir(mode=0o700, exist_ok=True)
    data['application']['machine_id'] = Path('/etc/machine-id').read_text().strip()
    write_private(root / 'greenfield.json', data['application'])
    write_private(root / 'greenfield-release.json', data['release'])
    library = Path('/usr/local/lib/siemcore-cascade')
    library.mkdir(mode=0o755, exist_ok=True)
    shutil.copyfile(kit / 'greenfield-hook.py', library / 'greenfield-hook.py')
    (library / 'greenfield-hook.py').chmod(0o644)
    wrapper = Path('/usr/local/sbin/siemcore-apply-update')
    content = '#!/bin/sh\nexec /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin /usr/bin/python3 /usr/local/lib/siemcore-cascade/greenfield-hook.py "$@"\n'
    if wrapper.exists() and (wrapper.is_symlink() or wrapper.read_text() != content):
        raise ValueError('existing root hook differs')
    wrapper.write_text(content)
    wrapper.chmod(0o755)
    sudoers = Path('/etc/sudoers.d/' + NAME)
    sudoers.write_text(NAME + ' ALL=(root) NOPASSWD: /usr/local/sbin/siemcore-apply-update apply, /usr/local/sbin/siemcore-apply-update rollback\n')
    sudoers.chmod(0o440)
    subprocess.run(['visudo', '-cf', str(sudoers)], check=True)
    subprocess.run(['install', '-d', '-m', '0755', '-o', NAME, '-g', NAME, '/opt/siemcore-cascade'], check=True)
    dropin = Path('/etc/systemd/system/' + NAME + '.service.d')
    dropin.mkdir(exist_ok=True)
    (dropin / 'executor.conf').write_text('[Service]\nNoNewPrivileges=false\nProtectSystem=no\nReadWritePaths=/opt/siemcore-cascade\n')
    config.write_text(text)
    subprocess.run(['systemctl', 'daemon-reload'], check=True)
    write_private(receipt, {'input_sha256': fingerprint})
    subprocess.run(['systemctl', 'start', NAME], check=True)
    print('Updater started; signed first installation will run through the cascade.')


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print('updater bootstrap failed: ' + type(error).__name__, file=sys.stderr)
        sys.exit(1)
