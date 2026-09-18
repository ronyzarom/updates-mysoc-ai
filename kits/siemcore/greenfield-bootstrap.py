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
FILESYSTEM_BLOCK = '''  executor: filesystem
  filesystem:
    install_root: /opt/siemcore-cascade
    restart_command: ["sudo", "-n", "/usr/local/sbin/siemcore-apply-update"]
    health_command: ["sudo", "-n", "/usr/local/sbin/siemcore-apply-update"]
    command_timeout: 15m
    keep_releases: 3
'''


def updater_identity(application):
    # A pod has one logical application identity and distinct enrolled hosts.
    # Existing standalone inputs keep their previous identity convention.
    value = application.get('updater_instance_id', application.get('instance_id', ''))
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]{0,100}', value):
        raise ValueError('invalid updater node identity')
    return value


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
    if not re.fullmatch(r'[a-z][a-z0-9-]{0,19}', release.get('channel', '')):
        raise ValueError('explicit release channel required')
    app = data['application']
    shape = (app.get('schema'), app.get('topology'))
    if type(app.get('schema')) is int and shape == (5, 'node-unlinked'):
        validate_node(app)
        return
    if type(app.get('schema')) is int and shape == (4, 'observer-unlinked'):
        validate_observer(app)
        return
    if type(app.get('schema')) is not int or shape not in ((1, 'single'), (2, 'pod'), (3, 'single'), (3, 'pod')):
        raise ValueError('supported standalone or pod bootstrap required')
    role = app.get('pod_role') if app['topology'] == 'pod' else None
    if app['topology'] == 'pod' and role not in ('a', 'b', 'witness'):
        raise ValueError('pod bootstrap role required')
    features = {'archive', 'allocation_observer'} & set(app)
    if (app['schema'] == 3) != bool(features):
        raise ValueError('new bootstrap settings require explicit schema 3')
    if 'archive' in features and role == 'witness':
        raise ValueError('archive belongs to data installations')
    if 'allocation_observer' in features:
        if role != 'witness' or not app.get('updater_instance_id'):
            raise ValueError('allocation observer requires explicit witness updater identity')
    # Detailed feature validation belongs to the signed product executor.
    identities = ('cluster_id',)
    if role != 'witness':
        identities += ('instance_id', 'database_name')
    for name in identities:
        if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]{0,100}', app.get(name, '')):
            raise ValueError('invalid application identity')
    updater_identity(app)


def validate_observer(app):
    required = {'schema', 'topology', 'machine_id', 'installation_id', 'updater_instance_id', 'management'}
    if set(app) != required:
        raise ValueError('independent Observer requires exact identity and management fields')
    for field in ('installation_id', 'updater_instance_id'):
        if not isinstance(app[field], str) or not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]{0,100}', app[field]):
            raise ValueError('invalid Observer identity')
    if not isinstance(app['machine_id'], str) or not re.fullmatch(r'[0-9a-f]{32}', app['machine_id']):
        raise ValueError('exact local machine ID required')
    management = app['management']
    if not isinstance(management, dict) or set(management) != {'listen', 'hostname', 'certificate', 'key'}:
        raise ValueError('Observer management TLS settings required')
    if management['listen'] != '0.0.0.0:443' or not isinstance(management['hostname'], str) or not re.fullmatch(r'[a-zA-Z0-9][a-zA-Z0-9.-]+', management['hostname']):
        raise ValueError('Observer HTTPS listener required')
    for field in ('certificate', 'key'):
        path = Path(management[field])
        if not path.is_absolute() or '..' in path.parts:
            raise ValueError('protected absolute TLS paths required')


def validate_node(app):
    extra = {'node_id', 'settings_file', 'settings_sha256'}
    # Reuse the same strictly bounded identity/HTTPS envelope as Observer.
    validate_observer({k: v for k, v in app.items() if k not in extra})
    if not extra.issubset(app) or app['node_id'] not in ('1', '2'):
        raise ValueError('independent node slot must be 1 or 2')
    path = Path(app['settings_file'])
    if not path.is_absolute() or '..' in path.parts:
        raise ValueError('protected absolute settings path required')
    if not isinstance(app['settings_sha256'], str) or not re.fullmatch(r'[0-9a-f]{64}', app['settings_sha256']):
        raise ValueError('settings checksum required')


def validate_local_node(app):
    if app.get('topology') != 'node-unlinked':
        return
    path = Path(app['settings_file'])
    for item in (path, *path.parents):
        info = item.lstat()
        if stat.S_ISLNK(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
            raise ValueError('unprotected node settings path')
    info = path.stat()
    if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600 or info.st_size > 65536:
        raise ValueError('node settings must be bounded root-owned 0600 JSON')
    raw = path.read_bytes()
    if hashlib.sha256(raw).hexdigest() != app['settings_sha256']:
        raise ValueError('node settings checksum mismatch')
    if not isinstance(json.loads(raw, object_pairs_hook=unique_object), dict):
        raise ValueError('node settings must be an object')


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError('duplicate JSON field')
        result[key] = value
    return result


def validate_local_observer(app):
    if app.get('topology') not in ('observer-unlinked', 'node-unlinked'):
        return
    if app['machine_id'] != Path('/etc/machine-id').read_text().strip():
        raise ValueError('Observer machine binding mismatch')
    for field in ('certificate', 'key'):
        path = Path(app['management'][field])
        for item in (path, *path.parents):
            info = item.lstat()
            if stat.S_ISLNK(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
                raise ValueError('unprotected Observer TLS path')
        if not path.is_file() or (field == 'key' and path.stat().st_mode & 0o077):
            raise ValueError('Observer TLS key must be private')


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


def read_input(source):
    info = source.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o077:
        raise ValueError('input must be root-owned and private')
    if info.st_size > 65536:
        raise ValueError('bootstrap input exceeds size limit')
    for parent in source.parents:
        info = parent.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
            raise ValueError('bootstrap parent must be root-owned and protected')
    data = json.loads(source.read_text(), object_pairs_hook=unique_object)
    validate(data)
    validate_local_observer(data['application'])
    validate_local_node(data['application'])
    return data


def require_delivery(data):
    if data['application'].get('topology') == 'node-unlinked':
        raise ValueError('independent node delivery disabled pending joint product qualification')


def main():
    if os.geteuid() != 0 or len(sys.argv) != 3:
        raise ValueError('root invocation and input/kit paths required')
    if sys.argv[1] in ('--validate-input', '--validate-install'):
        data = read_input(Path(sys.argv[2]))
        if sys.argv[1] == '--validate-install':
            require_delivery(data)
        print('Bootstrap envelope valid; signed product validates application prerequisites.')
        return
    source, kit = Path(sys.argv[1]), Path(sys.argv[2])
    data = read_input(source)
    require_delivery(data)
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
    if 'id: ' + updater_identity(data['application']) + '\n' not in text:
        raise ValueError('updater enrollment does not match bootstrap node identity')
    if 'public_key: "' + data['release']['public_key'] + '"' not in text:
        raise ValueError('updater and bootstrap signing pins differ')
    block = FILESYSTEM_BLOCK
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
    if data['application'].get('topology') not in ('observer-unlinked', 'node-unlinked'):
        data['application']['machine_id'] = Path('/etc/machine-id').read_text().strip()
    write_private(root / 'greenfield.json', data['application'])
    write_private(root / 'greenfield-release.json', data['release'])
    library = Path('/usr/local/lib/siemcore-cascade')
    library.mkdir(mode=0o755, exist_ok=True)
    shutil.copyfile(kit / 'greenfield-hook.py', library / 'greenfield-hook.py')
    (library / 'greenfield-hook.py').chmod(0o644)
    recovery = library / 'recovery'
    recovery.mkdir(mode=0o755, exist_ok=True)
    for name in ('recovery.py', 'artifact.py', 'supervise.py'):
        source = kit / 'recovery' / name
        if not source.is_file() or source.is_symlink():
            raise ValueError('transactional recovery payload missing')
        shutil.copyfile(source, recovery / name)
        (recovery / name).chmod(0o644)
    recovery_state = Path('/var/lib/siemcore-recovery')
    recovery_state.mkdir(mode=0o700, exist_ok=True)
    if recovery_state.is_symlink() or recovery_state.stat().st_uid != 0 or recovery_state.stat().st_mode & 0o077:
        raise ValueError('unsafe recovery state directory')
    wrapper = Path('/usr/local/sbin/siemcore-apply-update')
    content = '#!/bin/sh\nexec /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin /usr/bin/python3 /usr/local/lib/siemcore-cascade/greenfield-hook.py "$@"\n'
    if wrapper.exists() and (wrapper.is_symlink() or wrapper.read_text() != content):
        raise ValueError('existing root hook differs')
    wrapper.write_text(content)
    wrapper.chmod(0o755)
    sudoers = Path('/etc/sudoers.d/' + NAME)
    sudoers.write_text(NAME + ' ALL=(root) NOPASSWD: /usr/local/sbin/siemcore-apply-update apply, /usr/local/sbin/siemcore-apply-update rollback, /usr/local/sbin/siemcore-apply-update health\n')
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
